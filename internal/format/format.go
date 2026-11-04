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
//     "normal" vs "lenient" вҖ” the formatter just formats.
//
// Every format lives in its own file (json.go, html.go, xml.go, ...) and
// registers itself in init(). This mirrors the pattern already used by
// internal/formatters, so a future maintainer sees a familiar shape.
package format

import (
	"bytes"
	"encoding/json"
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
	Changed  bool     `json:"changed"`            // false вҶ’ input was already canonical
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
	"yml":      "yaml", // YAML's other conventional extension
	"markdown": "md",   // Markdown's long-form extension
}

// register is called from each format file's init(). Duplicate
// registrations panic вҖ” a duplicate is always a bug, and we want it
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
//  1. JSON вҖ” first non-space byte is `{` or `[` AND the content
//     parses as JSON.
//  2. XML вҖ” `<?xml` prolog, `<!DOCTYPE` that is not HTML, or `<tag>`
//     where the tag name is NOT one of the standard HTML tag names.
//  3. TOML вҖ” requires a table header OR 2+ key=value lines with at
//     least one quoted string value.
//  4. INI вҖ” a `[section]` header OR 2+ bare key=value lines (no
//     quoted values).
//  5. YAML вҖ” `---` marker OR 2+ `key: value` lines.
//  6. CSV вҖ” 2+ non-empty lines with the same positive delimiter count
//     across `,`, `;`, or `\t`, OR the same pattern after unescaping
//     literal `\n` sequences.
//  7. Markdown вҖ” a heading, code fence, blockquote, setext underline,
//     or reference link line is present. Runs before the HTML fallback
//     because plain prose with a `# heading` is Markdown, not HTML.
//  8. HTML вҖ” doctype, `<html`, or any tag (fallback for anything that
//     starts with `<` and was not routed to XML above).
func (e *Engine) DetectFormat(filename string, raw []byte) string {
	// 1. Extension
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext != "" {
		if _, ok := lookup(ext); ok {
			return ext
		}
		// Some formats have multiple extensions that map to the
		// same formatter. YAML and Markdown are the only such cases.
		if alias, ok := extensionAlias[ext]; ok {
			return alias
		}
	}

	// 2. Content sniff (skip BOM and leading whitespace)
	body := trimBOMAndSpace(raw)
	if len(body) == 0 {
		return ""
	}

	// -------------------------------------------------------------------------
	// JSON sniff вҖ” runs first because it is the strongest signal.
	// -------------------------------------------------------------------------
	if body[0] == '{' || body[0] == '[' {
		if json.Valid(body) {
			return "json"
		}
	}

	// -------------------------------------------------------------------------
	// XML sniff вҖ” runs before TOML/INI/YAML/CSV because XML is
	// unambiguously identified by the leading `<`.
	// -------------------------------------------------------------------------
	if body[0] == '<' {
		lower := strings.ToLower(string(body))
		if strings.HasPrefix(lower, "<?xml") {
			return "xml"
		}
		if strings.HasPrefix(lower, "<!doctype") {
			if strings.HasPrefix(lower, "<!doctype html") {
				return "html"
			}
			return "xml"
		}
		if len(body) >= 2 && body[1] != '!' && body[1] != '?' && body[1] != '/' {
			tagName := extractXMLTagName(body)
			if tagName != "" && !isHTMLTagName(tagName) {
				return "xml"
			}
		}
	}

	// -------------------------------------------------------------------------
	// TOML content sniff
	// -------------------------------------------------------------------------
	if isLikelyTOML(body) {
		return "toml"
	}

	// -------------------------------------------------------------------------
	// INI content sniff
	// -------------------------------------------------------------------------
	if isLikelyINI(body) {
		return "ini"
	}

	// -------------------------------------------------------------------------
	// YAML content sniff
	// -------------------------------------------------------------------------
	if isLikelyYAML(body) {
		return "yaml"
	}

	// -------------------------------------------------------------------------
	// CSV content sniff
	// -------------------------------------------------------------------------
	if csvLooksLikeCSV(body) {
		return "csv"
	}

	// -------------------------------------------------------------------------
	// Markdown content sniff
	// -------------------------------------------------------------------------
	// Runs after CSV (CSV has stronger structural signals) and
	// before the HTML fallback (any remaining `<`-leading content
	// is HTML, not Markdown).
	if isLikelyMarkdown(body) {
		return "md"
	}

	// -------------------------------------------------------------------------
	// HTML fallback
	// -------------------------------------------------------------------------
	if body[0] == '<' {
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

	safe := make([]byte, len(raw))
	copy(safe, raw)

	out, report, err := f.FormatBytes(safe)
	if err != nil {
		return nil, report, err
	}

	if report.Format == "" {
		report.Format = name
	}
	return out, report, nil
}

// trimBOMAndSpace strips a UTF-8 BOM and leading ASCII whitespace.
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
// XML detection helpers
// -----------------------------------------------------------------------------

func extractXMLTagName(body []byte) string {
	i := 1
	if i >= len(body) || !isXMLLetter(body[i]) {
		return ""
	}
	start := i
	for i < len(body) && isXMLNameChar(body[i]) {
		i++
	}
	return string(body[start:i])
}

func isXMLLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isXMLNameChar(c byte) bool {
	return isXMLLetter(c) || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == ':'
}

func isHTMLTagName(name string) bool {
	htmlTags := map[string]bool{
		"html": true, "head": true, "body": true, "title": true,
		"meta": true, "link": true, "style": true, "script": true,
		"div": true, "span": true, "p": true, "a": true,
		"ul": true, "ol": true, "li": true, "table": true,
		"thead": true, "tbody": true, "tfoot": true, "tr": true,
		"td": true, "th": true, "form": true, "input": true,
		"button": true, "select": true, "option": true, "textarea": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"section": true, "article": true, "nav": true, "header": true,
		"footer": true, "main": true, "aside": true,
		"img": true, "br": true, "hr": true, "video": true, "audio": true,
		"canvas": true, "svg": true, "label": true, "fieldset": true,
		"legend": true, "iframe": true, "pre": true, "code": true,
		"blockquote": true, "strong": true, "em": true, "b": true, "i": true,
		"u": true, "small": true, "sub": true, "sup": true,
		"figure": true, "figcaption": true, "details": true,
		"summary": true, "dialog": true, "menu": true,
	}
	return htmlTags[strings.ToLower(name)]
}

// -----------------------------------------------------------------------------
// TOML detection helpers
// -----------------------------------------------------------------------------

func isLikelyTOML(body []byte) bool {
	trimmedStart := bytes.TrimSpace(body)
	if len(trimmedStart) > 0 && trimmedStart[0] == '<' {
		return false
	}
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
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			hasHeader = true
			continue
		}
		eq := strings.IndexByte(t, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(t[:eq])
		value := strings.TrimSpace(t[eq+1:])
		if key == "" {
			continue
		}
		if len(value) > 0 && (value[0] == '"' || value[0] == '\'') {
			quotedKeyValue++
		} else {
			plainKeyValue++
		}
	}

	if hasHeader && quotedKeyValue >= 1 {
		return true
	}
	if quotedKeyValue >= 1 && (quotedKeyValue+plainKeyValue) >= 2 {
		return true
	}
	return false
}

