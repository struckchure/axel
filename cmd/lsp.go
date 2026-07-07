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

	"github.com/struckchure/axel/core/asl"
	corelsp "github.com/struckchure/axel/core/lsp"
)

const lspName = "axel"

// lspServer holds the open-document store and the cached workspace schema.
type lspServer struct {
	mu         sync.RWMutex
	docs       map[protocol.DocumentUri]string
	schema     *asl.SchemaIR
	schemaURI  protocol.DocumentUri
	schemaText string
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
		TextDocumentSync:       syncFull,
		HoverProvider:          true,
		DefinitionProvider:     true,
		DocumentSymbolProvider: true,
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
	// If this is the workspace schema file, re-resolve so query diagnostics stay fresh.
	if uri == s.schemaURI {
		s.schemaText = text
		s.schema = resolveSchema(text)
		return
	}
	// Fallback: with no configured schema, adopt the first .asl that resolves
	// cleanly as the workspace schema.
	if s.schemaURI == "" && strings.HasSuffix(uriToPath(uri), ".asl") {
		if ir := resolveSchema(text); ir != nil {
			s.schemaURI = uri
			s.schemaText = text
			s.schema = ir
		}
	}
}

// refresh recomputes and publishes diagnostics for uri, and — if uri is the
// schema — for every open .aql document too.
func (s *lspServer) refresh(ctx *glsp.Context, uri protocol.DocumentUri) {
	s.publish(ctx, uri)
	if uri == s.schemaURI {
		s.mu.RLock()
		uris := make([]protocol.DocumentUri, 0, len(s.docs))
		for u := range s.docs {
			if u != uri && strings.HasSuffix(uriToPath(u), ".aql") {
				uris = append(uris, u)
			}
		}
		s.mu.RUnlock()
		for _, u := range uris {
			s.publish(ctx, u)
		}
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
		diags = corelsp.SchemaDiagnostics(text)
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
	s.mu.RLock()
	schema := s.schema
	s.mu.RUnlock()

	var h *corelsp.Hover
	if strings.HasSuffix(uriToPath(params.TextDocument.URI), ".asl") {
		h = corelsp.SchemaHover(text, offset, resolveSchema(text))
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
		loc = corelsp.SchemaDefinition(text, offset)
		if loc != nil && loc.URI == "" {
			loc.URI = string(uri) // same-document reference
		}
	} else {
		s.mu.RLock()
		schema, schemaURI, schemaText := s.schema, s.schemaURI, s.schemaText
		s.mu.RUnlock()
		loc = corelsp.QueryDefinition(text, offset, schema, string(schemaURI), schemaText)
	}
	if loc == nil {
		return nil, nil
	}
	return protocol.Location{URI: protocol.DocumentUri(loc.URI), Range: toProtocolRange(loc.Range)}, nil
}

func (s *lspServer) completion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	defer recoverLog("completion")
	uri := params.TextDocument.URI
	text, ok := s.getDoc(uri)
	if !ok {
		return nil, nil
	}
	offset := corelsp.PositionToOffset(text, toCorePosition(params.Position))
	s.mu.RLock()
	schema := s.schema
	s.mu.RUnlock()

	var items []corelsp.CompletionItem
	if strings.HasSuffix(uriToPath(uri), ".asl") {
		items = corelsp.SchemaCompletion(text, offset, resolveSchema(text))
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
	for _, schemaPath := range candidates {
		schemaText, err := os.ReadFile(schemaPath)
		if err != nil {
			continue
		}
		abs, _ := filepath.Abs(schemaPath)
		s.mu.Lock()
		s.schemaText = string(schemaText)
		s.schema = resolveSchema(string(schemaText))
		s.schemaURI = protocol.DocumentUri("file://" + abs)
		s.mu.Unlock()
		return
	}
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
