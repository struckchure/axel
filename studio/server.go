// Package studio serves the Axel Studio — a Neon/Prisma-Studio-style database
// viewer and editor driven by AQL, embeddable as a library and mounted by the
// `axel studio` CLI command.
package studio

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/struckchure/axel/core/lsp"
	"github.com/struckchure/axel/studio/db"
	"github.com/struckchure/axel/studio/pages"
)

const (
	// DefaultAddr is the studio's default listen address.
	DefaultAddr    = ":4530"
	defaultDBURL   = "postgres://user:password@localhost:5432/db?sslmode=disable"
	defaultPageSze = 50
)

//go:embed assets
var assetsFS embed.FS

// Options configures a studio server.
type Options struct {
	Addr        string // listen address, e.g. ":4530"
	DatabaseURL string // postgres connection url ("" → default)
	SchemaPath  string // path to an .asl file ("" → AQL disabled)
}

type server struct {
	store  db.Store
	schema *db.Schema // loaded ASL schema, or nil
	aql    *db.AQL    // AQL engine (compile-only or live), or nil
}

// Handler builds the studio's HTTP handler for the given options, connecting to
// the database and loading the schema. Assets are embedded, so it runs from any
// working directory.
func Handler(opts Options) http.Handler {
	if opts.DatabaseURL == "" {
		opts.DatabaseURL = defaultDBURL
	}
	srv := &server{store: openStore(opts.DatabaseURL)}
	srv.loadSchema(opts.SchemaPath)

	assets, _ := fs.Sub(assetsFS, "assets")
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/query", srv.handleSQL)
	mux.HandleFunc("/aql", srv.handleAQL)
	mux.HandleFunc("/mutate", srv.handleMutate)
	mux.HandleFunc("/lsp/completion", srv.handleCompletion)
	mux.HandleFunc("/lsp/hover", srv.handleHover)
	mux.HandleFunc("/lsp/diagnostics", srv.handleDiagnostics)
	mux.HandleFunc("/", srv.handleStudio)

	log.Printf("axel studio → http://localhost%s  (db: %s, aql: %s)", opts.Addr, srv.store.Name(), srv.aqlStatus())
	return mux
}

// Run starts the studio server and blocks until it exits.
func Run(opts Options) error {
	if opts.Addr == "" {
		opts.Addr = DefaultAddr
	}
	return http.ListenAndServe(opts.Addr, Handler(opts))
}

// openStore connects to Postgres, falling back to sample data so the UI always
// renders even without a reachable database.
func openStore(url string) db.Store {
	pg, err := db.Connect(context.Background(), url)
	if err != nil {
		log.Printf("no database (%v) — serving sample data", err)
		return db.NewMock()
	}
	return pg
}

// loadSchema loads the ASL schema (if any) and builds the AQL engine. The engine
// executes when the store is a live Postgres, and is compile-only otherwise.
func (s *server) loadSchema(path string) {
	if path == "" {
		return
	}
	schema, err := db.LoadSchema(path)
	if err != nil {
		log.Printf("schema not loaded (%v) — AQL disabled", err)
		return
	}
	s.schema = schema
	var pool = poolOf(s.store)
	s.aql = db.NewAQL(schema, pool)
}

// aqlLive reports whether AQL execution is backed by a live database.
func (s *server) aqlLive() bool { return s.aql != nil && s.aql.Live() }

func (s *server) aqlStatus() string {
	switch {
	case s.aqlLive():
		return "live @ " + s.schema.Path
	case s.aql != nil:
		return "compile-only @ " + s.schema.Path
	default:
		return "off"
	}
}

func (s *server) handleStudio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	v := pages.StudioView{
		ConnName:  s.store.Name(),
		Live:      s.store.Live(),
		Tab:       firstNonEmpty(q.Get("tab"), "data"),
		QuerySQL:  q.Get("sql"),
		HasSchema: s.aql != nil,
		AQLLive:   s.aqlLive(),
	}

	tables, err := s.tables(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v.Tables = tables

	if s.schema != nil && s.schema.IR != nil {
		v.Enums = make(map[string][]string)
		for name, enum := range s.schema.IR.EnumTypes {
			v.Enums[name] = enum.Values
		}
	}

	schema, table := q.Get("schema"), q.Get("table")
	if schema != "" && table != "" {
		if active := findTable(tables, schema, table); active != nil {
			v.Active = active
			if sort := q.Get("sort"); sort != "" {
				v.Order = &db.Order{Column: sort, Desc: q.Get("desc") == "true"}
			}
			if v.Tab == "data" {
				rows, err := s.read(ctx, *active, pageOf(q), v.Order)
				if err != nil {
					v.DataErr = err.Error()
				} else {
					v.Rows = rows
				}
				v.LinkOptions = s.linkOptions(ctx, *active)
			}
		}
	}

	render(w, r, hxComponent(r, pages.StudioBody(v), pages.Studio(v)))
}