// -----------------------------------------------------------------------------
// INI detection helpers
// -----------------------------------------------------------------------------

func isLikelyINI(body []byte) bool {
	firstLine := body
	if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
		firstLine = body[:idx]
	}
	trimmed := bytes.TrimSpace(firstLine)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return false
	}

	if isINISectionHeader(trimmed) {
		return true
	}

	lines := strings.Split(string(body), "\n")
	kvCount := 0
	for _, line := range lines {
		t := strings.TrimLeft(line, " \t")
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") ||
			strings.HasPrefix(t, "[") {
			continue
		}
		if strings.HasPrefix(t, "{") {
			return false
		}
		if strings.HasPrefix(t, "[") && len(t) > 1 &&
			(t[1] == '"' || (t[1] >= '0' && t[1] <= '9')) {
			return false
		}
		if hasEqualsSeparator(t) {
			kvCount++
			if kvCount >= 2 {
				return true
			}
		}
	}
	return false
}

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

func isLikelyYAML(body []byte) bool {
	trimmedStart := bytes.TrimSpace(body)
	if len(trimmedStart) > 0 && trimmedStart[0] == '<' {
		return false
	}
	if len(trimmedStart) > 0 && trimmedStart[0] == '{' {
		return false
	}
	if len(trimmedStart) > 1 && trimmedStart[0] == '[' {
		inner := trimmedStart[1]
		if inner == '"' || inner == ']' || (inner >= '0' && inner <= '9') ||
			inner == '{' || inner == 't' || inner == 'f' || inner == 'n' {
			return false
		}
	}

	if bytes.HasPrefix(trimmedStart, []byte("---")) {
		rest := trimmedStart[3:]
		if len(rest) == 0 || rest[0] == '\n' || rest[0] == '\r' || rest[0] == ' ' || rest[0] == '\t' {
			return true
		}
	}

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

func looksLikeYAMLMapping(t string) bool {
	if t == "" {
		return false
	}
	if t[0] == '"' || t[0] == '\'' {
		close := strings.IndexByte(t[1:], t[0])
		if close < 0 {
			return false
		}
		after := t[1+close+1:]
		return strings.HasPrefix(after, ":")
	}
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

// -----------------------------------------------------------------------------
// CSV detection helpers
// -----------------------------------------------------------------------------

func csvLooksLikeCSV(body []byte) bool {
	if csvLooksLikeCSVWithNewlines(body) {
		return true
	}
	if bytes.Contains(body, []byte(`\n`)) {
		unescaped := bytes.ReplaceAll(body, []byte(`\n`), []byte("\n"))
		if csvLooksLikeCSVWithNewlines(unescaped) {
			return true
		}
	}
	return false
}

func csvLooksLikeCSVWithNewlines(body []byte) bool {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")

	var sample []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		sample = append(sample, line)
		if len(sample) >= 10 {
			break
		}
	}
	if len(sample) < 2 {
		return false
	}

	candidates := []byte{',', ';', '\t'}
	for _, d := range candidates {
		firstCount := countDelimiterCSV(sample[0], d)
		if firstCount == 0 {
			continue
		}
		consistent := true
		for _, line := range sample[1:] {
			if countDelimiterCSV(line, d) != firstCount {
				consistent = false
				break
			}
		}
		if consistent {
			return true
		}
	}
	return false
}

func countDelimiterCSV(line string, delim byte) int {
	n := 0
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			if inQuote && i+1 < len(line) && line[i+1] == '"' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if !inQuote && c == delim {
			n++
		}
	}
	return n
}

