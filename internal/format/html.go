package format

import (
	"bytes"
	"strings"

	xhtml "golang.org/x/net/html"
)

func init() {
	register(&htmlFormatter{})
}

// htmlFormatter is the HTML formatter. It uses golang.org/x/net/html,
// the HTML5 parser that Go ships to mirror the browser's own parser.
//
// This is why the formatter never fails: the HTML5 spec mandates a
// recovery path for every malformed input, so x/net/html can always
// produce a parse tree. `html.Render` then serialises that tree back
// out, and because our formatter runs it through an indentation layer,
// the result is always pretty.
//
// What "formatting" means here:
//
//   - Lowercase tag names ("<DIV>" → "<div>")
//   - Quote bare attributes ("<a href=foo>" → "<a href=\"foo\">")
//   - Close unclosed tags ("<p>hello" → "<p>hello</p>")
//   - Indent block elements to 2 spaces per nesting level
//   - Preserve inline whitespace inside <pre>, <code>, <script>, <style>
//   - Strip leading/trailing blank lines
//   - Normalise the doctype when a full document is present
type htmlFormatter struct{}

func (h *htmlFormatter) Format() string { return "html" }

// HTML element categories used by the indenter.
//
// We only indent "block" elements. Inline elements (<span>, <a>, <b>)
// stay on the same line as their parent content, because inserting
// newlines around them would change the rendered output.
//
// These two lists are deliberately conservative. Missing an element
// from the block list only means it stays inline — never that the
// output is broken.
var htmlBlockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"body": true, "div": true, "dl": true, "fieldset": true,
	"figcaption": true, "figure": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hgroup": true, "hr": true, "html": true,
	"main": true, "nav": true, "ol": true, "p": true, "pre": true,
	"section": true, "table": true, "thead": true, "tbody": true,
	"tfoot": true, "tr": true, "td": true, "th": true,
	"ul": true, "li": true, "canvas": true, "details": true,
	"dialog": true, "summary": true, "menu": true, "legend": true,
	"head": true, "title": true, "meta": true, "link": true,
	"script": true, "style": true,
}

// htmlVoidElements are tags with no close tag and no children. We emit
// them inline and do not push them onto the indentation stack.
var htmlVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// htmlVerbatimElements have their text content preserved byte-for-byte
// — indenting inside them would change the semantics (for <script> /
// <style> it would also change what runs).
var htmlVerbatimElements = map[string]bool{
	"pre": true, "code": true, "script": true, "style": true,
	"textarea": true,
}

// FormatBytes implements Formatter.
//
// Because x/net/html cannot fail on well-formed or malformed input
// alike, the only "failure" branch is if rendering itself returns an
// error (which, in practice, only happens on an invalid token stream
// that the parser already accepted). Even then, we fall back to a
// line-level cleanup rather than returning a 4xx.
func (h *htmlFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "html"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	// Stage 1: parse with the HTML5 parser.
	doc, err := xhtml.Parse(bytes.NewReader(work))
	if err != nil {
		// This path is extremely unlikely (x/net/html almost never
		// returns an error from Parse) but we still don't want to 4xx.
		report.AddWarning("HTML parser reported an issue; returning cleaned input")
		cleaned := collapseBlankLines(string(work))
		return []byte(cleaned), report, nil
	}

	// Stage 2: run our indenter to produce a pretty document.
	var buf bytes.Buffer
	if err := indentHTMLNode(&buf, doc, 0, true); err != nil {
		report.AddWarning("indentation pass failed; returning cleaned input")
		cleaned := collapseBlankLines(string(work))
		return []byte(cleaned), report, nil
	}

	// Stage 3: trim trailing blank lines and ensure a single final newline.
	out := buf.Bytes()
	out = trimTrailingNewlines(out)
	out = append(out, '\n')

	// Compare against the input to decide whether we changed anything.
	// Byte equality is the cheapest reliable signal.
	if bytes.Equal(out, work) {
		return work, report, nil
	}

	report.AddFix("formatted")
	// If the parser had to invent closing tags, the output differs from
	// the input purely because of that — worth calling out.
	if hasUnclosedTags(work) {
		report.AddWarning("some tags were auto-closed during formatting")
	}

	return out, report, nil
}

// -----------------------------------------------------------------------------
// Indenter
// -----------------------------------------------------------------------------

