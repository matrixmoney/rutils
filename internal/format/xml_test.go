package format

import (
	"bytes"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input вҶ’ unchanged
// -----------------------------------------------------------------------
func TestXML_AlreadyCanonical(t *testing.T) {
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<root>
  <a>1</a>
  <b>2</b>
</root>
`)

	out, report, err := (&xmlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Changed {
		t.Fatalf("expected Changed=false for canonical input; fixes=%v\n--- in ---\n%s\n--- out ---\n%s",
			report.Applied, in, out)
	}
	if !bytes.Equal(out, in) {
		t.Fatalf("byte mismatch:\n--- in ---\n%s\n--- out ---\n%s", in, out)
	}
}

// -----------------------------------------------------------------------
// Test 2: messy but parseable вҶ’ formatted
// -----------------------------------------------------------------------
func TestXML_MessyButParseable(t *testing.T) {
	in := []byte(`<root><a><b>1</b></a><c>2</c></root>`)

	out, report, err := (&xmlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	s := string(out)

	// Multi-line.
	if strings.Count(s, "\n") < 4 {
		t.Fatalf("expected multi-line output, got:\n%s", s)
	}

	// <a> must be indented under <root>.
	assertXMLIndentUnder(t, s, "<root>", "<a>")
	// <b> must be indented under <a>.
	assertXMLIndentUnder(t, s, "<a>", "<b>")

	// Whitespace-only text nodes dropped.
	if strings.Contains(s, "\n\n") {
		t.Fatalf("blank line found in output:\n%s", s)
	}
}

// -----------------------------------------------------------------------
// Test 3: broken input вҶ’ still returns, with a warning
// -----------------------------------------------------------------------
func TestXML_BrokenInputStillProducesOutput(t *testing.T) {
	in := []byte(`<root><a>1</b></root>`)

	out, report, err := (&xmlFormatter{}).FormatBytes(in)
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
// Test 4: unclosed tag auto-closed
// -----------------------------------------------------------------------
func TestXML_UnclosedTag(t *testing.T) {
	in := []byte(`<root><a>hello`)

	out, report, err := (&xmlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}
	if !strings.Contains(string(out), "</a>") {
		t.Fatalf("expected auto-closed </a>, got:\n%s", out)
	}
	if !strings.Contains(string(out), "</root>") {
		t.Fatalf("expected auto-closed </root>, got:\n%s", out)
	}
	hasUnclosedWarning := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "unclosed") {
			hasUnclosedWarning = true
			break
		}
	}
	if !hasUnclosedWarning {
		t.Fatalf("expected warning mentioning 'unclosed', got %v", report.Warnings)
	}
}

// -----------------------------------------------------------------------
// Test 5: DetectFormat
// -----------------------------------------------------------------------
func TestXML_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{"extension .xml", "doc.xml", []byte("<root/>"), "xml"},
		{"xml prolog", "noext", []byte(`<?xml version="1.0"?><root/>`), "xml"},
		{"xml prolog uppercase", "noext", []byte(`<?XML version="1.0"?><root/>`), "xml"},
		{"plain tag non-html", "noext", []byte("<root><a>1</a></root>"), "xml"},
		{"doctype non-html", "noext", []byte("<!DOCTYPE note><note/>"), "xml"},
		{"html is html", "noext", []byte("<!doctype html><html></html>"), "html"},
		{"html tag", "noext", []byte("<html><body></body></html>"), "html"},
		{"not xml", "noext", []byte("this is not xml"), ""},
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
// Test 6: empty input вҶ’ ErrEmptyInput
// -----------------------------------------------------------------------
func TestXML_EmptyInput(t *testing.T) {
	_, _, err := (&xmlFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func assertXMLIndentUnder(t *testing.T, doc, parent, child string) {
	t.Helper()
	parentLine := firstXMLLineContaining(doc, parent)
	if parentLine == "" {
		t.Fatalf("no line containing %q in output:\n%s", parent, doc)
	}
	childLine := firstXMLLineContaining(doc, child)
	if childLine == "" {
		t.Fatalf("no line containing %q in output:\n%s", child, doc)
	}
	pIndent := xmlLeadingSpaces(parentLine)
	cIndent := xmlLeadingSpaces(childLine)
	if cIndent <= pIndent {
		t.Fatalf(
			"expected %q (indent %d) to be more indented than %q (indent %d)\noutput:\n%s",
			child, cIndent, parent, pIndent, doc,
		)
	}
}

func firstXMLLineContaining(doc, substr string) string {
	for _, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

func xmlLeadingSpaces(line string) int {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}