package format

import (
	"bytes"
	"encoding/json"
	"strings"
)

func init() {
	register(&jsonFormatter{})
}

// jsonFormatter formats JSON documents using json.Indent.
//
// WHY json.Indent AND NOT Unmarshal+Marshal?
//
// The naive approach вҖ” json.Unmarshal into interface{}, then
// json.MarshalIndent вҖ” destroys several properties of the input:
//
//   - KEY ORDER. Go's map iteration randomises key order, and
//     MarshalIndent sorts keys alphabetically when marshalling a
//     map[string]any. So {"z":1,"a":2} would become {"a":2,"z":1},
//     silently changing the document's intent.
//
//   - NUMBER FORMATTING. Unmarshal into float64 turns "1.0" into
//     "1" (losing the decimal), and "1e10" into "10000000000"
//     (losing scientific notation). json.Number would preserve it,
//     but Indent preserves it for free.
//
//   - STRING ESCAPING. Unmarshal decodes "\u00e9" to the literal
//     character "Г©", then Marshal re-encodes it as "Г©" (UTF-8),
//     changing the byte representation.
//
// json.Indent is a pure re-indentation pass. It reads the token
// stream and writes the same tokens with new whitespace. The byte
// sequence of every value, every key, every escape вҖ” all preserved.
// Only the whitespace between tokens changes.
//
// Broken input handling (best-effort fix pipeline in order):
//
//  1. Strip // line comments and /* */ block comments (outside strings).
//  2. Strip trailing commas before } or ].
//  3. Convert single-quoted strings to double-quoted.
//  4. Wrap bareword keys: {name: "x"} вҶ’ {"name": "x"}.
//
// If any fix produces parseable JSON, use it and record which fix
// applied. If all fail, return line-cleaned input with a warning.
// NEVER coerce values: NaN does not become null. If the document
// has NaN and nothing works, we warn and return it as-is.
type jsonFormatter struct{}

func (j *jsonFormatter) Format() string { return "json" }

// jsonFix is one entry in the best-effort fix pipeline.
type jsonFix struct {
	name  string
	apply func([]byte) ([]byte, bool)
}

// jsonFixPipeline is the ordered list of fixes. Each fix returns
// (fixedBytes, applied). The first one whose output passes json.Valid
// wins; we return the fixed bytes and record only that fix.
var jsonFixPipeline = []jsonFix{
	{"comments-stripped", stripJSONComments},
	{"trailing-comma", stripJSONTrailingCommas},
	{"single-quotes", convertJSONSingleQuotes},
	{"unquoted-keys", quoteJSONUnquotedKeys},
}

// FormatBytes implements Formatter.
func (j *jsonFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "json"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	// Fast path: already-valid JSON. Use json.Indent directly.
	if json.Valid(work) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, work, "", "  "); err == nil {
			out := buf.Bytes()
			out = trimTrailingNewlines(out)
			out = append(out, '\n')
			if bytes.Equal(out, work) {
				return work, report, nil
			}
			report.AddFix("formatted")
			return out, report, nil
		}
	}

	// Stage 1: best-effort fixes.
	for _, fix := range jsonFixPipeline {
		fixed, applied := fix.apply(work)
		if !applied {
			continue
		}
		if !json.Valid(fixed) {
			continue
		}
		// The fix succeeded and produced valid JSON.
		report.AddFix(fix.name)
		var buf bytes.Buffer
		if err := json.Indent(&buf, fixed, "", "  "); err != nil {
			continue
		}
		out := buf.Bytes()
		out = trimTrailingNewlines(out)
		out = append(out, '\n')
		report.AddFix("formatted")
		return out, report, nil
	}

	// Stage 2: line-level cleanup + warning.
	report.AddWarning("could not parse JSON; returning cleaned input")
	cleaned := lineCleanupJSON(work)
	report.AddFix("line-cleanup")
	return cleaned, report, nil
}

// -----------------------------------------------------------------------------
// Best-effort fixes
// -----------------------------------------------------------------------------

// stripJSONComments removes // and /* */ comments outside strings.
func stripJSONComments(raw []byte) ([]byte, bool) {
	var (
		out     bytes.Buffer
		inStr   bool
		escape  bool
		changed bool
	)

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		if inStr {
			out.WriteByte(c)
			if escape {
				escape = false
				continue
			}
			switch c {
			case '\\':
				escape = true
			case '"':
				inStr = false
			}
			continue
		}

		switch c {
		case '"':
			inStr = true
			out.WriteByte(c)
		case '/':
			if i+1 < len(raw) && raw[i+1] == '/' {
				changed = true
				i += 2
				for i < len(raw) && raw[i] != '\n' {
					i++
				}
				if i < len(raw) {
					out.WriteByte('\n')
				}
				continue
			}
			if i+1 < len(raw) && raw[i+1] == '*' {
				changed = true
				i += 2
				for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
					i++
				}
				if i+1 < len(raw) {
					i++ // skip '/'
				}
				continue
			}
			out.WriteByte(c)
		default:
			out.WriteByte(c)
		}
	}

	return out.Bytes(), changed
}

