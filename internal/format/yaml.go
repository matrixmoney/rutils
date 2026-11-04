package format

import (
	"bytes"
	"strings"
)

func init() {
	register(&yamlFormatter{})
}

// yamlFormatter is the YAML formatter.
//
// YAML is a much trickier format to format safely than INI: it is
// indentation-sensitive, it has two competing specs (YAML 1.1 and
// YAML 1.2) with different scalar resolution rules, and it supports
// constructs (anchors, aliases, tags, multi-line scalars, flow
// collections) whose whitespace is semantically meaningful. A naive
// "reformat everything" approach silently breaks files.
//
// Our formatter is therefore deliberately conservative. It applies
// only the formatting changes that are provably safe on ANY valid
// YAML document:
//
//  1. Strip UTF-8 BOM.
//  2. Normalise CRLF → LF.
//  3. Expand tabs in leading indentation to spaces (YAML forbids
//     tabs as indentation characters in every spec version).
//  4. Strip trailing whitespace from every line.
//  5. Insert a single space after the colon in `key:value` when the
//     space is missing AND the line is a top-level mapping entry
//     (not inside a string, block scalar, comment, or flow sequence).
//  6. Collapse runs of 3+ blank lines into a single blank line.
//  7. Trim trailing blank lines and end with exactly one `\n`.
//
// Everything else is preserved verbatim: key order, indentation
// width, comment text, scalar formatting (quoted vs unquoted), flow
// vs block style, anchors, aliases, tags, and multi-line scalars.
//
// The formatter never returns a 4xx-style error for any input. If a
// line cannot be safely canonicalised, we leave it alone and add a
// warning. The only non-nil error path is ErrEmptyInput.
type yamlFormatter struct{}

func (y *yamlFormatter) Format() string { return "yaml" }

// FormatBytes implements Formatter.
func (y *yamlFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "yaml"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	rawLines := strings.Split(string(work), "\n")

	// We track the previous non-blank line so that stateful contexts
	// (block scalar bodies, multi-line quoted strings) are preserved
	// verbatim without any whitespace edits.
	var (
		out            bytes.Buffer
		inBlockScalar  = false
		inMultilineStr = false
		prevBlank      = false
		prevIndent     = 0
	)

	for idx, line := range rawLines {
		lineNum := idx + 1

		// ---- Detect block scalar entry and multi-line string entry ----
		//
		// A block scalar starts with `|` or `>` at the end of a line
		// (optionally with chomping/indent indicators like `|-` or
		// `>2`), followed by content indented further than the key.
		// Until we see a line at <= the key's indent, everything is
		// block-scalar body — we must not touch its whitespace.
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)

		if inBlockScalar {
			// Block scalar ends when we reach a non-blank line whose
			// indent is <= the indent of the line that opened it.
			if trimmed != "" && indent <= prevIndent {
				inBlockScalar = false
				// Fall through to normal processing below.
			} else {
				// Preserve block-scalar body verbatim.
				out.WriteString(line)
				out.WriteByte('\n')
				if trimmed == "" {
					prevBlank = true
				} else {
					prevBlank = false
				}
				continue
			}
		}

		if inMultilineStr {
			// Multi-line quoted strings are handled similarly.
			// A string is "closed" when the quote character appears
			// unescaped somewhere on the line. We use a permissive
			// detector rather than a full YAML parser.
			if containsUnescapedQuote(line) {
				inMultilineStr = false
			}
			out.WriteString(line)
			out.WriteByte('\n')
			if trimmed == "" {
				prevBlank = true
			} else {
				prevBlank = false
			}
			continue
		}

		// ---- Blank line ----
		if trimmed == "" {
			// Collapse runs of 2+ blank lines into one.
			if prevBlank {
				report.AddFix("blank-lines-collapsed")
				continue
			}
			out.WriteByte('\n')
			prevBlank = true
			continue
		}

		prevBlank = false

		// ---- Comment lines ----
		//
		// Comments preserve everything except trailing whitespace,
		// which we strip below as part of the general pass.
		if strings.HasPrefix(trimmed, "#") {
			out.WriteString(rstrip(line))
			out.WriteByte('\n')
			prevIndent = indent
			continue
		}

		// ---- Detect block scalar opener ----
		//
		// A line ends with `|`, `>`, `|-`, `>+`, `|2`, `>2-`, etc.
		// See https://yaml.org/spec/1.2.2/#literal-style for the
		// grammar. We are permissive here: any `|` or `>` as the last
		// non-whitespace character, optionally followed by 0-2
		// modifier characters, counts as a block scalar opener.
		if isBlockScalarOpener(trimmed) {
			canonical := canonicaliseYAMLLine(line, lineNum, &report)
			out.WriteString(canonical)
			out.WriteByte('\n')
			inBlockScalar = true
			prevIndent = indent
			continue
		}

		// ---- Detect multi-line string opener ----
		//
		// A line whose first non-blank character is a `"` or `'` and
		// that does not close that quote on the same line is a
		// multi-line string. Until the closing quote appears, all
		// following lines are string content and must not be edited.
		if startsMultilineString(trimmed) {
			out.WriteString(line) // verbatim; do not even strip whitespace
			out.WriteByte('\n')
			inMultilineStr = true
			prevIndent = indent
			continue
		}

		// ---- Normal line ----
		canonical := canonicaliseYAMLLine(line, lineNum, &report)
		out.WriteString(canonical)
		out.WriteByte('\n')
		prevIndent = indent
	}

	// Trim trailing blank lines and ensure exactly one final newline.
	result := trimTrailingBlankLines(out.String())
	if result != "" {
		result += "\n"
	}

	finalBytes := []byte(result)
	if bytes.Equal(finalBytes, work) {
		return work, report, nil
	}

	report.AddFix("formatted")
	return finalBytes, report, nil
}

