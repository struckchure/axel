package lsp

import (
	"unicode/utf8"

	"github.com/alecthomas/participle/v2/lexer"
)

// OffsetToPosition converts a byte offset in text into an LSP Position — a
// zero-based line and a UTF-16 code-unit character offset within that line.
func OffsetToPosition(text string, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line, lineStart := 0, 0
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return Position{Line: line, Char: utf16Len(text[lineStart:offset])}
}

// PositionToOffset converts an LSP Position back into a byte offset in text.
func PositionToOffset(text string, pos Position) int {
	// Advance to the start of the target line.
	i, line := 0, 0
	for i < len(text) && line < pos.Line {
		if text[i] == '\n' {
			line++
		}
		i++
	}
	// Advance pos.Char UTF-16 units within the line.
	units := 0
	for i < len(text) && text[i] != '\n' && units < pos.Char {
		r, size := utf8.DecodeRuneInString(text[i:])
		units += utf16RuneLen(r)
		i += size
	}
	return i
}

// LexerPosition maps a participle lexer.Position (byte offset) to an LSP Position.
func LexerPosition(text string, p lexer.Position) Position {
	return OffsetToPosition(text, p.Offset)
}

// nodeRange builds an LSP range spanning a node's [start,end) lexer positions.
func nodeRange(text string, start, end lexer.Position) Range {
	s, e := start.Offset, end.Offset
	if e <= s {
		e = s + 1
	}
	return Range{Start: OffsetToPosition(text, s), End: OffsetToPosition(text, e)}
}

// nameSelection returns the range of a node's name token, located as a whole
// word at or after the node's start offset.
func nameSelection(text string, from lexer.Position, name string) Range {
	idx := indexWord(text, name, from.Offset)
	if idx < 0 {
		idx = from.Offset
	}
	return Range{Start: OffsetToPosition(text, idx), End: OffsetToPosition(text, idx+len(name))}
}

// wordRange returns the range of the identifier-like word covering byte offset,
// or a single-character range if the offset is not on a word.
func wordRange(text string, offset int) Range {
	_, start, end := wordAt(text, offset)
	if end == start {
		end = start + 1
	}
	return Range{Start: OffsetToPosition(text, start), End: OffsetToPosition(text, end)}
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16RuneLen(r)
	}
	return n
}

// utf16RuneLen is the number of UTF-16 code units needed to encode r (2 for
// astral-plane runes that use a surrogate pair, 1 otherwise).
func utf16RuneLen(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}