// stripJSONTrailingCommas removes commas immediately before } or ]
// (with optional whitespace between).
func stripJSONTrailingCommas(raw []byte) ([]byte, bool) {
	var (
		out     bytes.Buffer
		inStr   bool
		escape  bool
		changed bool
	)

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		if inStr {
			out.WriteByte(c)
			if escape {
				escape = false
				continue
			}
			switch c {
			case '\\':
				escape = true
			case '"':
				inStr = false
			}
			continue
		}

		if c == '"' {
			inStr = true
			out.WriteByte(c)
			continue
		}

		if c == ',' {
			// Look ahead: skip whitespace, check for } or ].
			j := i + 1
			for j < len(raw) {
				switch raw[j] {
				case ' ', '\t', '\n', '\r':
					j++
					continue
				}
				break
			}
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				changed = true
				continue // drop the comma
			}
		}

		out.WriteByte(c)
	}

	return out.Bytes(), changed
}

// convertJSONSingleQuotes rewrites single-quoted strings to double-quoted,
// escaping any double quotes inside.
func convertJSONSingleQuotes(raw []byte) ([]byte, bool) {
	var (
		out     bytes.Buffer
		inStr   bool
		quote   byte
		escape  bool
		changed bool
	)

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		if !inStr {
			if c == '\'' {
				inStr = true
				quote = '\''
				out.WriteByte('"')
				changed = true
				continue
			}
			out.WriteByte(c)
			continue
		}

		if escape {
			escape = false
			if quote == '\'' && c == '\'' {
				out.WriteByte('\'')
				continue
			}
			if quote == '"' && c == '"' {
				out.WriteString(`\"`)
				continue
			}
			out.WriteByte('\\')
			out.WriteByte(c)
			continue
		}

		switch c {
		case '\\':
			escape = true
		case '\'':
			if quote == '\'' {
				out.WriteByte('"')
				inStr = false
				continue
			}
			out.WriteByte(c)
		case '"':
			if quote == '\'' {
				out.WriteString(`\"`)
				continue
			}
			out.WriteByte('"')
			inStr = false
		default:
			out.WriteByte(c)
		}
	}

	return out.Bytes(), changed
}

// quoteJSONUnquotedKeys wraps object keys that are not already quoted.
func quoteJSONUnquotedKeys(raw []byte) ([]byte, bool) {
	var out bytes.Buffer
	changed := false
	inStr := false
	escape := false
	expectKey := false

	i := 0
	for i < len(raw) {
		c := raw[i]

		if inStr {
			out.WriteByte(c)
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				inStr = false
			}
			i++
			continue
		}

		switch c {
		case '"':
			inStr = true
			expectKey = false
			out.WriteByte(c)
			i++
		case '{', ',':
			out.WriteByte(c)
			expectKey = true
			i++
		case ' ', '\t', '\n', '\r':
			out.WriteByte(c)
			i++
		case ':':
			out.WriteByte(c)
			expectKey = false
			i++
		case '}', ']':
			out.WriteByte(c)
			expectKey = false
			i++
		default:
			if expectKey && isIdentStart(c) {
				start := i
				for i < len(raw) && isIdentChar(raw[i]) {
					i++
				}
				ident := raw[start:i]
				// Peek ahead for ':'
				j := i
				for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
					j++
				}
				if j < len(raw) && raw[j] == ':' {
					out.WriteByte('"')
					out.Write(ident)
					out.WriteByte('"')
					changed = true
					expectKey = false
					continue
				}
				out.Write(ident)
				expectKey = false
				continue
			}
			out.WriteByte(c)
			i++
		}
	}

	return out.Bytes(), changed
}

// isIdentStart / isIdentChar are minimal JSON-identifier character tests.
func isIdentStart(c byte) bool {
	return c == '_' || c == '$' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// lineCleanupJSON is the last-resort fallback for unparseable JSON:
// trims trailing whitespace per line, collapses blank runs, ensures
// exactly one trailing newline.
func lineCleanupJSON(raw []byte) []byte {
	lines := strings.Split(string(raw), "\n")
	var out bytes.Buffer
	prevBlank := false
	for _, line := range lines {
		trimmed := rstrip(line)
		if trimmed == "" {
			if prevBlank {
				continue
			}
			prevBlank = true
			out.WriteByte('\n')
			continue
		}
		prevBlank = false
		out.WriteString(trimmed)
		out.WriteByte('\n')
	}
	result := out.Bytes()
	result = trimTrailingNewlines(result)
	if len(result) > 0 {
		result = append(result, '\n')
	}
	return result
}