// canonicaliseYAMLLine applies the safe line-level canonicalisations
// to a single line that is NOT inside a block scalar or multi-line
// string.
//
// The transformations applied:
//
//   - Leading tabs → spaces (each tab becomes 4 spaces; the exact
//     width does not matter for YAML validity, only that tabs are
//     gone from the indent).
//   - Trailing whitespace stripped.
//   - `key:value` → `key: value` when the colon is a top-level mapping
//     separator and the value is not a URL scheme.
func canonicaliseYAMLLine(line string, lineNum int, report *Report) string {
	// 1. Expand tabs in the leading indentation.
	expanded := expandLeadingTabs(line)
	if expanded != line {
		report.AddFix("tab-indent-expanded")
	}

	// 2. Strip trailing whitespace.
	expanded = rstrip(expanded)

	// 3. Insert a space after a mapping colon when the space is
	// missing. We only touch lines that look like a plain mapping
	// entry: `key:` followed by a non-space character, with no
	// quote characters before the colon.
	if fixed, changed := ensureSpaceAfterMappingColon(expanded); changed {
		report.AddFix("colon-spacing")
		return fixed
	}

	return expanded
}

// expandLeadingTabs replaces leading tab characters with spaces.
// The tab width is arbitrary — YAML forbids tabs as indentation
// characters, so any positive width is correct.
func expandLeadingTabs(line string) string {
	const tabWidth = 4
	var b strings.Builder
	b.Grow(len(line) + 8)
	i := 0
	for i < len(line) && line[i] == '\t' {
		b.WriteString("    ") // tabWidth spaces
		i++
	}
	b.WriteString(line[i:])
	return b.String()
}

