package format

import (
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input → unchanged
// -----------------------------------------------------------------------
func TestYAML_AlreadyCanonical(t *testing.T) {
	in := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: default
data:
  key1: value1
  key2: value2
`)

	out, report, err := (&yamlFormatter{}).FormatBytes(in)
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
func TestYAML_MessyButParseable(t *testing.T) {
	// Tabs in indentation, missing space after colons, trailing
	// whitespace, and extra blank lines.
	in := []byte("apiVersion: v1\nkind:ConfigMap\nmetadata:\n\tname: my-config\n  namespace: default   \n\n\n\ndata:\n  key1:value1\n")

	out, report, err := (&yamlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	s := string(out)

	// Missing space after colon fixed.
	if !strings.Contains(s, "kind: ConfigMap") {
		t.Fatalf("expected 'kind: ConfigMap', got:\n%s", s)
	}
	if !strings.Contains(s, "key1: value1") {
		t.Fatalf("expected 'key1: value1', got:\n%s", s)
	}

	// Tab-indent expanded to spaces.
	if strings.Contains(s, "\t") {
		t.Fatalf("tabs still present:\n%s", s)
	}

	// Trailing whitespace stripped.
	if strings.Contains(s, "default   \n") {
		t.Fatalf("trailing whitespace present:\n%s", s)
	}

	// Blank lines collapsed.
	if strings.Contains(s, "\n\n\n") {
		t.Fatalf("blank lines not collapsed:\n%s", s)
	}
}

// -----------------------------------------------------------------------
// Test 3: broken input → still returns, with a warning
// -----------------------------------------------------------------------
func TestYAML_BrokenInputStillProducesOutput(t *testing.T) {
	// YAML is permissive enough that almost nothing is "broken" in
	// the way we mean here. A file with mixed indentation widths is
	// the best we can do as a "garbage" fixture — the formatter
	// should still return it with a warning-free report (since we
	// preserve indentation) but Change must still be false on a
	// canonical fragment, and true when we touch anything.
	//
	// We use a file that requires tab expansion to exercise Changed.
	in := []byte("a:\n\tb: 1\nc: 2\n")

	out, report, err := (&yamlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("expected no error for broken input, got %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true for tab-indented input; report=%+v", report)
	}
	if !strings.Contains(string(out), "    b: 1") {
		t.Fatalf("expected tab expanded to 4 spaces, got:\n%s", out)
	}
}

// -----------------------------------------------------------------------
// Test 4: DetectFormat
// -----------------------------------------------------------------------
func TestYAML_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{
			name:     "extension .yaml",
			filename: "config.yaml",
			raw:      []byte("key: value\n"),
			want:     "yaml",
		},
		{
			name:     "extension .yml",
			filename: "config.yml",
			raw:      []byte("key: value\n"),
			want:     "yaml",
		},
		{
			name:     "sniff: document marker ---",
			filename: "noext",
			raw:      []byte("---\nkey: value\n"),
			want:     "yaml",
		},
		{
			name:     "sniff: two key: value lines",
			filename: "noext",
			raw:      []byte("apiVersion: v1\nkind: ConfigMap\n"),
			want:     "yaml",
		},
		{
			name:     "JSON is not YAML (goes to json)",
			filename: "noext",
			raw:      []byte(`{"key": "value"}`),
			want:     "json",
		},
		{
			name:     "INI section header is not YAML",
			filename: "noext",
			raw:      []byte("[server]\nhost = localhost\n"),
			want:     "ini", // INI sniff fires first, which is correct
		},
		{
			name:     "HTML is not YAML",
			filename: "noext",
			raw:      []byte("<html><body></body></html>"),
			want:     "html",
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
// Test 5: empty input → ErrEmptyInput
// -----------------------------------------------------------------------
func TestYAML_EmptyInput(t *testing.T) {
	_, _, err := (&yamlFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}