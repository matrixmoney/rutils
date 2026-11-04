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

// extensionAlias maps non-canonical file extensions to the canonical
// formatter name that handles them. Used by DetectFormat so that
// e.g. a file named `config.yml` resolves to the `yaml` formatter
// even though only `yaml` is registered.
var extensionAlias = map[string]string{
	"yml": "yaml", // YAML's other conventional extension
}

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
// Sniffing order (from most to least specific):
//
//  1. TOML — requires a table header OR 2+ key=value lines with at
//     least one quoted string value. Very specific: quoted string
//     values distinguish TOML from INI in practice.
//  2. INI — a `[section]` header OR 2+ bare key=value lines (no
//     quoted values). Cannot collide with TOML because TOML runs
//     first and requires quoted values.
//  3. YAML — `---` marker OR 2+ `key: value` lines. Cannot collide
//     with INI because INI requires `=` not `:`.
//  4. HTML — doctype or `<html` or any tag.
func (e *Engine) DetectFormat(filename string, raw []byte) string {
	// 1. Extension
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext != "" {
		if _, ok := lookup(ext); ok {
			return ext
		}
		// Some formats have multiple extensions that map to the
		// same formatter. YAML is the only such case today.
		if alias, ok := extensionAlias[ext]; ok {
			return alias
		}
	}

	// 2. Content sniff (skip BOM and leading whitespace)
	body := trimBOMAndSpace(raw)
	if len(body) == 0 {
		return ""
	}

	// TOML content sniff. Runs first because it is the most
	// specific: it requires quoted string values, which distinguish
	// it from plain INI files with `=` separators.
	if isLikelyTOML(body) {
		return "toml"
	}

	// INI content sniff. Requires a section header OR 2+ bare
	// key=value lines (no quoted values — those would have matched
	// the TOML sniff above).
	if isLikelyINI(body) {
		return "ini"
	}

	// YAML content sniff. Requires `---` marker OR 2+ `key: value`
	// lines. Cannot collide with INI because INI requires `=`.
	if isLikelyYAML(body) {
		return "yaml"
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
// TOML detection helpers
// -----------------------------------------------------------------------------

// isLikelyTOML reports whether the leading bytes of body look like a
// TOML document. See the comment at the call site in DetectFormat.
//
// The discriminator is quoted string values: TOML frequently uses
// them (`key = "value"`), while INI almost never does. A file with
// `[section]` headers AND quoted values is overwhelmingly TOML.
func isLikelyTOML(body []byte) bool {
	// Reject HTML/XML.
	trimmedStart := bytes.TrimSpace(body)
	if len(trimmedStart) > 0 && trimmedStart[0] == '<' {
		return false
	}
	// Reject JSON.
	if len(trimmedStart) > 0 && trimmedStart[0] == '{' {
		return false
	}

	lines := strings.Split(string(body), "\n")
	hasHeader := false
	quotedKeyValue := 0
	plainKeyValue := 0

	for _, line := range lines {
		t := strings.TrimLeft(line, " \t")
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}

		// Table header — `[name]` or `[[name]]`.
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			hasHeader = true
			continue
		}

		// Key-value line.
		eq := strings.IndexByte(t, '=')
		if eq <= 0 {
			continue
		}

		key := strings.TrimSpace(t[:eq])
		value := strings.TrimSpace(t[eq+1:])
		if key == "" {
			continue
		}

		// Quoted value?
		if len(value) > 0 && (value[0] == '"' || value[0] == '\'') {
			quotedKeyValue++
		} else {
			plainKeyValue++
		}
	}

	// Signal (a): a table header PLUS at least one quoted value.
	if hasHeader && quotedKeyValue >= 1 {
		return true
	}

	// Signal (b): at least two key-value lines, at least one quoted.
	if quotedKeyValue >= 1 && (quotedKeyValue+plainKeyValue) >= 2 {
		return true
	}

	return false
}