// -----------------------------------------------------------------------------
// Markdown detection helper
// -----------------------------------------------------------------------------

// isLikelyMarkdown reports whether body contains at least one strong
// Markdown signature. We deliberately require a *strong* signal (a
// heading, code fence, blockquote, setext underline, or reference
// link definition) so that plain prose with no Markdown syntax is
// not detected as Markdown.
//
// Plain prose with an isolated `#` character (e.g. "issue #42") is
// NOT a Markdown signature вҖ” we require a heading at the start of a
// line, not just a `#` anywhere in the text.
func isLikelyMarkdown(body []byte) bool {
	// Reject content that starts with HTML вҖ” that is definitely not
	// Markdown.
	if len(body) > 0 && body[0] == '<' {
		return false
	}

	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")

	// Walk up to the first 50 lines. Real Markdown files have their
	// signal near the top; if the signature is buried past line 50,
	// it's more likely to be prose with a stray `#`.
	maxScan := 50
	if len(lines) < maxScan {
		maxScan = len(lines)
	}

	hasPriorContent := false
	for i := 0; i < maxScan; i++ {
		line := lines[i]
		trimmed := strings.TrimLeft(line, " \t")

		// ATX heading: `# ` or `## ` ... up to `###### `. Must have
		// a space after the hashes.
		if looksLikeATXHeading(trimmed) {
			return true
		}

		// Fenced code block opener: 3+ backticks or tildes at the
		// start of the line (after up to 3 leading spaces).
		if _, ok := detectMarkdownFenceOpener(trimmed); ok {
			return true
		}

		// Blockquote: a line starting with `> ` or `>` followed by
		// content. A bare `>` with nothing after is a lazily
		// continued blockquote, which is less common; we require
		// `> ` (space after) to be conservative.
		if strings.HasPrefix(trimmed, "> ") {
			return true
		}

		// Setext heading underline: a line that is only `===` or
		// `---` (3+ chars). Only counts if there is prior content
		// (otherwise `---` at the top is a YAML doc marker, which
		// was already claimed by the YAML sniff).
		if hasPriorContent {
			if isSetextUnderline(trimmed) {
				return true
			}
		}

		// Reference link definition: `[label]: url`. Common in
		// README files and other long-form Markdown. Requires a
		// leading `[` so we don't over-trigger on prose.
		if looksLikeReferenceLinkDefinition(trimmed) {
			return true
		}

		// Mark a line as "prior content" once we've seen a
		// non-blank line. Setext underline detection requires
		// this to fire (see above).
		if strings.TrimSpace(trimmed) != "" {
			hasPriorContent = true
		}
	}

	return false
}

