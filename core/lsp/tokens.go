package lsp

import "strings"

func isWordByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// wordAt returns the identifier-like word covering byte offset, and its byte
// range [start,end). If the offset is not on a word, start==end==offset (after
// nudging left by one so a cursor just past a word still selects it).
func wordAt(text string, offset int) (word string, start, end int) {
	if offset > len(text) {
		offset = len(text)
	}
	// If sitting just after a word char (cursor at the word's right edge), step back.
	if offset > 0 && offset < len(text) && !isWordByte(text[offset]) && isWordByte(text[offset-1]) {
		offset--
	} else if offset == len(text) && offset > 0 && isWordByte(text[offset-1]) {
		offset--
	}
	if offset >= len(text) || !isWordByte(text[offset]) {
		return "", offset, offset
	}
	start, end = offset, offset
	for start > 0 && isWordByte(text[start-1]) {
		start--
	}
	for end < len(text) && isWordByte(text[end]) {
		end++
	}
	return text[start:end], start, end
}

// prevSignificant returns the last non-whitespace, non-comment byte before
// offset, and its index, or (0, -1) if none.
func prevSignificant(text string, offset int) (byte, int) {
	i := offset - 1
	for i >= 0 {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i--
			continue
		}
		// Skip a trailing line comment segment if this position is within one.
		if lineHasCommentBefore(text, i) {
			// jump to before the '#'
			i = commentStart(text, i) - 1
			continue
		}
		return c, i
	}
	return 0, -1
}

// prevWord returns the identifier immediately before offset (skipping spaces),
// e.g. the keyword before the cursor. Empty if the previous token isn't a word.
func prevWord(text string, offset int) string {
	i := offset - 1
	for i >= 0 && (text[i] == ' ' || text[i] == '\t' || text[i] == '\r' || text[i] == '\n') {
		i--
	}
	end := i + 1
	for i >= 0 && isWordByte(text[i]) {
		i--
	}
	if end == i+1 {
		return ""
	}
	return text[i+1 : end]
}

// lineHasCommentBefore reports whether byte i sits after a '#' on its line.
func lineHasCommentBefore(text string, i int) bool {
	return commentStart(text, i) >= 0
}

// commentStart returns the index of the '#' that starts a comment covering i on
// its line, or -1 if i is not inside a comment.
func commentStart(text string, i int) int {
	lineStart := strings.LastIndexByte(text[:i+1], '\n') + 1
	hash := strings.IndexByte(text[lineStart:i+1], '#')
	if hash < 0 {
		return -1
	}
	return lineStart + hash
}
