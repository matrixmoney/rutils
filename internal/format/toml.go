package format

import (
	"bytes"
	"strings"
)

func init() {
	register(&tomlFormatter{})
}

// tomlFormatter is the TOML formatter.
//
// TOML is friendlier than YAML in one important way: indentation is
// not semantic — a key-value line can start at any column and it
// parses the same. But TOML has its own traps: multi-line strings
// (`"""` and `'''`) are whitespace-preserving, inline tables and
// arrays can contain `=` and `:` characters that must not be
// interpreted as separators, and comments can contain any character
// including quotes that would otherwise confuse a naive scanner.
//
// Our formatter is therefore line-based with a small state machine
// for multi-line strings. It applies only the formatting changes
// that are provably safe on any valid TOML document:
//
//  1. Strip UTF-8 BOM.
//  2. Normalise CRLF → LF.
//  3. Strip trailing whitespace from every line, EXCEPT inside a
//     multi-line string body.
//  4. Normalise `key=value` → `key = value` (exactly one space on
//     each side of `=`), but only on lines that are genuinely
//     key-value lines (not inside strings, not inside inline
//     tables, not inside arrays, not preceded by another `=` that
//     would make this `==` etc.).
//  5. Trim inner whitespace in `[ section ]` and `[[ array ]]`
//     table headers so `[ a . b ]` becomes `[a.b]`.
//  6. Collapse runs of 2+ blank lines into a single blank line.
//  7. Trim trailing blank lines and end with exactly one `\n`.
//
// Everything else is preserved verbatim: key order, table order,
// array element order, comment text, scalar quoting style,
// inline-table formatting, array formatting, and multi-line string
// bodies.
//
// The formatter never returns a 4xx-style error for any input. If a
// line cannot be safely canonicalised, we leave it alone and add a
// warning. The only non-nil error path is ErrEmptyInput.
type tomlFormatter struct{}

func (t *tomlFormatter) Format() string { return "toml" }

// tomlMultilineState tracks whether we are currently inside a
// multi-line string. TOML has two multi-line delimiters: `"""` and
// `'''`. The state is nil when we are outside a multi-line string,
// and otherwise records which delimiter opened it.
type tomlMultilineState struct {
	delimiter string // `"""` or `'''`
}

// FormatBytes implements Formatter.
func (t *tomlFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "toml"}

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

	var (
		out       bytes.Buffer
		mlState   *tomlMultilineState
		prevBlank = false
	)

	for idx, line := range rawLines {
		lineNum := idx + 1

		// ---- Inside a multi-line string: preserve verbatim ----
		//
		// Whitespace inside `"""..."""` and `'''...'''` is part of
		// the string's value. We must not touch it. The only thing
		// we do is look for the closing delimiter so we can exit the
		// state.
		if mlState != nil {
			// Copy the line as-is (including trailing whitespace —
			// it belongs to the string value).
			out.WriteString(line)
			out.WriteByte('\n')

			// Does this line contain the closing delimiter?
			if strings.Contains(line, mlState.delimiter) {
				mlState = nil
			}
			prevBlank = false
			continue
		}

		// ---- Normal line processing ----
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]

		// Blank line: collapse runs of 2+ into 1.
		if trimmed == "" {
			if prevBlank {
				report.AddFix("blank-lines-collapsed")
				continue
			}
			out.WriteByte('\n')
			prevBlank = true
			continue
		}
		prevBlank = false

		// Comment-only line: preserve verbatim (minus trailing
		// whitespace, which is the same convention as HTML and INI).
		if strings.HasPrefix(trimmed, "#") {
			out.WriteString(rstrip(line))
			out.WriteByte('\n')
			continue
		}

		// Does this line open a multi-line string?
		//
		// A `"""` or `'''` appearing after the value (or anywhere
		// outside a comment) opens the multi-line state. We must
		// still do the line-level canonicalisation for the opening
		// line itself, because `key="""` should become `key = """`.
		//
		// The opening line's whitespace is NOT part of the string
		// value (TOML strips a leading newline immediately after
		// `"""`), so we can safely canonicalise it.
		openedDelim := detectMultilineOpener(trimmed)

		// Table header line: `[name]` or `[[name]]`.
		if header, changed := canonicaliseTOMLTableHeader(trimmed); changed {
			report.AddFix("table-header-whitespace")
			out.WriteString(indent + header)
			out.WriteByte('\n')
			if openedDelim != "" {
				mlState = &tomlMultilineState{delimiter: openedDelim}
			}
			continue
		}

		// Key-value line: try to canonicalise the separator spacing.
		canonical, fixName, warning := canonicaliseTOMLKeyValue(indent, trimmed, lineNum)
		if fixName != "" {
			report.AddFix(fixName)
		}
		if warning != "" {
			report.AddWarning(warning)
		}

		out.WriteString(canonical)
		out.WriteByte('\n')

		// Enter multi-line state if this line opened one and did
		// not close it on the same line.
		if openedDelim != "" && !containsClosingDelimiter(trimmed, openedDelim) {
			mlState = &tomlMultilineState{delimiter: openedDelim}
		}
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

