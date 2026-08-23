package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	glspserver "github.com/tliron/glsp/server"
	"gopkg.in/yaml.v3"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	corelsp "github.com/struckchure/axel/core/lsp"
)

const lspName = "axel"

// lspServer holds the open-document store and the cached workspace schema. The
// schema may be split across several .asl files (schema-path can be a glob), in
// which case schemaFiles lists them all and schema is resolved from their merge.
// schemaURI/schemaText still point at the first of them, which is what
// go-to-definition can currently jump into.
type lspServer struct {
	mu          sync.RWMutex
	docs        map[protocol.DocumentUri]string
	schema      *asl.SchemaIR
	schemaURI   protocol.DocumentUri
	schemaText  string
	schemaFiles []protocol.DocumentUri
	schemaSpec  string // the configured schema-path, re-expanded when a new .asl appears
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run the Axel language server (LSP over stdio)",
	Long: `Start the Axel language server. It speaks the Language Server Protocol over
stdio and provides diagnostics, document symbols, hover, go-to-definition, and
completion for .asl schemas and .aql queries. Editors launch it as "axel lsp".`,
	// A language server must not touch the database or write to stdout, so skip
	// the root PersistentPreRun (which builds a migration manager and can print +
	// os.Exit(1) on failure). stdout is reserved for the LSP JSON-RPC stream.
	PersistentPreRun: func(_ *cobra.Command, _ []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {
		s := &lspServer{docs: map[protocol.DocumentUri]string{}}
		handler := s.handler()
		srv := glspserver.NewServer(handler, lspName, false)
		return srv.RunStdio()
	},
}

func init() {
	// LSP clients (e.g. vscode-languageclient) launch the server as
	// `axel lsp --stdio`. Accept that flag as a no-op — stdio is the only
	// transport — and tolerate any other flags an editor may append.
	lspCmd.Flags().Bool("stdio", true, "communicate over stdio (default; accepted for LSP-client compatibility)")
	lspCmd.FParseErrWhitelist.UnknownFlags = true
	RootCmd.AddCommand(lspCmd)
}

func (s *lspServer) handler() *protocol.Handler {
	h := &protocol.Handler{}
	h.Initialize = s.initialize
	h.Initialized = func(ctx *glsp.Context, params *protocol.InitializedParams) error { return nil }
	h.Shutdown = func(ctx *glsp.Context) error { return nil }
	h.SetTrace = func(ctx *glsp.Context, params *protocol.SetTraceParams) error { return nil }
	h.TextDocumentDidOpen = s.didOpen
	h.TextDocumentDidChange = s.didChange
	h.TextDocumentDidClose = s.didClose
	h.TextDocumentDocumentSymbol = s.documentSymbol
	h.TextDocumentHover = s.hover
	h.TextDocumentDefinition = s.definition
	h.TextDocumentCompletion = s.completion
	h.TextDocumentFormatting = s.formatting
	return h
}

// ─────────────────────────────────────────────────────────────
// Lifecycle
// ─────────────────────────────────────────────────────────────

func (s *lspServer) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	if root := rootPath(params); root != "" {
		s.loadWorkspaceSchema(root)
	}

	syncFull := protocol.TextDocumentSyncKindFull
	capabilities := protocol.ServerCapabilities{
		TextDocumentSync:           syncFull,
		HoverProvider:              true,
		DefinitionProvider:         true,
		DocumentSymbolProvider:     true,
		DocumentFormattingProvider: true,
		CompletionProvider: &protocol.CompletionOptions{
			TriggerCharacters: []string{".", "{", "$", "<"},
		},
	}
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo:   &protocol.InitializeResultServerInfo{Name: lspName},
	}, nil
}

// ─────────────────────────────────────────────────────────────
// Document sync
// ─────────────────────────────────────────────────────────────

func (s *lspServer) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.setDoc(params.TextDocument.URI, params.TextDocument.Text)
	s.refresh(ctx, params.TextDocument.URI)
	return nil
}

func (s *lspServer) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	var text string
	for _, ch := range params.ContentChanges {
		switch c := ch.(type) {
		case protocol.TextDocumentContentChangeEventWhole:
			text = c.Text
		case protocol.TextDocumentContentChangeEvent:
			text = c.Text
		}
	}
	s.setDoc(params.TextDocument.URI, text)
	s.refresh(ctx, params.TextDocument.URI)
	return nil
}

