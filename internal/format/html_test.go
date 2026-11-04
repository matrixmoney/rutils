package format

import (
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input → unchanged
// -----------------------------------------------------------------------
// A document that is already in the formatter's canonical shape must
// come back byte-for-byte identical, with Changed=false.
//
// The fixture below is the exact output our indenter produces for a
// simple document:
//   - every block element on its own line
//   - text children indented under their block parent
//   - close tags on their own lines
//   - empty block elements on a single line
// If we feed that back in, the formatter should not change it.
func TestHTML_AlreadyCanonical(t *testing.T) {
	in := []byte(`<!DOCTYPE html>
<html>
  <head></head>
  <body>
    <p>
      Hello
    </p>
  </body>
</html>
`)

	out, report, err := (&htmlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Changed {
		t.Fatalf("expected Changed=false for canonical input; fixes=%v\n--- in ---\n%s\n--- out ---\n%s",
			report.Applied, in, out)
	}
	if string(out) != string(in) {
		t.Fatalf("byte mismatch:\n--- in ---\n%s\n--- out ---\n%s", in, out)
	}
}

// -----------------------------------------------------------------------
// Test 2: messy but parseable → formatted
// -----------------------------------------------------------------------
// A single-line HTML fragment should be rewritten into multi-line
// indented form. We assert on the structure — each block element on its
// own line, text indented beneath its block parent — not on the exact
// byte sequence, because x/net/html injects <html><head><body> when it
// sees a fragment.
func TestHTML_MessyButParseable(t *testing.T) {
	in := []byte(`<div><p>Hello</p><ul><li>One</li><li>Two</li></ul></div>`)

	out, report, err := (&htmlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	s := string(out)

	// Each block element must appear on its own line, and each must be
	// more indented than its parent. We check the nesting chain:
	//   <div>
	//     <p>
	//       Hello
	//     </p>
	//     <ul>
	//       <li>
	//         One
	//       </li>
	// ...
	assertLineIndentedUnder(t, s, "<div>", "<p>")
	assertLineIndentedUnder(t, s, "<p>", "Hello")
	assertSameIndent(t, s, "<p>", "</p>")
	assertLineIndentedUnder(t, s, "<div>", "<ul>")
	assertLineIndentedUnder(t, s, "<ul>", "<li>")
	assertLineIndentedUnder(t, s, "<li>", "One")
	assertSameIndent(t, s, "<li>", "</li>")

	// "Hello" must be on its own line, not glued to <p> or </p>.
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "<p>Hello") {
			t.Fatalf("<p> and Hello must be on separate lines; got line: %q", line)
		}
		if strings.Contains(trim, "Hello</p>") {
			t.Fatalf("Hello and </p> must be on separate lines; got line: %q", line)
		}
	}
}

// -----------------------------------------------------------------------
// Test 3: unparseable-ish input → still returns, with a warning
// -----------------------------------------------------------------------
// x/net/html almost never rejects input, but we check that the
// formatter does not surface a 4xx-style error when the input is
// obviously broken. The result must be a byte slice and a Report
// with at least one warning.
func TestHTML_BrokenInputStillProducesOutput(t *testing.T) {
	in := []byte(`<div class=foo bar"><p>Hi</span><ul><li>x</ul>`)

	out, report, err := (&htmlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("expected no error for broken input, got %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true for broken input; report=%+v", report)
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected at least one warning; report=%+v", report)
	}
}

// -----------------------------------------------------------------------
// Test 4: DetectFormat
// -----------------------------------------------------------------------
func TestHTML_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{"extension .html", "page.html", []byte("<p>x</p>"), "html"},
		{"extension .htm", "page.htm", []byte("<p>x</p>"), "html"},
		{"doctype sniff", "noext", []byte("<!doctype html><html></html>"), "html"},
		{"html tag sniff", "noext", []byte("<html><body></body></html>"), "html"},
		{"any tag sniff", "noext", []byte("<div>x</div>"), "html"},
		{"not html", "noext", []byte("this is not html"), ""},
		{"xml prolog is not html", "noext", []byte(`<?xml version="1.0"?><root/>`), "xml"},
	}

	eng := NewEngine()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := eng.DetectFormat(tc.filename, tc.raw)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------
// Test 5: empty input → ErrEmptyInput
// -----------------------------------------------------------------------
func TestHTML_EmptyInput(t *testing.T) {
	_, _, err := (&htmlFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}

// -----------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------

// assertLineIndentedUnder asserts that some line containing `child`
// exists and is indented strictly more than some line containing
// `parent`. Reports the full output on failure so the reader can see
// what actually happened.
func assertLineIndentedUnder(t *testing.T, doc, parent, child string) {
	t.Helper()

	parentLine := firstLineContaining(doc, parent)
	if parentLine == "" {
		t.Fatalf("no line containing %q in output:\n%s", parent, doc)
	}
	childLine := firstLineContaining(doc, child)
	if childLine == "" {
		t.Fatalf("no line containing %q in output:\n%s", child, doc)
	}

	pIndent := leadingSpaces(parentLine)
	cIndent := leadingSpaces(childLine)

	// For text nodes ("Hello", "One") we require strictly greater
	// indentation. For sibling elements at the same depth we accept
	// equal — assertLineIndentedUnder is called with the correct pair
	// in each case, so if a call reaches here it should be an
	// inequality.
	if cIndent <= pIndent {
		t.Fatalf(
			"expected %q (indent %d) to be more indented than %q (indent %d)\noutput:\n%s",
			child, cIndent, parent, pIndent, doc,
		)
	}
}
// assertSameIndent asserts that the lines containing `a` and `b` are
// indented to the same depth. Used for open-tag / close-tag pairs and
// for sibling elements — these sit at the same nesting level, so their
// leading spaces must match exactly.
func assertSameIndent(t *testing.T, doc, a, b string) {
	t.Helper()

	aLine := firstLineContaining(doc, a)
	if aLine == "" {
		t.Fatalf("no line containing %q in output:\n%s", a, doc)
	}
	bLine := firstLineContaining(doc, b)
	if bLine == "" {
		t.Fatalf("no line containing %q in output:\n%s", b, doc)
	}

	aIndent := leadingSpaces(aLine)
	bIndent := leadingSpaces(bLine)
	if aIndent != bIndent {
		t.Fatalf(
			"expected %q (indent %d) and %q (indent %d) to be at the same depth\noutput:\n%s",
			a, aIndent, b, bIndent, doc,
		)
	}
}
// firstLineContaining returns the first line of doc that contains
// substr, or "" if none does.
func firstLineContaining(doc, substr string) string {
	for _, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

// leadingSpaces counts leading ' ' on a line.
func leadingSpaces(line string) int {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}