// looksLikeATXHeading reports whether trimmed begins with 1-6 `#`
// characters followed by a space and at least one non-space char.
func looksLikeATXHeading(trimmed string) bool {
	hashes := 0
	for hashes < len(trimmed) && hashes < 6 && trimmed[hashes] == '#' {
		hashes++
	}
	if hashes == 0 {
		return false
	}
	if hashes >= len(trimmed) {
		// Line is only hashes, no heading text. This is a valid
		// empty heading, but rare enough that we do not count
		// it as a signature.
		return false
	}
	// Must be followed by whitespace and text.
	if trimmed[hashes] != ' ' && trimmed[hashes] != '\t' {
		return false
	}
	rest := strings.TrimLeft(trimmed[hashes:], " \t")
	return rest != ""
}

// detectMarkdownFenceOpener reports whether trimmed opens a fenced
// code block. Same rules as the formatter's detectFenceOpener.
func detectMarkdownFenceOpener(trimmed string) (string, bool) {
	if len(trimmed) < 3 {
		return "", false
	}
	first := trimmed[0]
	if first != '`' && first != '~' {
		return "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == first {
		n++
	}
	if n < 3 {
		return "", false
	}
	marker := trimmed[:n]
	if first == '`' && strings.ContainsRune(trimmed[n:], '`') {
		return "", false
	}
	return marker, true
}

// isSetextUnderline reports whether trimmed is a valid setext heading
// underline: 3+ `=` or 3+ `-` characters and nothing else.
func isSetextUnderline(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	first := trimmed[0]
	if first != '=' && first != '-' {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != first {
			return false
		}
	}
	return true
}

// looksLikeReferenceLinkDefinition reports whether trimmed looks like
// a Markdown reference link definition: `[label]: url` or
// `[label]: url "title"`. Requires a leading `[` so we do not
// over-trigger on prose.
func looksLikeReferenceLinkDefinition(trimmed string) bool {
	if len(trimmed) < 4 || trimmed[0] != '[' {
		return false
	}
	// Find the closing `]`.
	closeIdx := strings.IndexByte(trimmed, ']')
	if closeIdx < 2 {
		return false
	}
	// Must be immediately followed by a colon.
	after := trimmed[closeIdx+1:]
	if len(after) == 0 || after[0] != ':' {
		return false
	}
	// Must have at least one non-space character after the colon.
	rest := strings.TrimLeft(after[1:], " \t")
	return rest != ""
}