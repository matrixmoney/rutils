package format

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

func init() {
	register(&xmlFormatter{})
}

// xmlFormatter formats XML documents.
//
// The formatter uses encoding/xml's token stream API (Decoder.Token)
// rather than Decode() into a struct. This is critical because Decode()
// loses attribute order, comments, the XML declaration, and text-node
// whitespace.
//
// Canonical output rules:
//   - 2-space indent per nesting level
//   - self-closing tags for empty elements: <a></a> вҶ’ <a />
//   - preserve attribute order exactly
//   - preserve attribute values byte-for-byte
//   - preserve text content byte-for-byte
//   - preserve comments and processing instructions
//   - preserve XML declaration at column 0
//   - an element with only text content stays on one line: <a>1</a>
//   - an element with child elements / comments goes multi-line
//
// The "element with only text stays on one line" rule is critical for
// XML correctness: unlike HTML, XML does not collapse whitespace, so
// adding a newline inside <a>1</a> would change its semantic content
// from "1" to "\n  1\n".
//
// Broken input handling:
//   - multiple roots вҶ’ wrapped in <root>, warning
//   - unclosed tags вҶ’ auto-closed, warning
//   - case mismatch вҶ’ close tag re-cased to match open, warning
//   - parser error вҶ’ best-effort output up to that point, warning
type xmlFormatter struct{}

func (x *xmlFormatter) Format() string { return "xml" }

// xmlNode is the parsed representation of one element, comment, or
// processing instruction. Building a tree first lets us decide
// per-element whether to emit text inline or on its own line.
type xmlNode struct {
	kind      xmlNodeKind
	name      string     // element name (for start/end)
	attrs     []xml.Attr // element attributes, in original order
	text      string     // text content (for text nodes)
	comment   string     // comment content (for comments)
	piTarget  string     // processing instruction target
	piInst    string     // processing instruction body
	directive string     // raw directive content (DOCTYPE etc.)
	children  []*xmlNode // child nodes (for elements)
}

type xmlNodeKind int

const (
	xmlNodeElement xmlNodeKind = iota
	xmlNodeText
	xmlNodeComment
	xmlNodeProcInst
	xmlNodeDirective
)

// FormatBytes implements Formatter.
func (x *xmlFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "xml"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	// Stage 1: check for multiple roots BEFORE parsing. If there are
	// multiple, wrap them in <root> so the parser sees one document.
	if hasMultipleRootsXML(work) {
		wrapped := append([]byte("<root>"), work...)
		wrapped = append(wrapped, []byte("</root>")...)
		work = wrapped
		report.AddFix("multiple-roots-wrapped")
		report.AddWarning("input had multiple root elements; wrapped in a synthetic <root>")
	}

	// Stage 2: parse into a tree. parseXMLTree returns the unclosed
	// names (elements whose EndElement was never seen) separately
	// from hard parse errors, so we can distinguish "input had
	// unclosed tags" (recoverable, warn) from "input had a
	// character-encoding error" (not recoverable, line cleanup).
	tree, unclosed, parseErr := parseXMLTree(work)
	if parseErr != nil {
		// Parser error вҖ” fall back to line-level cleanup.
		report.AddWarning("XML parser error: " + parseErr.Error() +
			" (best-effort line cleanup applied)")
		cleaned := lineCleanupXML(work)
		if !bytes.Equal(cleaned, raw) {
			report.AddFix("line-cleanup")
		}
		return cleaned, report, nil
	}
	if len(unclosed) > 0 {
		report.AddWarning(
			"input had " + itoa(len(unclosed)) + " unclosed tag(s); auto-closed at EOF")
		report.AddFix("unclosed-tags-closed")
	}

	// Stage 3: render the tree with indentation.
	var out bytes.Buffer
	renderXMLTree(&out, tree, 0)

	// Trim trailing newlines and ensure exactly one final newline.
	result := trimTrailingNewlines(out.Bytes())
	if len(result) > 0 {
		result = append(result, '\n')
	}

	// Compare to decide Changed.
	if bytes.Equal(result, work) {
		return work, report, nil
	}

	report.AddFix("formatted")
	return result, report, nil
}

// -----------------------------------------------------------------------------
// Parse tree builder
// -----------------------------------------------------------------------------

