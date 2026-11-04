// Package format implements the "permissive formatting" engine used by
// the Formatter tab.
//
// The philosophy is the one format-express.dev uses:
//
//   - The formatter NEVER refuses because the input is broken. If the
//     HTML5 parser can ingest it, we format it. If a text format has a
//     real syntax error, we do best-effort line-level cleanup and return
//     the result with a warning, not a 4xx.
//
//   - The formatter ALWAYS tries to produce a prettier output. A
//     canonical input comes back unchanged with Changed=false; a messy
//     input comes back pretty-printed with Changed=true and a list of
//     what was done.
//
//   - There is no "level" parameter. One formatter per format. One
//     canonical style. The user does not have to pick "strict" vs
//     "normal" vs "lenient" — the formatter just formats.
//
// Every format lives in its own file (json.go, html.go, xml.go, ...) and
// registers itself in init(). This mirrors the pattern already used by
// internal/formatters, so a future maintainer sees a familiar shape.
package format

import (
	"bytes"
	"path/filepath"
	"strings"
	"sync"
)

type Formatter interface {
	// Format returns the canonical lower-case name: "html", "json", ...
	Format() string

	// FormatBytes takes raw bytes and returns a formatted copy.
	//
	//   - If the input is already canonical: returns it unchanged
	//     with Report.Changed == false.
	//   - If the input is messy but parseable: returns the pretty
	//     version, Changed == true, and lists what was applied.
	//   - If the input is unparseable: does whatever line-level cleanup
	//     is possible, returns the result with Changed == true AND at
	//     least one warning explaining what could not be parsed. Does
	//     NOT return an error.
	//
	// The only case where FormatBytes returns a non-nil error is
	// ErrEmptyInput (zero bytes). Every other scenario must produce
	// a Report and some output.
	FormatBytes(raw []byte) ([]byte, Report, error)
}

// Report is the minimal record of what the formatter did.
type Report struct {
	Format   string   `json:"format"`             // "html", "json", ...
	Changed  bool     `json:"changed"`            // false → input was already canonical
	Applied  []string `json:"applied,omitempty"`  // e.g. ["indented", "normalised-tags"]
	Warnings []string `json:"warnings,omitempty"` // e.g. ["parser recovered from unclosed tags"]
}

// AddFix records that a particular formatting step ran. De-duplicates.
func (r *Report) AddFix(name string) {
	for _, a := range r.Applied {
		if a == name {
			return
		}
	}
	r.Applied = append(r.Applied, name)
	r.Changed = true
}

// AddWarning records a non-fatal notice shown to the user.
func (r *Report) AddWarning(msg string) {
	r.Warnings = append(r.Warnings, msg)
}

// HeaderValue returns a compact, header-safe summary used by the
// /format endpoint when the caller did not request the JSON envelope.
//
//	"html;changed=true;fixed=indented,normalised-tags;warn=0"
func (r *Report) HeaderValue() string {
	changed := "false"
	if r.Changed {
		changed = "true"
	}
	var b strings.Builder
	b.WriteString(r.Format)
	b.WriteString(";changed=")
	b.WriteString(changed)
	if len(r.Applied) > 0 {
		b.WriteString(";fixed=")
		b.WriteString(strings.Join(r.Applied, ","))
	}
	b.WriteString(";warn=")
	b.WriteString(itoa(len(r.Warnings)))
	return b.String()
}

// itoa is a tiny dependency-free integer formatter, so this file has
// only the standard library as an import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// -----------------------------------------------------------------------------
// Registry
// -----------------------------------------------------------------------------

var (
	regMu    sync.RWMutex
	registry = map[string]Formatter{}
)

// register is called from each format file's init(). Duplicate
// registrations panic — a duplicate is always a bug, and we want it
// loud at startup.
func register(f Formatter) {
	regMu.Lock()
	defer regMu.Unlock()

	name := strings.ToLower(f.Format())
	if name == "" {
		panic("format: Register called with empty Format()")
	}
	if _, exists := registry[name]; exists {
		panic("format: duplicate registration for format " + name)
	}
	registry[name] = f
}

