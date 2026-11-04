package format

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input вҶ’ unchanged
// -----------------------------------------------------------------------
func TestJSON_AlreadyCanonical(t *testing.T) {
	in := []byte(`{
  "name": "Alice",
  "age": 30
}
`)

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
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
func TestJSON_MessyButParseable(t *testing.T) {
	in := []byte(`{"name":"Alice","age":30,"tags":["a","b"]}`)

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	// Output must be indented.
	if !strings.Contains(string(out), "\n  \"name\"") {
		t.Fatalf("expected 2-space indented output, got:\n%s", out)
	}

	// Output must be valid JSON.
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON:\n%s", out)
	}
}

// -----------------------------------------------------------------------
// Test 3: trailing comma stripped (best-effort fix)
// -----------------------------------------------------------------------
func TestJSON_TrailingComma(t *testing.T) {
	in := []byte(`{"name":"Alice","age":30,}`)

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON:\n%s", out)
	}

	hasTrailingCommaFix := false
	for _, f := range report.Applied {
		if f == "trailing-comma" {
			hasTrailingCommaFix = true
			break
		}
	}
	if !hasTrailingCommaFix {
		t.Fatalf("expected 'trailing-comma' fix, got %v", report.Applied)
	}
}

// -----------------------------------------------------------------------
// Test 4: comments stripped (best-effort fix)
// -----------------------------------------------------------------------
func TestJSON_Comments(t *testing.T) {
	in := []byte(`{
  // leading comment
  "name": "Alice",
  "age": 30 /* inline */
}`)

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON:\n%s", out)
	}

	hasCommentsFix := false
	for _, f := range report.Applied {
		if f == "comments-stripped" {
			hasCommentsFix = true
			break
		}
	}
	if !hasCommentsFix {
		t.Fatalf("expected 'comments-stripped' fix, got %v", report.Applied)
	}
}

// -----------------------------------------------------------------------
// Test 5: single quotes converted
// -----------------------------------------------------------------------
func TestJSON_SingleQuotes(t *testing.T) {
	in := []byte(`{'name':'Alice','age':30}`)

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON:\n%s", out)
	}

	hasSingleQuotesFix := false
	for _, f := range report.Applied {
		if f == "single-quotes" {
			hasSingleQuotesFix = true
			break
		}
	}
	if !hasSingleQuotesFix {
		t.Fatalf("expected 'single-quotes' fix, got %v", report.Applied)
	}
}

// -----------------------------------------------------------------------
// Test 6: unquoted keys quoted
// -----------------------------------------------------------------------
func TestJSON_UnquotedKeys(t *testing.T) {
	in := []byte(`{name:"Alice",age:30}`)

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON:\n%s", out)
	}

	hasUnquotedFix := false
	for _, f := range report.Applied {
		if f == "unquoted-keys" {
			hasUnquotedFix = true
			break
		}
	}
	if !hasUnquotedFix {
		t.Fatalf("expected 'unquoted-keys' fix, got %v", report.Applied)
	}
}

// -----------------------------------------------------------------------
// Test 7: key order preserved
// -----------------------------------------------------------------------
func TestJSON_KeyOrderPreserved(t *testing.T) {
	in := []byte(`{"z":1,"a":2,"m":3}`)

	out, _, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s := string(out)
	zIdx := strings.Index(s, `"z"`)
	aIdx := strings.Index(s, `"a"`)
	mIdx := strings.Index(s, `"m"`)

	if zIdx < 0 || aIdx < 0 || mIdx < 0 {
		t.Fatalf("one of the keys is missing:\n%s", s)
	}
	if !(zIdx < aIdx && aIdx < mIdx) {
		t.Fatalf("key order not preserved: z=%d, a=%d, m=%d\noutput:\n%s", zIdx, aIdx, mIdx, s)
	}
}

// -----------------------------------------------------------------------
// Test 8: number formatting preserved (1.0 stays 1.0)
// -----------------------------------------------------------------------
func TestJSON_NumberFormattingPreserved(t *testing.T) {
	in := []byte(`{"x":1.0,"y":1e10}`)

	out, _, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, "1.0") {
		t.Fatalf("expected '1.0' preserved, got:\n%s", s)
	}
	if !strings.Contains(s, "1e10") {
		t.Fatalf("expected '1e10' preserved, got:\n%s", s)
	}
}

// -----------------------------------------------------------------------
// Test 9: unparseable JSON вҶ’ best-effort line cleanup + warning
// -----------------------------------------------------------------------
func TestJSON_UnparseableStillProducesOutput(t *testing.T) {
	in := []byte("this is definitely not json at all\nand neither is this\n")

	out, report, err := (&jsonFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("expected no error for unparseable input, got %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected at least one warning; report=%+v", report)
	}

	hasParseWarning := false
	for _, w := range report.Warnings {
		if strings.Contains(strings.ToLower(w), "parse") {
			hasParseWarning = true
			break
		}
	}
	if !hasParseWarning {
		t.Fatalf("expected a warning mentioning parsing, got %v", report.Warnings)
	}
}

// -----------------------------------------------------------------------
// Test 10: DetectFormat
// -----------------------------------------------------------------------
func TestJSON_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{"extension .json", "data.json", []byte(`{"a":1}`), "json"},
		{"object sniff", "noext", []byte(`{"a":1}`), "json"},
		{"array of objects", "noext", []byte(`[{"a":1}]`), "json"},
		{"array of numbers", "noext", []byte(`[1,2,3]`), "json"},
		{"ini section is not json", "noext", []byte("[section]\nkey=value\n"), "ini"},
		{"plain text", "noext", []byte("this is plain text"), ""},
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
// Test 11: empty input вҶ’ ErrEmptyInput
// -----------------------------------------------------------------------
func TestJSON_EmptyInput(t *testing.T) {
	_, _, err := (&jsonFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}