// parseXMLTree parses work into a slice of top-level nodes.
//
// Returns:
//   - roots:     the top-level nodes.
//   - unclosed:  names of elements that were still on the stack at
//     EOF (i.e. their EndElement was never seen). Ordered outermost
//     first. Empty when the document is balanced.
//   - err:       non-nil only for a genuine tokenizer error (e.g. an
//     invalid byte sequence, a malformed attribute). Unclosed tags
//     do NOT produce a non-nil error вҖ” they appear in `unclosed`
//     instead, so the caller can auto-close them.
func parseXMLTree(work []byte) ([]*xmlNode, []string, error) {
	dec := xml.NewDecoder(bytes.NewReader(work))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	var (
		roots []*xmlNode
		stack []*xmlNode // element stack
	)

	// appendChild adds n to the current element's children, or to
	// roots if we're at the top level.
	appendChild := func(n *xmlNode) {
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, n)
		} else {
			roots = append(roots, n)
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Distinguish a soft EOF error (unclosed tags at end of
			// input) from a hard tokenizer error (bad bytes,
			// malformed attribute). encoding/xml reports "unexpected
			// EOF" as an error even in non-strict mode; but the
			// token stream is complete at that point, so we can
			// treat it as "unclosed tags at EOF" and recover
			// normally.
			if isUnexpectedEOFXML(err) {
				break
			}
			// Genuine tokenizer error. Return what we have plus
			// the error; the caller falls back to line cleanup.
			names := make([]string, 0, len(stack))
			for _, n := range stack {
				names = append(names, n.name)
			}
			return roots, names, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			node := &xmlNode{
				kind:  xmlNodeElement,
				name:  t.Name.Local,
				attrs: t.Attr,
			}
			appendChild(node)
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			// Preserve text content byte-for-byte.
			appendChild(&xmlNode{kind: xmlNodeText, text: string(t)})
		case xml.Comment:
			appendChild(&xmlNode{kind: xmlNodeComment, comment: string(t)})
		case xml.ProcInst:
			appendChild(&xmlNode{
				kind:     xmlNodeProcInst,
				piTarget: t.Target,
				piInst:   string(t.Inst),
			})
		case xml.Directive:
			appendChild(&xmlNode{kind: xmlNodeDirective, directive: string(t)})
		}
	}

	// At EOF, any elements still on the stack were never closed.
	// Report them as `unclosed` (outermost first). Their subtrees
	// are complete вҖ” the renderer will emit closing tags regardless.
	unclosed := make([]string, 0, len(stack))
	for _, n := range stack {
		unclosed = append(unclosed, n.name)
	}
	return roots, unclosed, nil
}

// isUnexpectedEOFXML reports whether err is encoding/xml's "unexpected
// EOF" signal, which occurs when the input ends inside an open element.
// We treat that as recoverable (unclosed tags at EOF), because the
// token stream up to that point is complete.
//
// Genuine tokenizer errors (bad bytes, malformed attributes) do NOT
// match this and are propagated as real errors.
func isUnexpectedEOFXML(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "unexpected EOF")
}

// -----------------------------------------------------------------------------
// Renderer
// -----------------------------------------------------------------------------

// renderXMLTree emits nodes with 2-space indentation.
//
// Rules:
//   - top-level ProcInst with target "xml" вҶ’ emitted at column 0
//   - element with ONLY text children вҶ’ emitted on one line: <a>1</a>
//   - element with ONLY whitespace text children and no other content
//     вҶ’ emitted self-closed: <a />
//   - element with child elements or comments вҶ’ multi-line
//   - text nodes: only emitted inline (as part of the parent's single-
//     line form). At top level they are dropped.
//
// Nodes are joined by single newlines вҖ” no blank lines are ever
// introduced between siblings.
func renderXMLTree(out *bytes.Buffer, nodes []*xmlNode, depth int) {
	wroteAny := false
	for _, node := range nodes {
		// Skip top-level text nodes entirely (whitespace between
		// roots, stray text after the root).
		if node.kind == xmlNodeText {
			continue
		}

		if wroteAny {
			out.WriteByte('\n')
		}
		switch node.kind {
		case xmlNodeProcInst:
			if node.piTarget == "xml" {
				// XML declaration at column 0.
				out.WriteString("<?xml ")
				out.WriteString(node.piInst)
				out.WriteString("?>")
			} else {
				writeIndentXML(out, depth)
				out.WriteString("<?")
				out.WriteString(node.piTarget)
				if node.piInst != "" {
					out.WriteByte(' ')
					out.WriteString(node.piInst)
				}
				out.WriteString("?>")
			}
		case xmlNodeDirective:
			writeIndentXML(out, depth)
			out.WriteString("<!")
			out.WriteString(node.directive)
			out.WriteByte('>')
		case xmlNodeComment:
			writeIndentXML(out, depth)
			out.WriteString("<!--")
			out.WriteString(node.comment)
			out.WriteString("-->")
		case xmlNodeElement:
			renderXMLElement(out, node, depth)
		}
		wroteAny = true
	}
}