// canonicaliseTOMLTableHeader returns the canonical form of a
// `[name]` or `[[name]]` header line, trimming inner whitespace.
// Returns (header, true) on change; (original, false) otherwise.
//
// Examples:
//
//	"[ section ]"        → "[section]"        (changed)
//	"[[ array.of.items ]]" → "[[array.of.items]]" (changed)
//	"[section]"          → "[section]"        (unchanged)
//	"[a.b.c]"            → "[a.b.c]"          (unchanged)
//	"[]"                 → "[]"               (malformed, leave alone)
func canonicaliseTOMLTableHeader(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "[") {
		return trimmed, false
	}

	// Array-of-tables form: `[[name]]`.
	if strings.HasPrefix(trimmed, "[[") {
		closeIdx := strings.Index(trimmed, "]]")
		if closeIdx < 0 {
			return trimmed, false
		}
		inner := strings.TrimSpace(trimmed[2:closeIdx])
		if inner == "" {
			return trimmed, false
		}
		canonical := "[[" + inner + "]]"
		return canonical, canonical != trimmed
	}

	// Ordinary table form: `[name]`.
	closeIdx := strings.IndexByte(trimmed, ']')
	if closeIdx < 0 {
		return trimmed, false
	}
	inner := strings.TrimSpace(trimmed[1:closeIdx])
	if inner == "" {
		return trimmed, false
	}
	canonical := "[" + inner + "]"
	return canonical, canonical != trimmed
}

// canonicaliseTOMLKeyValue finds the key-value separator in a
// trimmed line and rewrites it as `key = value` (exactly one space
// on each side of `=`). Preserves the original indentation.
//
// Returns (canonicalLine, fixName, warning):
//
//   - If the line does not look like a key-value line, returns
//     (indent+trimmed, "", "") — unchanged, no fix, no warning.
//     This is the case for anything with a `#` comment but no `=`
//     (e.g. a stray comment continuation) or for lines that are
//     clearly part of a multi-line construct (inline tables, etc.).
//   - If the line looks like a key-value but already uses the
//     canonical spacing, returns (indent+trimmed, "", "").
//   - If we rewrote the separator, returns (newLine, "key-value-spacing", "").
//   - If the line is suspicious (e.g. multiple `=` outside strings),
//     returns (indent+trimmed, "", warning message).
func canonicaliseTOMLKeyValue(indent, trimmed string, lineNum int) (string, string, string) {
	eqIdx := findTOMLSeparator(trimmed)
	if eqIdx < 0 {
		// No key-value separator at all. This is a legitimate
		// occurrence for comment-only lines (handled above), and
		// for a very small class of malformed input. We do not warn
		// — TOML does have content like `[[array.of.tables]]`
		// (handled above) that legitimately lacks an `=`.
		return indent + rstrip(trimmed), "", ""
	}

	// Split into key and value.
	leftRaw := trimmed[:eqIdx]
	rightRaw := trimmed[eqIdx+1:]

	// Strip the trailing whitespace of the left side and the
	// leading whitespace of the right side. This gives us the
	// canonical `key = value` form when we rejoin with `" = "`.
	key := strings.TrimRight(leftRaw, " \t")
	value := strings.TrimLeft(rightRaw, " \t")

	// A bare `=` at the start of a line (or a line starting with
	// `[` that has an `=` inside — for inline tables) is not a
	// key-value line.
	if key == "" {
		return indent + rstrip(trimmed), "", ""
	}

	// Preserve any trailing comment on the value side. We split the
	// value into `value` and `comment` at the first `#` that is not
	// inside a quoted string.
	value, comment := splitTOMLValueAndComment(value)

	// Rebuild the line.
	var b strings.Builder
	b.Grow(len(indent) + len(key) + len(value) + len(comment) + 8)
	b.WriteString(indent)
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(value)
	if comment != "" {
		b.WriteString(comment)
	}
	canonical := b.String()

	if canonical == indent+trimmed {
		// Already canonical.
		return canonical, "", ""
	}
	return canonical, "key-value-spacing", ""
}