// linkOptions fetches picker candidates for each single-link column of an
// editable table. Returns nil when editing isn't available.
func (s *server) linkOptions(ctx context.Context, t db.Table) map[string][]db.Option {
	if !s.aqlLive() || t.Type == "" {
		return nil
	}
	opts := map[string][]db.Option{}
	for _, c := range t.Columns {
		if !c.IsLink || c.ForeignRef == "" {
			continue // covers both single and multi links
		}
		cands, err := s.aql.Candidates(ctx, c.ForeignRef)
		if err != nil {
			continue // a link picker failing shouldn't break the grid
		}
		opts[c.Name] = cands
	}
	return opts
}

// tables lists tables from the schema when AQL is live, else from the store.
func (s *server) tables(ctx context.Context) ([]db.Table, error) {
	if s.aqlLive() {
		return s.schema.Tables(), nil
	}
	return s.store.Tables(ctx)
}

// read fetches a page of rows via AQL when live, else via the store.
func (s *server) read(ctx context.Context, t db.Table, page int, order *db.Order) (db.Rows, error) {
	offset := (page - 1) * defaultPageSze
	if s.aqlLive() && t.Type != "" {
		return s.aql.Read(ctx, t, defaultPageSze, offset, order)
	}
	return s.store.Read(ctx, t.Schema, t.Name, defaultPageSze, offset, order)
}

// handleSQL runs the raw-SQL console.
func (s *server) handleSQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := s.store.Query(r.Context(), r.FormValue("sql"))
	render(w, r, pages.QueryResultFragment(result))
}

// handleAQL runs the AQL console (executes live, compiles otherwise).
func (s *server) handleAQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var result db.QueryResult
	if s.aql == nil {
		result = db.QueryResult{Err: "no ASL schema loaded — start the studio with -schema <file.asl>"}
	} else {
		r.ParseForm()
		params := make(map[string]any)
		for k, v := range r.Form {
			if len(k) > 6 && k[:6] == "param_" && len(v) > 0 {
				params[k[6:]] = v[0]
			}
		}
		result = s.aql.Exec(r.Context(), r.FormValue("aql"), params)
	}
	render(w, r, pages.QueryResultFragment(result))
}

// handleMutate applies a batch of staged row edits (insert/update/delete) as AQL.
func (s *server) handleMutate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !s.aqlLive() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "editing needs a live database with a schema"})
		return
	}
	var muts []db.Mutation
	if err := json.NewDecoder(r.Body).Decode(&muts); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad request: " + err.Error()})
		return
	}
	res := s.aql.Mutate(r.Context(), muts)
	status := http.StatusOK
	if res.Err != "" {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{"applied": res.Applied, "error": res.Err})
}

type CompletionRequest struct {
	Text string `json:"text"`
	Line int    `json:"line"`
	Char int    `json:"char"`
}

func (s *server) handleCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	
	if s.schema == nil || s.schema.IR == nil {
		writeJSON(w, http.StatusOK, []lsp.CompletionItem{})
		return
	}

	offset := lsp.PositionToOffset(req.Text, lsp.Position{Line: req.Line, Char: req.Char})
	items := lsp.QueryCompletion(req.Text, offset, s.schema.IR)
	if items == nil {
		items = []lsp.CompletionItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

type HoverRequest struct {
	Text string `json:"text"`
	Line int    `json:"line"`
	Char int    `json:"char"`
}

type DiagnosticsRequest struct {
	Text string `json:"text"`
}

func (s *server) handleHover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req HoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	
	if s.schema == nil || s.schema.IR == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	offset := lsp.PositionToOffset(req.Text, lsp.Position{Line: req.Line, Char: req.Char})
	hover := lsp.QueryHover(req.Text, offset, s.schema.IR)
	writeJSON(w, http.StatusOK, hover)
}

func (s *server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DiagnosticsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	
	if s.schema == nil || s.schema.IR == nil {
		writeJSON(w, http.StatusOK, []lsp.Diagnostic{})
		return
	}

	diags := lsp.QueryDiagnostics(req.Text, s.schema.IR)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	writeJSON(w, http.StatusOK, diags)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// poolOf returns the live pgx pool behind a store, or nil.
func poolOf(store db.Store) *pgxpool.Pool {
	if pg, ok := store.(*db.Postgres); ok {
		return pg.Pool()
	}
	return nil
}

func hxComponent(r *http.Request, partial, full templ.Component) templ.Component {
	if r.Header.Get("HX-Request") == "true" {
		return partial
	}
	return full
}

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}

func findTable(tables []db.Table, schema, name string) *db.Table {
	for i := range tables {
		if tables[i].Schema == schema && tables[i].Name == name {
			return &tables[i]
		}
	}
	return nil
}

func pageOf(q interface{ Get(string) string }) int {
	if p := atoiDefault(q.Get("page"), 1); p >= 1 {
		return p
	}
	return 1
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}