func (s *lspServer) didClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.mu.Lock()
	delete(s.docs, params.TextDocument.URI)
	s.mu.Unlock()
	// Clear diagnostics for the closed document.
	ctx.Notify(string(protocol.ServerTextDocumentPublishDiagnostics), protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	return nil
}

func (s *lspServer) setDoc(uri protocol.DocumentUri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = text
	if !strings.HasSuffix(uriToPath(uri), ".asl") {
		return
	}
	// A file created after the schema-path glob was expanded (a new model in
	// schema/) joins the set the moment it is opened.
	if !s.isSchemaFile(uri) && s.schemaSpec != "" {
		s.expandSchemaSpec(s.schemaSpec)
	}
	// If this is (part of) the workspace schema, re-resolve the whole set so
	// diagnostics, completion and go-to-definition see every file.
	if s.isSchemaFile(uri) {
		if uri == s.schemaURI {
			s.schemaText = text
		}
		s.resolveWorkspaceSchema()
		return
	}
	// Fallback: with no configured schema, adopt the .asl files sitting next to
	// the first one opened — a split schema works without an axel.yaml.
	if s.schemaURI == "" {
		if ir := resolveSchema(text); ir == nil {
			return
		}
		s.schemaURI = uri
		s.schemaText = text
		s.schemaFiles = []protocol.DocumentUri{uri}
		if dir := filepath.Dir(uriToPath(uri)); dir != "" {
			s.schemaSpec = dir
			s.expandSchemaSpec(dir)
		}
		s.resolveWorkspaceSchema()
	}
}

// expandSchemaSpec re-expands a schema-path spec (file, directory or glob) into
// the current file set, keeping schemaURI — the file go-to-definition falls back
// to — pointing at the same document when it is still part of it. Callers must
// hold s.mu for writing.
func (s *lspServer) expandSchemaSpec(spec string) {
	paths, err := asl.ExpandPaths(spec)
	if err != nil {
		return
	}
	uris := make([]protocol.DocumentUri, 0, len(paths))
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		uris = append(uris, protocol.DocumentUri("file://"+abs))
	}
	s.schemaFiles = uris
	for _, u := range uris {
		if u == s.schemaURI {
			return
		}
	}
	s.schemaURI = uris[0]
}

// isSchemaFile reports whether uri is one of the workspace schema's files.
// Callers must hold s.mu.
func (s *lspServer) isSchemaFile(uri protocol.DocumentUri) bool {
	for _, u := range s.schemaFiles {
		if u == uri {
			return true
		}
	}
	return uri == s.schemaURI
}

// schemaSource returns the current text of a schema file: the open editor buffer
// when there is one, the file on disk otherwise. Callers must hold s.mu.
func (s *lspServer) schemaSource(uri protocol.DocumentUri) (string, bool) {
	if text, ok := s.docs[uri]; ok {
		return text, true
	}
	data, err := os.ReadFile(uriToPath(uri))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// resolveWorkspaceSchema re-resolves the schema from the merge of every file in
// schemaFiles. Callers must hold s.mu for writing.
func (s *lspServer) resolveWorkspaceSchema() {
	var parsed []*asl.SourceFile
	for _, uri := range s.schemaFiles {
		text, ok := s.schemaSource(uri)
		if !ok {
			continue
		}
		sf, err := asl.ParseNamed(uriToPath(uri), []byte(text))
		if err != nil {
			// One unparseable file leaves the previous schema in place rather
			// than blanking completion/hover across the workspace mid-keystroke.
			return
		}
		parsed = append(parsed, sf)
	}
	ir, err := (&asl.Resolver{}).Resolve(asl.Merge(parsed...))
	if err != nil {
		return
	}
	s.schema = ir
}

// schemaFileSet returns the files of the workspace schema, with their current
// text. `skip` is omitted (pass "" to keep them all) — diagnostics want the
// siblings of the document being checked, while go-to-definition wants every
// file, including the one it started from.
func (s *lspServer) schemaFileSet(skip protocol.DocumentUri) []corelsp.SchemaFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var files []corelsp.SchemaFile
	for _, u := range s.schemaFiles {
		if u == skip {
			continue
		}
		if text, ok := s.schemaSource(u); ok {
			files = append(files, corelsp.SchemaFile{Path: uriToPath(u), URI: string(u), Text: text})
		}
	}
	return files
}