// lookup returns the registered Formatter for a canonical name.
func lookup(name string) (Formatter, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

// -----------------------------------------------------------------------------
// Engine
// -----------------------------------------------------------------------------

// Engine is the top-level dispatcher the API layer calls.
// Safe for concurrent use; it holds no per-call state.
type Engine struct{}

// NewEngine returns an Engine backed by the global registry.
func NewEngine() *Engine { return &Engine{} }

// DetectFormat examines the filename extension and the raw bytes and
// returns the canonical format name. It returns "" if no format
// matches, and the caller should then respond with ErrUnsupportedFormat.
//
// Detection rules for HTML:
//
//   - extension `.html` or `.htm` → "html" (authoritative)
//   - content starts with `<!doctype html` (case-insensitive) → "html"
//   - content starts with `<html` → "html"
//   - content starts with `<!--` followed by any amount of data and
//     eventually `<html` on a subsequent line → "html"
//
// Extension wins if it names a registered format. Content sniffing is a
// fallback for extensionless uploads.
func (e *Engine) DetectFormat(filename string, raw []byte) string {
	// 1. Extension
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext != "" {
		if _, ok := lookup(ext); ok {
			return ext
		}
	}

	// 2. Content sniff (skip BOM and leading whitespace)
	body := trimBOMAndSpace(raw)
	if len(body) == 0 {
		return ""
	}

	// INI content sniff.
	//
	// An INI file is identified by one of two patterns:
	//
	//   (a) The first non-blank line is a section header: `[name]`.
	//   (b) The file contains at least two lines that look like
	//       `key=value` or `key:value`, AND no line starts with `<`
	//       (which would indicate HTML/XML) and no line starts with
	//       `{` or `[` followed by a quote or digit (which would
	//       indicate JSON).
	//
	// We check this BEFORE the DOCTYPE/`<html` checks because INI
	// files cannot start with `<`, and before the generic "any tag
	// starts with `<`" check because that would otherwise claim
	// this file as HTML.
	if isLikelyINI(body) {
		return "ini"
	}

	// DOCTYPE
	lower := body
	if len(lower) >= 15 && strings.HasPrefix(strings.ToLower(string(lower[:15])), "<!doctype html") {
		return "html"
	}

	// <html …>
	if len(lower) >= 5 && strings.HasPrefix(strings.ToLower(string(lower[:5])), "<html") {
		return "html"
	}

	// Any tag at all → HTML is a reasonable guess, because no other
	// supported format starts with `<` outside of XML. XML has its own
	// `<?xml` prolog; the HTML parser will mangle that, so we require a
	// plausible HTML element name.
	if lower[0] == '<' && !strings.HasPrefix(strings.ToLower(string(lower[:5])), "<?xml") {
		return "html"
	}

	return ""
}

// Format is the single entry point used by the API layer. It detects
// the format and dispatches to the registered Formatter.
//
// The only non-nil error paths are ErrEmptyInput and ErrUnsupportedFormat.
// A formatter returning a Report with warnings is still a success.
func (e *Engine) Format(filename string, raw []byte) ([]byte, Report, error) {
	if len(raw) == 0 {
		return nil, Report{}, ErrEmptyInput
	}

	name := e.DetectFormat(filename, raw)
	if name == "" {
		return nil, Report{}, ErrUnsupportedFormat
	}

	f, ok := lookup(name)
	if !ok {
		return nil, Report{}, ErrUnsupportedFormat
	}

	// Defensive copy so a buggy formatter cannot corrupt the caller's
	// slice.
	safe := make([]byte, len(raw))
	copy(safe, raw)

	out, report, err := f.FormatBytes(safe)
	if err != nil {
		return nil, report, err
	}

	// Fill in Report.Format if a formatter forgot to.
	if report.Format == "" {
		report.Format = name
	}
	return out, report, nil
}

// trimBOMAndSpace strips a UTF-8 BOM and leading ASCII whitespace.
// Used by DetectFormat so the sniffing logic sees the real first byte.
func trimBOMAndSpace(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	i := 0
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return b[i:]
		}
	}
	return b[i:]
}

// -----------------------------------------------------------------------------
// INI detection helpers
// -----------------------------------------------------------------------------
//
// These helpers live here (not in ini.go) because DetectFormat needs
// them, and format.go must not depend on any specific formatter being
// compiled in. Keeping them format-agnostic means a future format can
// reuse them without importing ini.go.
//
// NOTE: The formatter itself (iniFormatter, iniLineKind, etc.) lives
// in ini.go. Only the pure detection predicates live here. This keeps
// the boundary clean: format.go answers "what format is this?", and
// ini.go answers "how do we format it?".

// isLikelyINI reports whether the leading bytes of body look like an
// INI document. See the comment at the call site in DetectFormat.
func isLikelyINI(body []byte) bool {
	// Reject anything that starts with '<' — that is HTML, XML, or a
	// processing instruction. INI files never start with '<' outside
	// of a section name, and section names cannot begin with '<'.
	firstLine := body
	if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
		firstLine = body[:idx]
	}
	trimmed := bytes.TrimSpace(firstLine)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return false
	}

	// Pattern (a): first non-blank line is `[name]`.
	// We validate the inner characters so a JSON array like `[1,2,3]`
	// (which also starts with `[`) does not match.
	if isINISectionHeader(trimmed) {
		return true
	}

	// Pattern (b): at least two lines that match key=value or key:value.
	lines := strings.Split(string(body), "\n")
	kvCount := 0
	for _, line := range lines {
		t := strings.TrimLeft(line, " \t")
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") ||
			strings.HasPrefix(t, "[") {
			continue
		}
		// Reject lines starting with `{` — that is JSON.
		if strings.HasPrefix(t, "{") {
			return false
		}
		// Reject lines starting with `[` followed by `"` or a digit —
		// that is JSON.
		if strings.HasPrefix(t, "[") && len(t) > 1 &&
			(t[1] == '"' || (t[1] >= '0' && t[1] <= '9')) {
			return false
		}
		// Match key=value or key:value.
		if hasEqColon(t) {
			kvCount++
			if kvCount >= 2 {
				return true
			}
		}
	}
	return false
}

// hasEqColon reports whether t contains '=' or ':' outside of quotes.
// Used only by isLikelyINI to count key-value-looking lines.
func hasEqColon(t string) bool {
	inQuote := byte(0)
	for i := 0; i < len(t); i++ {
		c := t[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '=', ':':
			return true
		}
	}
	return false
}

// isINISectionHeader reports whether the single-line input t looks
// like an INI section header — i.e. `[name]` where `name` contains
// only letters, digits, dots, underscores, hyphens, or spaces.
//
// `[1,2,3]` (JSON array) and `["a","b"]` (JSON string array) are
// rejected because their contents include commas or quotes.
func isINISectionHeader(t []byte) bool {
	if len(t) < 3 || t[0] != '[' || t[len(t)-1] != ']' {
		return false
	}
	inner := t[1 : len(t)-1]
	if len(inner) == 0 {
		return false
	}
	for _, c := range inner {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-' || c == ' ' || c == '\t':
		default:
			return false
		}
	}
	return true
}