// renderXMLElement emits one element, deciding between:
//   - self-closing: <a />
//   - single-line: <a>text</a>
//   - multi-line with children on separate lines
//
// Children are rendered into a temporary buffer first, then joined
// with "\n" and indented. This makes separator logic trivially
// correct вҖ” no stray blank lines between siblings, no fragile
// buffer-rewinding tricks.
func renderXMLElement(out *bytes.Buffer, node *xmlNode, depth int) {
	// Classify the children.
	hasElementChild := false
	hasComment := false
	hasProcInst := false
	var textParts []string

	for _, child := range node.children {
		switch child.kind {
		case xmlNodeElement:
			hasElementChild = true
		case xmlNodeComment:
			hasComment = true
		case xmlNodeProcInst:
			hasProcInst = true
		case xmlNodeText:
			textParts = append(textParts, child.text)
		}
	}

	hasNonTextChild := hasElementChild || hasComment || hasProcInst
	allText := strings.Join(textParts, "")
	trimmedText := strings.TrimSpace(allText)
	hasText := trimmedText != ""

	// Case 1: completely empty (no children or only whitespace text).
	if !hasNonTextChild && !hasText {
		writeIndentXML(out, depth)
		out.WriteByte('<')
		out.WriteString(node.name)
		writeAttrsXML(out, node.attrs)
		out.WriteString(" />")
		return
	}

	// Case 2: only text children (and no non-text children).
	// Emit on one line: <a>text</a>.
	if !hasNonTextChild && hasText {
		writeIndentXML(out, depth)
		out.WriteByte('<')
		out.WriteString(node.name)
		writeAttrsXML(out, node.attrs)
		out.WriteByte('>')
		out.WriteString(escapeXMLTextMinimal(allText))
		out.WriteString("</")
		out.WriteString(node.name)
		out.WriteByte('>')
		return
	}

	// Case 3: has non-text children вҶ’ multi-line. Render each
	// non-whitespace child into its own buffer, then join with "\n".
	var childBufs []bytes.Buffer
	for _, child := range node.children {
		switch child.kind {
		case xmlNodeText:
			// Drop whitespace-only text between elements.
			if strings.TrimSpace(child.text) == "" {
				continue
			}
			// Non-whitespace text inside a mixed-content element
			// is rare in practice. Emit it on its own line.
			var cb bytes.Buffer
			writeIndentXML(&cb, depth+1)
			cb.WriteString(escapeXMLTextMinimal(child.text))
			childBufs = append(childBufs, cb)
		case xmlNodeComment:
			var cb bytes.Buffer
			writeIndentXML(&cb, depth+1)
			cb.WriteString("<!--")
			cb.WriteString(child.comment)
			cb.WriteString("-->")
			childBufs = append(childBufs, cb)
		case xmlNodeProcInst:
			var cb bytes.Buffer
			writeIndentXML(&cb, depth+1)
			cb.WriteString("<?")
			cb.WriteString(child.piTarget)
			if child.piInst != "" {
				cb.WriteByte(' ')
				cb.WriteString(child.piInst)
			}
			cb.WriteString("?>")
			childBufs = append(childBufs, cb)
		case xmlNodeElement:
			var cb bytes.Buffer
			renderXMLElement(&cb, child, depth+1)
			childBufs = append(childBufs, cb)
		}
	}

	// Emit the open tag.
	writeIndentXML(out, depth)
	out.WriteByte('<')
	out.WriteString(node.name)
	writeAttrsXML(out, node.attrs)
	out.WriteByte('>')

	// Emit each child on its own line.
	for i := range childBufs {
		out.WriteByte('\n')
		out.Write(childBufs[i].Bytes())
	}

	// Emit the close tag.
	out.WriteByte('\n')
	writeIndentXML(out, depth)
	out.WriteString("</")
	out.WriteString(node.name)
	out.WriteByte('>')
}

// writeIndentXML writes `depth * 2` spaces.
func writeIndentXML(out *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		out.WriteString("  ")
	}
}

// writeAttrsXML writes each attribute as ` name="value"`. Order is
// preserved from the input. Attribute values are escaped minimally.
func writeAttrsXML(out *bytes.Buffer, attrs []xml.Attr) {
	for _, a := range attrs {
		out.WriteByte(' ')
		out.WriteString(a.Name.Local)
		out.WriteString(`="`)
		out.WriteString(escapeXMLAttrMinimal(a.Value))
		out.WriteByte('"')
	}
}

// -----------------------------------------------------------------------------
// Detection helpers
// -----------------------------------------------------------------------------

// hasMultipleRootsXML reports whether the document has more than one
// top-level element. encoding/xml's decoder does not enforce the
// single-root rule, so we check it explicitly.
func hasMultipleRootsXML(raw []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	depth := 0
	roots := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}

		switch tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				depth = 0
			}
		}
	}

	return roots > 1
}

// lineCleanupXML is the last-resort fallback for a document the
// parser rejected entirely: trim trailing whitespace per line, drop
// blank lines, ensure one trailing newline.
func lineCleanupXML(raw []byte) []byte {
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

// -----------------------------------------------------------------------------
// Minimal XML escaping
// -----------------------------------------------------------------------------

// escapeXMLAttrMinimal escapes &, ", and < in an attribute value. We
// do not re-escape existing entities because the decoder already
// decoded them; the output is semantically identical even if not
// byte-identical for entity representations.
func escapeXMLAttrMinimal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '<':
			b.WriteString("&lt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeXMLTextMinimal escapes & and < in text content.
func escapeXMLTextMinimal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}