// workspaceSchema returns the schema resolved from every file, and whether uri
// is one of them. A document outside the set (no axel.yaml, a scratch file) is
// resolved on its own instead.
func (s *lspServer) workspaceSchema(uri protocol.DocumentUri) (*asl.SchemaIR, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.schema, s.isSchemaFile(uri)
}

// refresh recomputes and publishes diagnostics for uri, and — if uri is part of
// the schema — for every open .aql document too.
func (s *lspServer) refresh(ctx *glsp.Context, uri protocol.DocumentUri) {
	s.publish(ctx, uri)

	s.mu.RLock()
	var uris []protocol.DocumentUri
	if s.isSchemaFile(uri) {
		// Every open query is compiled against the schema, and every sibling
		// schema file shares its namespace — so a change here can resolve or
		// introduce a problem in any of them.
		uris = make([]protocol.DocumentUri, 0, len(s.docs))
		for u := range s.docs {
			if u == uri {
				continue
			}
			if strings.HasSuffix(uriToPath(u), ".aql") || s.isSchemaFile(u) {
				uris = append(uris, u)
			}
		}
	}
	s.mu.RUnlock()

	for _, u := range uris {
		s.publish(ctx, u)
	}
}

// recoverLog turns a handler panic into a stderr log line instead of a process
// crash — a bad/in-progress document must degrade one request, not kill the
// server. It must never write to stdout (that is the LSP JSON-RPC stream).
func recoverLog(where string) {
	if r := recover(); r != nil {
		fmt.Fprintf(os.Stderr, "axel lsp: recovered panic in %s: %v\n", where, r)
	}
}

func (s *lspServer) publish(ctx *glsp.Context, uri protocol.DocumentUri) {
	defer recoverLog("publish")
	s.mu.RLock()
	text, ok := s.docs[uri]
	schema := s.schema
	s.mu.RUnlock()
	if !ok {
		return
	}
	var diags []corelsp.Diagnostic
	switch {
	case strings.HasSuffix(uriToPath(uri), ".asl"):
		diags = corelsp.SchemaDiagnosticsIn(uriToPath(uri), text, s.schemaFileSet(uri))
	case strings.HasSuffix(uriToPath(uri), ".aql"):
		diags = corelsp.QueryDiagnostics(text, schema)
	}
	ctx.Notify(string(protocol.ServerTextDocumentPublishDiagnostics), protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: toProtocolDiagnostics(diags),
	})
}

// ─────────────────────────────────────────────────────────────
// Language features
// ─────────────────────────────────────────────────────────────

func (s *lspServer) documentSymbol(ctx *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	defer recoverLog("documentSymbol")
	text, ok := s.getDoc(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	var syms []corelsp.Symbol
	if strings.HasSuffix(uriToPath(params.TextDocument.URI), ".asl") {
		syms = corelsp.SchemaSymbols(text)
	} else {
		syms = corelsp.QuerySymbols(text)
	}
	return toProtocolSymbols(syms), nil
}

func (s *lspServer) hover(ctx *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	defer recoverLog("hover")
	text, ok := s.getDoc(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	offset := corelsp.PositionToOffset(text, toCorePosition(params.Position))
	schema, inWorkspace := s.workspaceSchema(params.TextDocument.URI)

	var h *corelsp.Hover
	if strings.HasSuffix(uriToPath(params.TextDocument.URI), ".asl") {
		h = corelsp.SchemaHover(text, offset, schemaFor(schema, inWorkspace, text))
	} else {
		h = corelsp.QueryHover(text, offset, schema)
	}
	if h == nil {
		return nil, nil
	}
	rng := toProtocolRange(h.Range)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: h.Contents},
		Range:    &rng,
	}, nil
}

func (s *lspServer) definition(ctx *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	defer recoverLog("definition")
	uri := params.TextDocument.URI
	text, ok := s.getDoc(uri)
	if !ok {
		return nil, nil
	}
	offset := corelsp.PositionToOffset(text, toCorePosition(params.Position))

	var loc *corelsp.Location
	if strings.HasSuffix(uriToPath(uri), ".asl") {
		// Siblings only: the current document is searched first, from its live
		// buffer text rather than from disk.
		loc = corelsp.SchemaDefinitionIn(text, offset, s.schemaFileSet(uri))
		if loc != nil && loc.URI == "" {
			loc.URI = string(uri) // same-document reference
		}
	} else {
		schema, _ := s.workspaceSchema(uri)
		loc = corelsp.QueryDefinitionIn(text, offset, schema, s.schemaFileSet(""))
		if loc != nil && loc.URI == "" {
			loc.URI = string(uri) // same-document reference (e.g. var block, with binding)
		}
	}
	if loc == nil {
		return nil, nil
	}
	return protocol.Location{URI: protocol.DocumentUri(loc.URI), Range: toProtocolRange(loc.Range)}, nil
}