// indentHTMLNode walks the parse tree and writes a pretty HTML string.
//
// The rules, in one place:
//
//   - Block elements get their open tag on its own line, their children
//     each on their own line at depth+1, and their close tag on its own
//     line at depth.
//   - Empty block elements (<div></div> with no children) sit on one
//     line so we don't produce <div>\n</div> for zero-content elements.
//   - Void elements (<img>, <br>) sit on one line, self-closed.
//   - Verbatim elements (<pre>, <script>, <style>, <code>, <textarea>)
//     preserve their inner text byte-for-byte.
//   - Inline elements (<span>, <a>, <b>) are written in-line with no
//     surrounding newlines. Their content may itself be inline.
//   - Text nodes are collapsed of runs of whitespace, trimmed, and —
//     if their parent is a block element — written on their own line
//     indented at depth. If their parent is inline, they go inline.
//   - Comment and doctype nodes each get their own line at depth.
//
// The function ALWAYS writes a trailing newline after every top-level
// construct it emits, which is what makes the caller's
// "trim trailing newlines then add one" step produce a canonical file.
func indentHTMLNode(buf *bytes.Buffer, n *xhtml.Node, depth int, parentIsBlock bool) error {
	switch n.Type {
	case xhtml.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := indentHTMLNode(buf, c, depth, true); err != nil {
				return err
			}
		}
		return nil

	case xhtml.DoctypeNode:
		writeIndent(buf, depth)
		buf.WriteString("<!DOCTYPE html>")
		buf.WriteByte('\n')
		return nil

	case xhtml.CommentNode:
		writeIndent(buf, depth)
		buf.WriteString("<!--")
		buf.WriteString(n.Data)
		buf.WriteString("-->")
		buf.WriteByte('\n')
		return nil

	case xhtml.ElementNode:
		name := strings.ToLower(n.Data)

		// ---- Void element: single self-contained token on its own line ----
		if htmlVoidElements[name] {
			writeIndent(buf, depth)
			writeOpenTag(buf, n, true)
			buf.WriteByte('\n')
			return nil
		}

		// ---- Verbatim element: open tag, raw inner text, close tag ----
		//
		// No indentation of the inner content. The whole construct sits
		// on the current line up to the point that we emit the close
		// tag — matching what every formatter does for <script> and
		// <style>.
		if htmlVerbatimElements[name] {
			writeIndent(buf, depth)
			writeOpenTag(buf, n, false)
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xhtml.TextNode {
					buf.WriteString(c.Data)
				} else if err := indentHTMLNode(buf, c, depth, false); err != nil {
					return err
				}
			}
			buf.WriteString("</")
			buf.WriteString(name)
			buf.WriteByte('>')
			buf.WriteByte('\n')
			return nil
		}

		// ---- Block element ----
		if htmlBlockElements[name] {
			// Empty block element: <div></div> on one line.
			if n.FirstChild == nil {
				writeIndent(buf, depth)
				writeOpenTag(buf, n, false)
				buf.WriteString("</")
				buf.WriteString(name)
				buf.WriteByte('>')
				buf.WriteByte('\n')
				return nil
			}

			writeIndent(buf, depth)
			writeOpenTag(buf, n, false)
			buf.WriteByte('\n')
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if err := indentHTMLNode(buf, c, depth+1, true); err != nil {
					return err
				}
			}
			writeIndent(buf, depth)
			buf.WriteString("</")
			buf.WriteString(name)
			buf.WriteByte('>')
			buf.WriteByte('\n')
			return nil
		}

		// ---- Inline element ----
		//
		// No newline, no indent. The caller (the parent element's
		// loop) is responsible for emitting the newline + indent that
		// starts the line this inline element sits on.
		writeOpenTag(buf, n, false)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := indentHTMLNode(buf, c, depth, false); err != nil {
				return err
			}
		}
		buf.WriteString("</")
		buf.WriteString(name)
		buf.WriteByte('>')
		return nil

	case xhtml.TextNode:
		// Collapse whitespace runs to single spaces and trim. This is
		// what makes `<b>a</b>  <i>b</i>` render with exactly one
		// space between a and b, and what strips the leading newline
		// that x/net/html inserts when it "pretty-prints" its own
		// text nodes.
		text := strings.Join(strings.Fields(n.Data), " ")
		if text == "" {
			return nil
		}

		// Under a block parent, the text is its own line.
		// Under an inline parent, it goes inline.
		if parentIsBlock {
			writeIndent(buf, depth)
		}
		buf.WriteString(text)
		if parentIsBlock {
			buf.WriteByte('\n')
		}
		return nil
	}

	return nil
}

// writeIndent emits `depth` × 2 spaces.
func writeIndent(buf *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString("  ")
	}
}

// writeOpenTag emits `<name attr="v" ...>` (or `<name … />` for void).
func writeOpenTag(buf *bytes.Buffer, n *xhtml.Node, selfClose bool) {
	buf.WriteByte('<')
	buf.WriteString(strings.ToLower(n.Data))
	for _, a := range n.Attr {
		buf.WriteByte(' ')
		buf.WriteString(a.Key)
		if a.Val != "" {
			buf.WriteString(`="`)
			buf.WriteString(escapeAttrValue(a.Val))
			buf.WriteByte('"')
		}
	}
	if selfClose {
		buf.WriteString(" />")
	} else {
		buf.WriteByte('>')
	}
}

// escapeAttrValue escapes the three characters that must be escaped in
// an HTML attribute value: &, <, and ".
func escapeAttrValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// Small helpers
// -----------------------------------------------------------------------------

// trimTrailingNewlines removes trailing '\n' bytes.
func trimTrailingNewlines(b []byte) []byte {
	i := len(b)
	for i > 0 && b[i-1] == '\n' {
		i--
	}
	return b[:i]
}

// collapseBlankLines reduces runs of two-or-more newlines to a single
// newline. Used only on the fallback path, when we could not produce
// a full pretty-print.
func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n") {
		s = strings.ReplaceAll(s, "\n\n", "\n")
	}
	return strings.TrimRight(s, "\n") + "\n"
}

// hasUnclosedTags does a cheap scan for any open tag without a matching
// close — a heuristic that lets us warn the user their input relied on
// the parser's auto-close behaviour. We do not need this to be perfect;
// a false positive only means a warning that did not need to appear.
func hasUnclosedTags(b []byte) bool {
	s := strings.ToLower(string(b))
	// Rough heuristic: if the number of `<div` occurrences exceeds the
	// number of `</div` occurrences, at least one div was unclosed.
	// Repeat for <p>, <li>, <span>.
	pairs := [][2]string{
		{"<div", "</div"},
		{"<p", "</p"},
		{"<span", "</span"},
		{"<li", "</li"},
	}
	for _, p := range pairs {
		if strings.Count(s, p[0]) > strings.Count(s, p[1]) {
			return true
		}
	}
	return false
}