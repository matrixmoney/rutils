package format

import (
	"bytes"
	"strings"
)

// text.go — shared helpers for text-based formatters.
//
// Kept dependency-free so this package has no imports beyond the standard
// library plus what each individual formatter file needs.

// stripBOM removes a UTF-8 BOM (EF BB BF) if present, reporting whether
// it was there. Every text-format formatter calls this first so downstream
// logic never has to think about it.
func stripBOM(raw []byte) ([]byte, bool) {
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		return raw[3:], true
	}
	return raw, false
}

// normaliseLineEndings converts CRLF and lone CR to LF. Callers get a
// slice that can be safely split on "\n" afterwards.
func normaliseLineEndings(raw []byte) []byte {
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	raw = bytes.ReplaceAll(raw, []byte("\r"), []byte("\n"))
	return raw
}

// rstrip trims ASCII spaces and tabs from the right end of s. Fast
// enough to call on every line of a large document.
func rstrip(s string) string {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if c != ' ' && c != '\t' {
			break
		}
		i--
	}
	return s[:i]
}

// trimTrailingBlankLines removes trailing empty lines from a joined
// text document. Used by formatters that want to end their output with
// a single newline.
func trimTrailingBlankLines(s string) string {
	s = strings.TrimRight(s, "\n")
	return s
}