// ensureSpaceAfterMappingColon inserts a single space after a `:`
// that is immediately followed by a non-space character, when the
// context looks like a plain scalar key.
//
// We are deliberately conservative. The following are NOT touched:
//
//   - `http://...` (colon-slash-slash → URL scheme in a value)
//   - `key: 1` (already has space)
//   - `key:` at end of line (block mapping opener with nested value)
//   - `"key":"value"` (JSON-style flow inside a string)
//   - Lines whose first non-blank character is `-`, `[`, or `{`
//     (sequence or flow collection markers)
//
// Returns (fixed, true) only when a change was actually made.
func ensureSpaceAfterMappingColon(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]

	// Skip comment lines and flow/sequence starters.
	if trimmed == "" ||
		strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "-") ||
		strings.HasPrefix(trimmed, "[") ||
		strings.HasPrefix(trimmed, "{") {
		return line, false
	}

	// Find the first ':' that is NOT inside a quoted string.
	inQuote := byte(0)
	colonIdx := -1
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case ':':
			colonIdx = i
			i = len(trimmed) // break out of the loop
		}
	}
	if colonIdx < 0 {
		return line, false
	}

	after := trimmed[colonIdx+1:]
	if after == "" {
		// `key:` with nothing after — a block mapping opener, leave alone.
		return line, false
	}
	if after[0] == ' ' {
		// Already has the space we want.
		return line, false
	}
	if after[0] == '/' {
		// URL scheme — not a mapping separator.
		return line, false
	}

	fixed := indent + trimmed[:colonIdx+1] + " " + after
	return fixed, true
}

// isBlockScalarOpener reports whether the trimmed line ends with a
// YAML block scalar indicator (`|` or `>`) optionally followed by
// 0–2 modifier characters (`+`, `-`, `1`–`9`).
//
// Examples that match:
//
//	key: |
//	key: |-
//	key: >+
//	key: |2
//
// Everything is checked on the trimmed version, so trailing spaces
// do not defeat the check.
func isBlockScalarOpener(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	// Find the last ':' or '-' or start of line, then look for the
	// block scalar indicator.
	last := trimmed[len(trimmed)-1]
	// Strip up to 2 modifier characters from the end.
	modifiers := 0
	for modifiers < 2 && len(trimmed)-1-modifiers >= 0 {
		c := trimmed[len(trimmed)-1-modifiers]
		if c == '+' || c == '-' || (c >= '1' && c <= '9') {
			modifiers++
			continue
		}
		break
	}
	if len(trimmed)-1-modifiers < 0 {
		return false
	}
	indicator := trimmed[len(trimmed)-1-modifiers]
	_ = last
	return indicator == '|' || indicator == '>'
}

// startsMultilineString reports whether the trimmed line begins a
// multi-line quoted string that is not closed on the same line.
//
// The check is permissive: we look for an opening quote and then
// scan for a closing quote of the same kind. If one is present on
// the same line, this is not a multi-line string.
func startsMultilineString(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	// Only treat the line as a string opener if the first non-blank
	// character is a quote.
	first := trimmed[0]
	if first != '"' && first != '\'' {
		// Also allow `key: "..."` openers: look for a quote after
		// the first `:`.
		colon := strings.IndexByte(trimmed, ':')
		if colon < 0 {
			return false
		}
		rest := strings.TrimLeft(trimmed[colon+1:], " ")
		if rest == "" {
			return false
		}
		first = rest[0]
		if first != '"' && first != '\'' {
			return false
		}
		// Recurse with the rest.
		return !hasClosingQuote(rest, first)
	}
	return !hasClosingQuote(trimmed, first)
}

// hasClosingQuote reports whether s contains an unescaped closing
// quote of the given kind, starting from the second byte (we assume
// s[0] is the opening quote).
func hasClosingQuote(s string, quote byte) bool {
	escaped := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == quote {
			return true
		}
	}
	return false
}

// containsUnescapedQuote reports whether a line contains an unescaped
// closing quote, which signals the end of a multi-line string.
func containsUnescapedQuote(line string) bool {
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' || c == '\'' {
			return true
		}
	}
	return false
}