// -----------------------------------------------------------------------------
// INI detection helpers
// -----------------------------------------------------------------------------

// isLikelyINI reports whether the leading bytes of body look like an
// INI document. See the comment at the call site in DetectFormat.
func isLikelyINI(body []byte) bool {
	// Reject anything that starts with '<' — that is HTML, XML, or a
	// processing instruction.
	firstLine := body
	if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
		firstLine = body[:idx]
	}
	trimmed := bytes.TrimSpace(firstLine)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return false
	}

	// Pattern (a): first non-blank line is `[name]`.
	if isINISectionHeader(trimmed) {
		return true
	}

	// Pattern (b): at least two lines that match key=value.
	//
	// We deliberately do NOT count `key:value` (colon-separated)
	// lines here. A file made entirely of colon-separated pairs
	// with no section header and no equals-sign lines is
	// overwhelmingly more likely to be YAML than INI.
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
		// Match key=value only (equals, not colon).
		if hasEqualsSeparator(t) {
			kvCount++
			if kvCount >= 2 {
				return true
			}
		}
	}
	return false
}

// hasEqualsSeparator reports whether t contains '=' outside of quotes.
// Used only by isLikelyINI to count key=value lines.
func hasEqualsSeparator(t string) bool {
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
		case '=':
			return true
		}
	}
	return false
}

// isINISectionHeader reports whether the single-line input t looks
// like an INI section header — i.e. `[name]` where `name` contains
// only letters, digits, dots, underscores, hyphens, or spaces.
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

// -----------------------------------------------------------------------------
// YAML detection helpers
// -----------------------------------------------------------------------------

// isLikelyYAML reports whether the leading bytes of body look like a
// YAML document. See the comment at the call site in DetectFormat.
func isLikelyYAML(body []byte) bool {
	// Reject HTML/XML.
	trimmedStart := bytes.TrimSpace(body)
	if len(trimmedStart) > 0 && trimmedStart[0] == '<' {
		return false
	}

	// Reject JSON.
	if len(trimmedStart) > 0 && trimmedStart[0] == '{' {
		return false
	}

	// Reject JSON arrays: `[1,2,3]` or `["a","b"]`.
	if len(trimmedStart) > 1 && trimmedStart[0] == '[' {
		inner := trimmedStart[1]
		if inner == '"' || inner == ']' || (inner >= '0' && inner <= '9') ||
			inner == '{' || inner == 't' || inner == 'f' || inner == 'n' {
			return false
		}
	}

	// Pattern (a): YAML document marker `---` at the very start.
	if bytes.HasPrefix(trimmedStart, []byte("---")) {
		rest := trimmedStart[3:]
		if len(rest) == 0 || rest[0] == '\n' || rest[0] == '\r' || rest[0] == ' ' || rest[0] == '\t' {
			return true
		}
	}

	// Pattern (b): at least two `key: value` or `key:value` lines.
	lines := strings.Split(string(body), "\n")
	kvCount := 0
	for _, line := range lines {
		t := strings.TrimLeft(line, " \t")
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "-") {
			continue
		}
		if looksLikeYAMLMapping(t) {
			kvCount++
			if kvCount >= 2 {
				return true
			}
		}
	}
	return false
}

// looksLikeYAMLMapping reports whether t starts with a plausible
// YAML mapping key followed by ':'.
func looksLikeYAMLMapping(t string) bool {
	if t == "" {
		return false
	}

	// Quoted key.
	if t[0] == '"' || t[0] == '\'' {
		close := strings.IndexByte(t[1:], t[0])
		if close < 0 {
			return false
		}
		after := t[1+close+1:]
		return strings.HasPrefix(after, ":")
	}

	// Bare key: scan to the first ':' or whitespace.
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c == ':' {
			if i+1 < len(t) && t[i+1] == '/' {
				return false
			}
			return i > 0
		}
		if c == ' ' || c == '\t' {
			return false
		}
	}
	return false
}