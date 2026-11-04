package format

import (
	"bytes"
	"strings"
)

func init() {
	register(&iniFormatter{})
}

// iniFormatter is the INI formatter. Unlike HTML, INI has no standard
// parser in the Go standard library, and third-party parsers (like
// gopkg.in/ini.v1) rewrite the document in ways we do not want — they
// reorder keys alphabetically, drop comments, and normalise whitespace
// aggressively.
//
// We therefore use a hand-written, line-based formatter that treats
// the input as a sequence of classified lines and re-emits each line
// in its canonical shape. This preserves the user's intent (key order,
// indentation, comment style) while still applying the formatting
// improvements that matter: separator normalisation and blank-line
// collapsing.
//
// The formatter never fails on parseable or unparseable input alike.
// Lines that cannot be classified as blank, comment, section header,
// or key-value are kept verbatim with a warning — the user sees
// exactly what they sent plus a note explaining the ambiguity.
type iniFormatter struct{}

func (i *iniFormatter) Format() string { return "ini" }

// iniLineKind classifies each line so the re-emitter can apply the
// right rule.
type iniLineKind int

const (
	iniBlank iniLineKind = iota
	iniComment
	iniSection
	iniKeyValue
	iniGarbage
)

// FormatBytes implements Formatter.
//
// Because INI is a line-oriented format, "parsing" here means
// classifying each line and then re-emitting it. The only non-nil
// error path is ErrEmptyInput; every other input produces some
// output plus a Report.
func (i *iniFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "ini"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	// Split into lines. We keep the trailing-newline information by
	// using Split (not SplitN), which always yields at least one
	// element; a document ending in "\n" produces a final empty
	// element which we treat as the implicit end-of-file.
	rawLines := strings.Split(string(work), "\n")

	// Emit each line in its canonical form, with one important rule:
	// consecutive blank lines collapse to one. We track the previous
	// emitted kind to implement this.
	var (
		out      bytes.Buffer
		prevKind = iniGarbage // treat start-of-file as "not blank"
	)

	for idx, line := range rawLines {
		kind, canonical, warning := canonicaliseINILine(line, idx+1)

		if warning != "" {
			report.AddWarning(warning)
		}

		switch kind {
		case iniBlank:
			// Collapse runs of blank lines to one.
			if prevKind != iniBlank {
				out.WriteByte('\n')
				prevKind = iniBlank
			} else {
				report.AddFix("blank-lines-collapsed")
			}
			continue

		case iniGarbage:
			// Preserving a garbage line is itself a formatting
			// decision: we chose to keep it (rather than drop it)
			// AND we attached a warning. Both of those are user-
			// visible outcomes, so the Report must reflect that
			// the file was not already canonical.
			report.AddFix("garbage-line-preserved")
			out.WriteString(canonical)
			out.WriteByte('\n')
			prevKind = kind

		case iniComment, iniSection, iniKeyValue:
			out.WriteString(canonical)
			out.WriteByte('\n')
			prevKind = kind
		}
	}

	// Trim trailing blank lines and ensure the file ends with exactly
	// one newline.
	result := trimTrailingBlankLines(out.String())
	if result != "" {
		result += "\n"
	}

	// Decide whether we changed anything.
	//
	// The report's Changed flag may already be true from an earlier
	// stage (BOM, blank-line collapse, garbage preservation). If it
	// is still false, the canonicalisation must have produced
	// byte-identical output — which only happens when the input was
	// truly canonical.
	finalBytes := []byte(result)
	if bytes.Equal(finalBytes, work) {
		return work, report, nil
	}

	// The bytes differ, so we definitely changed something.
	report.AddFix("formatted")
	return finalBytes, report, nil
}

// canonicaliseINILine classifies and canonicalises a single line.
//
//   - Blank line: kind=iniBlank, canonical="".
//   - Comment line (starts with ';' or '#' after leading whitespace):
//     kind=iniComment, canonical=rstrip(line). Comment style preserved.
//   - Section header ([name]): kind=iniSection, canonical=[trimmed-name].
//     Inner whitespace stripped.
//   - Key-value (any of key=value, key = value, key: value, key : value):
//     kind=iniKeyValue, canonical=indent+key+" = "+value.
//   - Anything else: kind=iniGarbage, canonical=rstrip(line) with
//     a warning added.
//
// The leading indent (spaces or tabs) is preserved for every kind
// except blank lines.
func canonicaliseINILine(line string, lineNum int) (iniLineKind, string, string) {
	// Preserve leading whitespace exactly. Some INI files use
	// indentation to visually group keys, and forcing column 0 would
	// destroy that intent.
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	// Blank line.
	if trimmed == "" {
		return iniBlank, "", ""
	}

	// Comment line: preserve the character (';' or '#') the user
	// chose. Do NOT convert between them.
	if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
		return iniComment, rstrip(line), ""
	}

	// Section header: [name].
	if strings.HasPrefix(trimmed, "[") {
		if close := strings.LastIndexByte(trimmed, ']'); close > 0 {
			name := strings.TrimSpace(trimmed[1:close])
			return iniSection, indent + "[" + name + "]", ""
		}
		// Unclosed section header — treat as garbage below.
	}

	// Key-value: look for the first '=' or ':' outside of any leading
	// whitespace and outside of quoted values.
	sepIdx, _ := findINISeparator(trimmed)
	if sepIdx > 0 {
		key := strings.TrimRight(trimmed[:sepIdx], " \t")
		value := strings.TrimLeft(trimmed[sepIdx+1:], " \t")

		// Canonical separator: exactly one space on each side of '='.
		// This is the single biggest formatting win: users get
		// consistent spacing without editing every line.
		canonical := indent + key + " = " + value
		return iniKeyValue, canonical, ""
	}

	// Garbage line: we do NOT drop it. We keep it (trailing whitespace
	// stripped) and warn the user.
	return iniGarbage, rstrip(line),
		"line " + itoa(lineNum) + ": could not parse as key=value or section"
}

// findINISeparator returns the index of the first '=' or ':' that
// acts as a key-value separator, along with which character was
// found. Returns (-1, 0) if no separator is present.
//
// Separators inside quoted strings are ignored, so `key="a:b"` is
// treated as key="a:b" (separator at '=') and not split on the ':'
// inside the value.
func findINISeparator(s string) (int, byte) {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '=':
			return i, '='
		case ':':
			// Reject `://` (URL scheme in a value) — this can only
			// appear after '=' has already been found, so we never
			// reach this branch for a URL value. The check below
			// guards against key names that legitimately contain
			// ':' which is very rare.
			if i+1 < len(s) && s[i+1] == '/' {
				continue
			}
			return i, ':'
		}
	}
	return -1, 0
}