// formatting reformats the whole document, returning a single full-range edit.
// A parse error or an already-formatted document yields no edits.
func (s *lspServer) formatting(ctx *glsp.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	defer recoverLog("formatting")
	uri := params.TextDocument.URI
	text, ok := s.getDoc(uri)
	if !ok {
		return nil, nil
	}
	var out string
	var err error
	switch {
	case strings.HasSuffix(uriToPath(uri), ".asl"):
		out, err = asl.Format([]byte(text))
	case strings.HasSuffix(uriToPath(uri), ".aql"):
		out, err = aql.Format([]byte(text))
	default:
		return nil, nil
	}
	if err != nil || out == text {
		return nil, nil
	}
	end := corelsp.OffsetToPosition(text, len(text))
	return []protocol.TextEdit{{
		Range:   toProtocolRange(corelsp.Range{Start: corelsp.Position{Line: 0, Char: 0}, End: end}),
		NewText: out,
	}}, nil
}

func (s *lspServer) completion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	defer recoverLog("completion")
	uri := params.TextDocument.URI
	text, ok := s.getDoc(uri)
	if !ok {
		return nil, nil
	}
	offset := corelsp.PositionToOffset(text, toCorePosition(params.Position))
	schema, inWorkspace := s.workspaceSchema(uri)

	var items []corelsp.CompletionItem
	if strings.HasSuffix(uriToPath(uri), ".asl") {
		items = corelsp.SchemaCompletion(text, offset, schemaFor(schema, inWorkspace, text))
	} else {
		items = corelsp.QueryCompletion(text, offset, schema)
	}
	return toProtocolCompletion(items), nil
}

func (s *lspServer) getDoc(uri protocol.DocumentUri) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.docs[uri]
	return t, ok
}

// ─────────────────────────────────────────────────────────────
// Workspace schema discovery
// ─────────────────────────────────────────────────────────────

func (s *lspServer) loadWorkspaceSchema(root string) {
	cfgPath := filepath.Join(root, "axel.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	var cfg struct {
		SchemaPath string `yaml:"schema-path"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil || cfg.SchemaPath == "" {
		return
	}
	// schema-path may be written relative to the workspace root, to the axel.yaml
	// directory, or to the process CWD — try each and use the first that exists.
	var candidates []string
	if filepath.IsAbs(cfg.SchemaPath) {
		candidates = []string{cfg.SchemaPath}
	} else {
		candidates = []string{
			filepath.Join(root, cfg.SchemaPath),
			filepath.Join(filepath.Dir(cfgPath), cfg.SchemaPath),
			cfg.SchemaPath,
		}
	}
	for _, spec := range candidates {
		// spec may be a single file, a directory, or a glob matching several.
		if _, err := asl.ExpandPaths(spec); err != nil {
			continue
		}
		s.mu.Lock()
		s.schemaSpec = spec
		s.schemaURI = ""
		s.expandSchemaSpec(spec)
		if text, ok := s.schemaSource(s.schemaURI); ok {
			s.schemaText = text
		}
		s.resolveWorkspaceSchema()
		s.mu.Unlock()
		return
	}
}

// schemaFor picks the schema a .asl document should be analysed against: the
// merged workspace schema when the document is part of it (so a type declared in
// a sibling file is known), and the document alone otherwise.
func schemaFor(workspace *asl.SchemaIR, inWorkspace bool, text string) *asl.SchemaIR {
	if inWorkspace && workspace != nil {
		return workspace
	}
	return resolveSchema(text)
}

func resolveSchema(text string) *asl.SchemaIR {
	sf, err := asl.Parse([]byte(text))
	if err != nil {
		return nil
	}
	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		return nil
	}
	return ir
}

func rootPath(params *protocol.InitializeParams) string {
	if params.RootURI != nil {
		if p := uriToPath(protocol.DocumentUri(*params.RootURI)); p != "" {
			return p
		}
	}
	if params.RootPath != nil {
		return *params.RootPath
	}
	return ""
}

func uriToPath(uri protocol.DocumentUri) string {
	s := string(uri)
	if strings.HasPrefix(s, "file://") {
		return strings.TrimPrefix(s, "file://")
	}
	return s
}
