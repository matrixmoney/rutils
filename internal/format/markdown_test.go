package format

import (
	"bytes"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input вҶ’ unchanged
// -----------------------------------------------------------------------
func TestMarkdown_AlreadyCanonical(t *testing.T) {
	in := []byte(`# Title

## Section

Some text here.

` + "```go\nfmt.Println(\"hello\")\n```\n" + `
Another paragraph.
`)

	out, report, err := (&markdownFormatter{}).FormatBytes(in)
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
func TestMarkdown_MessyButParseable(t *testing.T) {
	in := []byte("#Title\n\n\n\n##Section  \n\nSome text here.\n")

	out, report, err := (&markdownFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	s := string(out)

	// ATX headings must have a space after the hashes.
	if !strings.Contains(s, "# Title") {
		t.Fatalf("expected '# Title', got:\n%s", s)
	}
	if !strings.Contains(s, "## Section") {
		t.Fatalf("expected '## Section', got:\n%s", s)
	}
	if strings.Contains(s, "#Title") {
		t.Fatalf("'#Title' should have been fixed:\n%s", s)
	}

	// Trailing whitespace stripped.
	if strings.Contains(s, "## Section  ") {
		t.Fatalf("trailing whitespace not stripped:\n%s", s)
	}

	// Runs of 3+ blank lines collapsed.
	if strings.Contains(s, "\n\n\n\n") {
		t.Fatalf("blank line run not collapsed:\n%s", s)
	}

	// Exactly one trailing newline.
	if !strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\n\n") {
		t.Fatalf("expected exactly one trailing newline, got:\n%q", s)
	}
}

// -----------------------------------------------------------------------
// Test 3: code fences preserved verbatim
// -----------------------------------------------------------------------
func TestMarkdown_CodeFencesPreserved(t *testing.T) {
	// The code block contains irregular whitespace, leading/trailing
	// spaces, and blank lines. None of it must be touched.
	in := []byte(`# Title

` + "```python\ndef foo():\n    x = 1\n\n    return   x + 2\n```\n" + `
Done.
`)

	out, report, err := (&markdownFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = report

	s := string(out)

	// The code block must be preserved byte-for-byte (aside from
	// trailing whitespace which we do strip even in fences вҖ”
	// CommonMark ignores trailing whitespace inside fences for
	// rendering, so this is safe).
	if !strings.Contains(s, "    x = 1") {
		t.Fatalf("code indent lost:\n%s", s)
	}
	if !strings.Contains(s, "    return   x + 2") {
		t.Fatalf("code whitespace lost:\n%s", s)
	}
	if !strings.Contains(s, "```python") {
		t.Fatalf("fence opener lost:\n%s", s)
	}
}

// -----------------------------------------------------------------------
// Test 4: DetectFormat
// -----------------------------------------------------------------------
func TestMarkdown_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{
			name:     "extension .md",
			filename: "README.md",
			raw:      []byte("# Title\n"),
			want:     "md",
		},
		{
			name:     "extension .markdown",
			filename: "notes.markdown",
			raw:      []byte("# Title\n"),
			want:     "md",
		},
		{
			name:     "heading sniff",
			filename: "noext",
			raw:      []byte("# Title\n\nParagraph.\n"),
			want:     "md",
		},
		{
			name:     "fence sniff",
			filename: "noext",
			raw:      []byte("```go\nfmt.Println(\"x\")\n```\n"),
			want:     "md",
		},
		{
			name:     "blockquote sniff",
			filename: "noext",
			raw:      []byte("> A quoted paragraph.\n"),
			want:     "md",
		},
		{
			name:     "plain prose is not markdown",
			filename: "noext",
			raw:      []byte("This is just plain prose with no markdown syntax at all.\n"),
			want:     "",
		},
		{
			name:     "html is not markdown",
			filename: "noext",
			raw:      []byte("<html><body></body></html>"),
			want:     "html",
		},
		{
			name:     "json is not markdown",
			filename: "noext",
			raw:      []byte(`{"a":1}`),
			want:     "json",
		},
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
// Test 5: empty input вҶ’ ErrEmptyInput
// -----------------------------------------------------------------------
func TestMarkdown_EmptyInput(t *testing.T) {
	_, _, err := (&markdownFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}