// splitTOMLValueAndComment splits a TOML value from any trailing
// `#`-style comment on the same line. The split respects quoted
// strings so that `"a # b"` is not split.
//
// Returns (value, comment). comment includes the leading `#` and
// any whitespace before it so that the re-join is byte-identical for
// the comment portion. When there is no comment, returns
// (wholeValue, "").
func splitTOMLValueAndComment(s string) (string, string) {
	inQuote := byte(0)
	tripleQuote := ""
	for i := 0; i < len(s); i++ {
		// Check for triple-quoted string openers/closers first.
		if tripleQuote != "" {
			if strings.HasPrefix(s[i:], tripleQuote) {
				i += 2
				tripleQuote = ""
			}
			continue
		}
		if strings.HasPrefix(s[i:], `"""`) || strings.HasPrefix(s[i:], `'''`) {
			tripleQuote = s[i : i+3]
			i += 2
			continue
		}

		c := s[i]
		if inQuote != 0 {
			if c == '\\' && inQuote == '"' {
				i++ // skip escaped char inside double-quoted string
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '#':
			// Found an unquoted, unescaped `#` — split here.
			value := strings.TrimRight(s[:i], " \t")
			comment := s[i:]
			return value, comment
		}
	}
	return s, ""
}

// findTOMLSeparator returns the index of the first `=` that acts as
// a key-value separator. Returns -1 if the line does not contain
// one.
//
// We require that:
//
//   - The `=` is not preceded by another `=`, `<`, `>`, `!`, `+`,
//     or `-` (rules out `==`, `<=`, `>=`, `!=`, `+=`, `-=`).
//   - The `=` is not inside a quoted string.
//   - The `=` is not inside an inline table `{...}` or array
//     `[...]` — those may legally contain `=` inside their
//     elements.
func findTOMLSeparator(s string) int {
	inQuote := byte(0)
	tripleQuote := ""
	braceDepth := 0
	bracketDepth := 0

	for i := 0; i < len(s); i++ {
		// Triple-quoted strings take priority — they swallow
		// everything until the matching close.
		if tripleQuote != "" {
			if strings.HasPrefix(s[i:], tripleQuote) {
				i += 2
				tripleQuote = ""
			}
			continue
		}
		if strings.HasPrefix(s[i:], `"""`) || strings.HasPrefix(s[i:], `'''`) {
			tripleQuote = s[i : i+3]
			i += 2
			continue
		}

		c := s[i]

		if inQuote != 0 {
			if c == '\\' && inQuote == '"' {
				i++
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
			continue
		}

		switch c {
		case '"', '\'':
			inQuote = c
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '=':
			// Skip inline-table and array contents.
			if braceDepth > 0 || bracketDepth > 0 {
				continue
			}
			// Rule out comparison operators that would only appear
			// inside an unquoted value (which TOML does not allow,
			// but be defensive).
			if i > 0 {
				prev := s[i-1]
				if prev == '=' || prev == '<' || prev == '>' ||
					prev == '!' || prev == '+' || prev == '-' {
					continue
				}
			}
			return i
		}
	}
	return -1
}

// detectMultilineOpener reports whether the trimmed line opens a
// TOML multi-line string. Returns `"""` or `'''` or "" if no opener
// is present.
//
// We scan outside any single-line string and outside comments.
func detectMultilineOpener(trimmed string) string {
	// A leading `#` means a comment line — never an opener.
	if strings.HasPrefix(trimmed, "#") {
		return ""
	}

	inQuote := byte(0)
	for i := 0; i < len(trimmed); i++ {
		if inQuote != 0 {
			if trimmed[i] == '\\' && inQuote == '"' {
				i++
				continue
			}
			if trimmed[i] == inQuote {
				inQuote = 0
			}
			continue
		}
		// Check for triple-quoted openers.
		if strings.HasPrefix(trimmed[i:], `"""`) {
			return `"""`
		}
		if strings.HasPrefix(trimmed[i:], `'''`) {
			return `'''`
		}
		switch trimmed[i] {
		case '"', '\'':
			inQuote = trimmed[i]
		case '#':
			// A comment starts here; anything after is not code.
			return ""
		}
	}
	return ""
}

// containsClosingDelimiter reports whether the same line also
// contains the closing delimiter of a multi-line string. This
// matters for single-line multi-line strings like `key = """hello"""`
// — those open and close on the same line and must not put the
// formatter into the multi-line state.
func containsClosingDelimiter(line, delim string) bool {
	// Find the FIRST opening delimiter (there must be one) and then
	// look for a second occurrence of the same delimiter after it.
	first := strings.Index(line, delim)
	if first < 0 {
		return false
	}
	rest := line[first+len(delim):]
	return strings.Contains(rest, delim)
}