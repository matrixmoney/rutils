package format

import (
	"encoding/csv"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input вҶ’ unchanged
// -----------------------------------------------------------------------
func TestCSV_AlreadyCanonical(t *testing.T) {
	in := []byte("id,name,age\n1,Alice,30\n2,Bob,25\n")

	out, report, err := (&csvFormatter{}).FormatBytes(in)
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
// Test 2: messy but parseable вҶ’ formatted
// -----------------------------------------------------------------------
func TestCSV_MessyButParseable(t *testing.T) {
	in := []byte("id,name,age\r\n1, Alice ,30\r\n2, Bob ,25\r\n")

	out, report, err := (&csvFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	// Output must end with exactly one newline.
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatalf("expected trailing newline, got: %q", out)
	}
	if strings.HasSuffix(string(out), "\n\n") {
		t.Fatalf("expected exactly one trailing newline, got double: %q", out)
	}

	// Output must have no \r.
	if strings.Contains(string(out), "\r") {
		t.Fatalf("CRLF not normalised: %q", out)
	}
}

// -----------------------------------------------------------------------
// Test 3: ragged short row вҶ’ padded with warning
// -----------------------------------------------------------------------
func TestCSV_ShortRowPadded(t *testing.T) {
	in := []byte("id,name,age\n1,Alice\n2,Bob,25\n")

	out, report, err := (&csvFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	hasPaddingFix := false
	for _, f := range report.Applied {
		if f == "rows-padded" {
			hasPaddingFix = true
			break
		}
	}
	if !hasPaddingFix {
		t.Fatalf("expected 'rows-padded' fix, got %v", report.Applied)
	}

	hasPaddingWarning := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "padded") {
			hasPaddingWarning = true
			break
		}
	}
	if !hasPaddingWarning {
		t.Fatalf("expected warning about padded rows, got %v", report.Warnings)
	}

	// Every row must now have 3 fields.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		rec, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil {
			t.Fatalf("output row unreadable: %v", err)
		}
		if len(rec) != 3 {
			t.Fatalf("expected 3 fields, got %d (%q)", len(rec), line)
		}
	}
}

// -----------------------------------------------------------------------
// Test 4: ragged long row вҶ’ truncated with warning
// -----------------------------------------------------------------------
func TestCSV_LongRowTruncated(t *testing.T) {
	in := []byte("id,name\n1,Alice,extra1,extra2\n")

	out, report, err := (&csvFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	hasTruncFix := false
	for _, f := range report.Applied {
		if f == "rows-truncated" {
			hasTruncFix = true
			break
		}
	}
	if !hasTruncFix {
		t.Fatalf("expected 'rows-truncated' fix, got %v", report.Applied)
	}

	// Every row must have exactly 2 fields.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		rec, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil {
			t.Fatalf("output row unreadable: %v", err)
		}
		if len(rec) != 2 {
			t.Fatalf("expected 2 fields, got %d (%q)", len(rec), line)
		}
	}
}

// -----------------------------------------------------------------------
// Test 5: quoted delimiter preserved
// -----------------------------------------------------------------------
func TestCSV_QuotedDelimiterPreserved(t *testing.T) {
	in := []byte("id,name\n1,\"Doe, John\"\n")

	out, report, err := (&csvFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = report

	if !strings.Contains(string(out), "Doe, John") {
		t.Fatalf("quoted delimiter lost: %q", out)
	}
	// Must still be parseable.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		rec, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil {
			t.Fatalf("output row unreadable: %v", err)
		}
		if len(rec) != 2 {
			t.Fatalf("expected 2 fields, got %d (%q)", len(rec), line)
		}
	}
}

// -----------------------------------------------------------------------
// Test 6: semicolon delimiter detected and preserved
// -----------------------------------------------------------------------
func TestCSV_SemicolonDelimiter(t *testing.T) {
	in := []byte("a;b;c\n1;2;3\n4;5;6\n")

	out, report, err := (&csvFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = report

	// Output should still use semicolons.
	if !strings.Contains(string(out), "1;2;3") {
		t.Fatalf("semicolons lost: %q", out)
	}
}

// -----------------------------------------------------------------------
// Test 7: DetectFormat
// -----------------------------------------------------------------------
func TestCSV_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{"extension .csv", "data.csv", []byte("a,b\n1,2\n"), "csv"},
		{"comma-delimited", "noext", []byte("a,b,c\n1,2,3\n4,5,6\n"), "csv"},
		{"semicolon-delimited", "noext", []byte("a;b;c\n1;2;3\n4;5;6\n"), "csv"},
		{"tab-delimited", "noext", []byte("a\tb\tc\n1\t2\t3\n4\t5\t6\n"), "csv"},
		{"ini has equals", "noext", []byte("[s]\na=1\nb=2\n"), "ini"},
		{"plain text", "noext", []byte("just plain text with no structure"), ""},
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
// Test 8: empty input вҶ’ ErrEmptyInput
// -----------------------------------------------------------------------
func TestCSV_EmptyInput(t *testing.T) {
	_, _, err := (&csvFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}