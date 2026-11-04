package format

import (
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// Test 1: canonical input → unchanged
// -----------------------------------------------------------------------
func TestINI_AlreadyCanonical(t *testing.T) {
	in := []byte(`[server]
host = localhost
port = 8080

[database]
name = mydb
user = admin
`)

	out, report, err := (&iniFormatter{}).FormatBytes(in)
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
func TestINI_MessyButParseable(t *testing.T) {
	in := []byte(`[server]
host:localhost
port : 8080


[database]
name=mydb
user : admin
`)

	out, report, err := (&iniFormatter{}).FormatBytes(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Changed {
		t.Fatalf("expected Changed=true; report=%+v", report)
	}

	s := string(out)

	if !strings.Contains(s, "host = localhost") {
		t.Fatalf("expected 'host = localhost', got:\n%s", s)
	}
	if !strings.Contains(s, "port = 8080") {
		t.Fatalf("expected 'port = 8080', got:\n%s", s)
	}
	if !strings.Contains(s, "name = mydb") {
		t.Fatalf("expected 'name = mydb', got:\n%s", s)
	}
	if !strings.Contains(s, "user = admin") {
		t.Fatalf("expected 'user = admin', got:\n%s", s)
	}
	if strings.Contains(s, "\n\n\n") {
		t.Fatalf("blank lines not collapsed:\n%s", s)
	}
	if !strings.Contains(s, "[server]") || !strings.Contains(s, "[database]") {
		t.Fatalf("sections not preserved:\n%s", s)
	}
}

// -----------------------------------------------------------------------
// Test 3: garbage input → returns, with a warning
// -----------------------------------------------------------------------
func TestINI_BrokenInputStillProducesOutput(t *testing.T) {
	in := []byte(`[section]
key = value
this line is not valid INI at all
another = one
`)

	out, report, err := (&iniFormatter{}).FormatBytes(in)
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
	if !strings.Contains(string(out), "this line is not valid INI at all") {
		t.Fatalf("garbage line was dropped:\n%s", out)
	}
	hasLineWarning := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "line 3") {
			hasLineWarning = true
			break
		}
	}
	if !hasLineWarning {
		t.Fatalf("expected a warning mentioning line 3, got %v", report.Warnings)
	}
}

// -----------------------------------------------------------------------
// Test 4: DetectFormat
// -----------------------------------------------------------------------
func TestINI_DetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      []byte
		want     string
	}{
		{
			name:     "extension .ini",
			filename: "config.ini",
			raw:      []byte("[section]\nkey=value\n"),
			want:     "ini",
		},
		{
			name:     "sniff: section header first",
			filename: "noext",
			raw:      []byte("[server]\nhost=localhost\n"),
			want:     "ini",
		},
		{
			name:     "sniff: two key=value lines, no section",
			filename: "noext",
			raw:      []byte("host = localhost\nport = 8080\n"),
			want:     "ini",
		},
		{
			// Changed: colon-only files are now YAML, not INI. The
			// INI sniff deliberately defers them so that real YAML
			// (which overwhelmingly uses `:`) is not misidentified
			// and mangled. See the comment on isLikelyINI in
			// format.go for the full rationale.
			name:     "sniff: two key:value lines, no section, defers to YAML",
			filename: "noext",
			raw:      []byte("host: localhost\nport: 8080\n"),
			want:     "yaml",
		},
		{
			name:     "JSON array is not INI",
			filename: "noext",
			raw:      []byte(`[1,2,3]`),
			want:     "",
		},
		{
			name:     "JSON object is not INI",
			filename: "noext",
			raw:      []byte(`{"key": "value"}`),
			want:     "",
		},
		{
			name:     "plain text is not INI",
			filename: "noext",
			raw:      []byte("just some plain text\nwithout any structure"),
			want:     "",
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
func TestINI_EmptyInput(t *testing.T) {
	_, _, err := (&iniFormatter{}).FormatBytes(nil)
	if err != ErrEmptyInput {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}