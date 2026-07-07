package main

import (
	protocol "github.com/tliron/glsp/protocol_3_16"

	corelsp "github.com/struckchure/axel/core/lsp"
)

// Converters between the editor-agnostic core/lsp types and glsp protocol types.

func toCorePosition(p protocol.Position) corelsp.Position {
	return corelsp.Position{Line: int(p.Line), Char: int(p.Character)}
}

func toProtocolPosition(p corelsp.Position) protocol.Position {
	return protocol.Position{Line: uint32(p.Line), Character: uint32(p.Char)}
}

func toProtocolRange(r corelsp.Range) protocol.Range {
	return protocol.Range{Start: toProtocolPosition(r.Start), End: toProtocolPosition(r.End)}
}

func toProtocolDiagnostics(diags []corelsp.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		sev := protocol.DiagnosticSeverity(d.Severity)
		src := lspName
		out = append(out, protocol.Diagnostic{
			Range:    toProtocolRange(d.Range),
			Severity: &sev,
			Source:   &src,
			Message:  d.Message,
		})
	}
	return out
}

func toProtocolSymbols(syms []corelsp.Symbol) []protocol.DocumentSymbol {
	out := make([]protocol.DocumentSymbol, 0, len(syms))
	for _, s := range syms {
		ds := protocol.DocumentSymbol{
			Name:           s.Name,
			Kind:           protocol.SymbolKind(s.Kind),
			Range:          toProtocolRange(s.Range),
			SelectionRange: toProtocolRange(s.Selection),
		}
		if s.Detail != "" {
			detail := s.Detail
			ds.Detail = &detail
		}
		if len(s.Children) > 0 {
			ds.Children = toProtocolSymbols(s.Children)
		}
		out = append(out, ds)
	}
	return out
}

func toProtocolCompletion(items []corelsp.CompletionItem) *protocol.CompletionList {
	out := make([]protocol.CompletionItem, 0, len(items))
	for _, it := range items {
		kind := protocol.CompletionItemKind(it.Kind)
		ci := protocol.CompletionItem{Label: it.Label, Kind: &kind}
		if it.Detail != "" {
			detail := it.Detail
			ci.Detail = &detail
		}
		out = append(out, ci)
	}
	return &protocol.CompletionList{IsIncomplete: false, Items: out}
}
