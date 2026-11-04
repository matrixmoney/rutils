package format

import (
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input → unchanged
// -----------------------------------------------------------------------
func TestTOML_AlreadyCanonical(t *testing.T) {
	in := []byte(`[package]
name = "my-crate"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = "1.0"
tokio = { version = "1", features = ["full"] }
`)

	out, report, err := (&tomlFormatter{}).FormatBytes(in)
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
func TestTOML_MessyButParseable(t *testing.T) {
	// Missing spaces around `=`, inner-whitespace in table headers,
	// extra blank lines, and trailing whitespace.
	in := []byte("[package]\nname=\"my-crate\"\nversion   =\"0.1.0\"\n\n\n\n[ deps ]\nserde=\"1.0\"\n")

	out, report, err := (&tomlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	s := string(out)

	// Key-value spacing canonicalised.
	if !strings.Contains(s, `name = "my-crate"`) {
		t.Fatalf("expected 'name = \"my-crate\"', got:\n%s", s)
	}
	if !strings.Contains(s, `version = "0.1.0"`) {
		t.Fatalf("expected 'version = \"0.1.0\"', got:\n%s", s)
	}
	if !strings.Contains(s, `serde = "1.0"`) {
		t.Fatalf("expected 'serde = \"1.0\"', got:\n%s", s)
	}

	// Table header whitespace normalised.
	if !strings.Contains(s, "[deps]") {
		t.Fatalf("expected '[deps]', got:\n%s", s)
	}

	// Blank-line runs collapsed.
	if strings.Contains(s, "\n\n\n") {
		t.Fatalf("blank lines not collapsed:\n%s", s)
	}

	// Original key order preserved.
	nameIdx := strings.Index(s, "name")
	versionIdx := strings.Index(s, "version")
	if nameIdx < 0 || versionIdx < 0 || versionIdx < nameIdx {
		t.Fatalf("key order not preserved:\n%s", s)
	}
}

// -----------------------------------------------------------------------
// Test 3: garbage line is preserved, and real formatting still happens
// -----------------------------------------------------------------------
//
// The TOML formatter is deliberately conservative with lines that are
// not table headers, not key-value lines, and not inside a multi-line
// string: it preserves them verbatim without warning. This is the
// correct behaviour — a TOML file can legitimately contain lines that
// do not start with `[` or `=` (e.g. comment continuations, or
// fragments that the state machine has already classified as
// multi-line-string content).
//
// The test therefore uses a fixture where the formatter MUST make a
// change (missing spaces around `=` on real key-value lines) alongside
// a garbage line that must be preserved verbatim. This exercises both
// invariants at once:
//
//   - Changed == true because of the key-value spacing fix.
//   - The garbage line survives in the output.
func TestTOML_BrokenInputStillProducesOutput(t *testing.T) {
	in := []byte(`[section]
key="value"
this is not TOML syntax
another="one"
`)

	out, report, err := (&tomlFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("expected no error for broken input, got %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true (the key=\"value\" lines need spacing fixes); report=%+v", report)
	}
	if !strings.Contains(string(out), "this is not TOML syntax") {
		t.Fatalf("garbage line was dropped:\n%s", out)
	}
	// The real key-value lines should be canonicalised.
	if !strings.Contains(string(out), `key = "value"`) {
		t.Fatalf("expected 'key = \"value\"', got:\n%s", out)
	}
	if !strings.Contains(string(out), `another = "one"`) {
		t.Fatalf("expected 'another = \"one\"', got:\n%s", out)
	}
}

// -----------------------------------------------------------------------
// Test 4: DetectFormat
// -----------------------------------------------------------------------
func TestTOML_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{
			name:     "extension .toml",
			filename: "Cargo.toml",
			raw:      []byte("[package]\nname = \"x\"\n"),
			want:     "toml",
		},
		{
			name:     "sniff: leading [table] with key=value inside",
			filename: "noext",
			raw:      []byte("[package]\nname = \"x\"\nversion = \"0.1\"\n"),
			want:     "toml",
		},
		{
			name:     "sniff: two key=value lines with quotes",
			filename: "noext",
			raw:      []byte("name = \"alice\"\nage = 30\n"),
			want:     "toml",
		},
		{
			name:     "INI without quotes is INI, not TOML",
			filename: "noext",
			raw:      []byte("[server]\nhost = localhost\nport = 8080\n"),
			want:     "ini",
		},
		{
			name:     "YAML is not TOML",
			filename: "noext",
			raw:      []byte("name: alice\nage: 30\n"),
			want:     "yaml",
		},
		{
			name:     "JSON is not TOML (goes to json)",
			filename: "noext",
			raw:      []byte(`{"key": "value"}`),
			want:     "json",
		},
		{
			name:     "HTML is not TOML",
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
func TestTOML_EmptyInput(t *testing.T) {
	_, _, err := (&tomlFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}