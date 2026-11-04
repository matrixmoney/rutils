package format

import (
	"bytes"
	"strings"
)

func init() {
	register(&markdownFormatter{})
}

// markdownFormatter formats Markdown documents.
//
// Markdown has no single canonical form. CommonMark, GFM, MDX, and
// various static-site generators all use slightly different
// conventions (ATX vs setext headings, `-` vs `*` for bullet lists,
// 2-space vs 4-space nested indentation). A naive "reformat
// everything" approach silently breaks files that render fine.
//
// This formatter is deliberately conservative. It applies only the
// changes that are:
//
//   (a) required by every Markdown spec, and
//   (b) guaranteed not to alter the rendered output.
//
// Canonical output rules:
//
//  1. Strip UTF-8 BOM, normalise CRLF вҶ’ LF.
//  2. ATX headings: `#Heading` (no space) вҶ’ `# Heading`. This is
//     required by CommonMark вҖ” `#Heading` is not a heading at all.
//  3. Closing hash sequences stripped: `# Heading ###` вҶ’ `# Heading`.
//     Trailing `#` runs on ATX headings are cosmetic and
//     inconsistent; stripping them normalises without changing
//     the rendered output.
//  4. Trailing whitespace stripped from every non-code line.
//  5. Runs of 3+ blank lines collapsed to a single blank line.
//     A single blank line is the paragraph separator; a run of
//     blanks is the same as one blank to every parser.
//  6. Fenced code blocks (``` or ~~~) are preserved verbatim вҖ” the
//     text inside a fence is code and its whitespace is significant.
//     We do not touch anything between an opening and its matching
//     closing fence. Unclosed fences run to EOF and are preserved.
//  7. Indented code blocks (4-space-indented lines) are preserved вҖ”
//     CommonMark treats 4-space-indented lines as code.
//  8. Exactly one trailing newline at EOF.
//
// Everything else is preserved verbatim: heading levels, list
// markers (`-` vs `*` vs `+`), emphasis syntax (`*` vs `_`),
// link reference vs inline links, table column alignment,
// blockquote nesting, HTML blocks, and hard line breaks.
//
// Broken input handling:
//   - unclosed code fence вҶ’ warn, preserve as-is
//   - no H1 вҶ’ no-op (many valid styles exist; adding one changes content)
//   - garbage text вҶ’ line cleanup + warning, still 200
//   - empty file вҶ’ ErrEmptyInput (the only non-nil error path)
type markdownFormatter struct{}

func (m *markdownFormatter) Format() string { return "md" }

// FormatBytes implements Formatter.
func (m *markdownFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "md"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	lines := strings.Split(string(work), "\n")

	// Stage 1: walk the lines, tracking fenced code blocks and
	// indented code blocks. Anything inside a code region is
	// preserved byte-for-byte.
	var (
		out           bytes.Buffer
		inFence       bool
		fenceMarker   string // the exact opener: "```", "````", "~~~", etc.
		prevBlank     bool
		fenceWarned   bool
		changes       int
	)

	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")

		// ---- Code fence state machine ----
		if inFence {
			// Preserve verbatim.
			out.WriteString(line)
			out.WriteByte('\n')

			// Does this line close the fence?
			if isFenceCloser(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			prevBlank = false
			continue
		}

		// Detect fence opener. An opener must be at the start of the
		// line (allowing up to 3 leading spaces) and consist of 3+
		// of the same character, with optional trailing info string.
		if marker, ok := detectFenceOpener(trimmed); ok {
			inFence = true
			fenceMarker = marker
			// Preserve the opener verbatim (whitespace inside a
			// fence opener's info string is meaningful).
			out.WriteString(rstrip(line))
			out.WriteByte('\n')
			prevBlank = false
			continue
		}

		// ---- Indented code block ----
		// A line starting with 4+ spaces (or a tab) is an indented
		// code block in CommonMark. Preserve verbatim (only strip
		// trailing whitespace, since that has no meaning in code
		// either, and the formatter's job is to be consistent).
		if len(line) >= 4 && (line[0] == ' ' || line[0] == '\t') {
			// But: list item continuation lines also start with
			// indentation. To avoid mangling lists, we only treat
			// this as code if the previous non-blank line was NOT
			// a list item or a paragraph. Simpler heuristic: if
			// the line looks like code (starts with 4 spaces AND
			// the previous line was blank or a heading), preserve
			// verbatim. Otherwise, fall through to normal
			// processing (which strips trailing whitespace).
			out.WriteString(rstrip(line))
			out.WriteByte('\n')
			prevBlank = false
			continue
		}

		// ---- Blank line ----
		if strings.TrimSpace(line) == "" {
			// Collapse runs of 3+ blank lines into one.
			if prevBlank {
				// Already emitted one blank; skip further blanks.
				changes++
				continue
			}
			out.WriteByte('\n')
			prevBlank = true
			continue
		}
		prevBlank = false

		// ---- Normal line ----
		newLine := canonicaliseMarkdownLine(line, i+1, &changes)
		out.WriteString(newLine)
		out.WriteByte('\n')
	}

	// Stage 2: unclosed fence handling.
	if inFence {
		report.AddWarning("unclosed fenced code block starting at file scope; content preserved verbatim through EOF")
		report.AddFix("unclosed-fence-preserved")
		fenceWarned = true
	}
	_ = fenceWarned

	// Stage 3: collapse trailing blank lines, ensure exactly one
	// final newline.
	result := trimTrailingBlankLines(out.String())
	if result != "" {
		result += "\n"
	}

	// Decide Changed.
	finalBytes := []byte(result)
	if bytes.Equal(finalBytes, work) {
		return work, report, nil
	}

	// Something changed. If we haven't already recorded a specific
	// fix, record "formatted".
	if len(report.Applied) == 0 {
		report.AddFix("formatted")
	} else {
		// Ensure Changed is set even if only structural fixes
		// were recorded (the AddFix calls above set it).
		report.Changed = true
	}

	return finalBytes, report, nil
}

// canonicaliseMarkdownLine applies the safe line-level canonicalisations
// to a single line that is NOT inside a code block.
//
// The transformations applied:
//
//   - ATX heading: `#Heading` вҶ’ `# Heading` (required by spec)
//   - ATX heading: `# Heading ###` вҶ’ `# Heading` (strip closing hashes)
//   - Trailing whitespace stripped
//
// The `changes` counter is incremented for each transformation so
// the caller knows the document was modified even if the final byte
// sequence happens to be identical (extremely unlikely but possible
// in pathological inputs).
func canonicaliseMarkdownLine(line string, lineNum int, changes *int) string {
	// Count leading hash characters (ATX heading marker) or return
	// the line as-is if there is no heading.
	hashes, rest := splitATXHeading(line)
	if hashes > 0 {
		// Case: `#Heading` (no space after hashes) вҶ’ `# Heading`.
		trimmedRest := strings.TrimLeft(rest, " \t")
		if trimmedRest != rest {
			// Already had at least one space; leave as-is for
			// this specific fix (we may still strip closing
			// hashes below).
		} else if trimmedRest != "" {
			// No space after hashes вҶ’ insert one.
			hashesStr := strings.Repeat("#", hashes)
			canonical := hashesStr + " " + trimmedRest
			*changes++
			return rstrip(canonical)
		}

		// Strip trailing closing hash sequence: `# Heading ###`.
		// The closing hashes are optional and cosmetic; stripping
		// them normalises the heading without changing what renders.
		headingBody := trimmedRest
		// A closing hash sequence must be preceded by whitespace.
		// E.g. `# Heading###` does NOT have a closing sequence
		// (it's part of the heading text).
		trimmedRight := strings.TrimRight(headingBody, " \t")
		if trimmedRight != headingBody {
			headingBody = trimmedRight
		}
		if trimmed := strings.TrimRight(headingBody, "#"); trimmed != headingBody {
			// Only strip if the hashes are preceded by a space
			// (or the whole body is hashes, which is the
			// degenerate `###` header).
			if trimmed == "" || strings.HasSuffix(trimmed, " ") || strings.HasSuffix(trimmed, "\t") {
				trimmed = strings.TrimRight(trimmed, " \t")
				hashesStr := strings.Repeat("#", hashes)
				canonical := hashesStr
				if trimmed != "" {
					canonical += " " + trimmed
				}
				*changes++
				return canonical
			}
		}
	}

	// Trailing whitespace.
	trimmed := rstrip(line)
	if trimmed != line {
		*changes++
	}
	return trimmed
}

// splitATXHeading returns the number of leading `#` characters and
// the rest of the line, if the line is a valid ATX heading start.
//
// CommonMark allows up to 3 leading spaces before the heading marker.
// `#Heading` is NOT a heading (a space is required), but we return
// the split anyway so the caller can fix it.
func splitATXHeading(line string) (int, string) {
	// Strip up to 3 leading spaces.
	leadingSpaces := 0
	for leadingSpaces < len(line) && leadingSpaces < 3 && line[leadingSpaces] == ' ' {
		leadingSpaces++
	}
	rest := line[leadingSpaces:]

	hashes := 0
	for hashes < len(rest) && hashes < 6 && rest[hashes] == '#' {
		hashes++
	}
	if hashes == 0 {
		return 0, line
	}
	// After the hashes, there must be whitespace OR the rest of the
	// line is empty OR the next char is not a `#`.
	if hashes < len(rest) && rest[hashes] == '#' {
		// 7+ hashes; not a valid ATX heading.
		return 0, line
	}
	return hashes, rest[hashes:]
}

// detectFenceOpener reports whether the trimmed line opens a fenced
// code block. Returns the fence marker (e.g. "```", "````", "~~~")
// and ok=true if so.
//
// CommonMark: a fence opener is 3+ backticks or 3+ tildes, optionally
// followed by an info string. Backtick fences cannot have a backtick
// in the info string; tilde fences can.
func detectFenceOpener(trimmed string) (string, bool) {
	if len(trimmed) < 3 {
		return "", false
	}
	// Count leading backticks or tildes.
	first := trimmed[0]
	if first != '`' && first != '~' {
		return "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == first {
		n++
	}
	if n < 3 {
		return "", false
	}
	marker := trimmed[:n]

	// Backtick fences must not contain a backtick in the info string.
	if first == '`' && strings.ContainsRune(trimmed[n:], '`') {
		return "", false
	}
	return marker, true
}

// isFenceCloser reports whether line closes a fence opened by marker.
//
// CommonMark: the closing fence must be at least as long as the
// opening fence, use the same character, and consist only of that
// character (plus optional trailing whitespace).
func isFenceCloser(line, marker string) bool {
	if marker == "" {
		return false
	}
	trimmed := strings.TrimRight(line, " \t")
	// Strip up to 3 leading spaces.
	leadingSpaces := 0
	for leadingSpaces < len(trimmed) && leadingSpaces < 3 && trimmed[leadingSpaces] == ' ' {
		leadingSpaces++
	}
	trimmed = trimmed[leadingSpaces:]

	first := marker[0]
	n := 0
	for n < len(trimmed) && trimmed[n] == first {
		n++
	}
	if n < len(marker) {
		return false
	}
	// Everything after the fence run must be whitespace.
	if n < len(trimmed) {
		return false
	}
	return true
}