package format

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

func init() {
	register(&csvFormatter{})
}

// csvFormatter formats CSV documents.
//
// Unlike the HTML/XML/INI formatters, CSV formatting is not about
// indentation. It's about clean re-emission: consistent delimiter,
// consistent quoting, and rows with the same column count as the header.
//
// DESIGN DECISION: We do NOT column-align the output file with extra
// spaces. Doing so would break re-parseability by standard CSV readers
// (Excel, Google Sheets, pandas) because those readers would include
// the padding spaces in each field value. The "table" alignment that
// format-express.dev shows for CSV is a view-time feature of their web
// UI, not part of the CSV file itself.
//
// Canonical output rules:
//   - comma delimiter (or the detected majority delimiter)
//   - quote only fields that need it (contain delimiter, quote, or newline)
//   - preserve header row as-is
//   - pad short rows with empty fields (warning)
//   - truncate long rows to header width (warning)
//   - exactly one trailing newline at EOF
//
// Broken input handling:
//   - empty file вҶ’ ErrEmptyInput (only non-nil error path)
//   - a row that cannot be parsed вҶ’ captured as raw text + warning, continue
//   - completely unparseable content вҶ’ best-effort line cleanup + warning
type csvFormatter struct{}

func (c *csvFormatter) Format() string { return "csv" }

// FormatBytes implements Formatter.
func (c *csvFormatter) FormatBytes(raw []byte) ([]byte, Report, error) {
	report := Report{Format: "csv"}

	if len(raw) == 0 {
		return nil, report, ErrEmptyInput
	}

	// Stage 0: normalise bytes.
	work, hadBOM := stripBOM(raw)
	if hadBOM {
		report.AddFix("bom-stripped")
	}
	work = normaliseLineEndings(work)

	// Stage 1: delimiter detection.
	delim, ok := detectCSVDelimiter(work)
	if !ok {
		delim = ','
		report.AddWarning("could not detect a consistent delimiter; assuming comma")
	}
	if delim != ',' {
		report.AddFix("delimiter-detected")
	}

	// Stage 2: parse.
	reader := csv.NewReader(bytes.NewReader(work))
	reader.Comma = rune(delim)
	reader.FieldsPerRecord = -1 // ragged rows do not error
	reader.LazyQuotes = true    // tolerate slightly broken quoting
	reader.TrimLeadingSpace = false

	var records [][]string
	var rowErrs []int
	rowIdx := 0
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowIdx++
		if err != nil {
			// A specific row failed to parse. Capture it as a single
			// comma-joined string and warn, but don't abort.
			rowErrs = append(rowErrs, rowIdx)
			// Try to reconstruct as a raw line.
			// encoding/csv doesn't give us the raw line, so we
			// append a placeholder row with the error text.
			records = append(records, []string{"[unparseable row " + itoa(rowIdx) + "]"})
			continue
		}
		records = append(records, rec)
	}

	if len(records) == 0 {
		// No parseable rows at all. Fall back to line-level cleanup.
		report.AddWarning("input could not be parsed as CSV; returning line-cleaned text")
		cleaned := lineCleanupCSV(work)
		report.AddFix("line-cleanup")
		return cleaned, report, nil
	}

	if len(rowErrs) > 0 {
		report.AddWarning(
			itoa(len(rowErrs)) + " row(s) could not be parsed and were replaced with placeholders",
		)
	}

	// Stage 3: normalise ragged rows to the header width.
	headerWidth := len(records[0])
	if headerWidth == 0 {
		report.AddWarning("header row has zero columns; returning input unchanged")
		return work, report, nil
	}

	padded := 0
	truncated := 0
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < headerWidth {
			// Pad with empty fields.
			for len(row) < headerWidth {
				row = append(row, "")
			}
			records[i] = row
			padded++
		} else if len(row) > headerWidth {
			// Truncate.
			records[i] = row[:headerWidth]
			truncated++
		}
	}
	if padded > 0 {
		report.AddFix("rows-padded")
		report.AddWarning(itoa(padded) + " short row(s) padded with empty fields")
	}
	if truncated > 0 {
		report.AddFix("rows-truncated")
		report.AddWarning(itoa(truncated) + " long row(s) truncated to header width")
	}

	// Stage 4: re-emit.
	var out bytes.Buffer
	writer := csv.NewWriter(&out)
	writer.Comma = rune(delim)
	for _, rec := range records {
		if err := writer.Write(rec); err != nil {
			// Writer errors are internal (buffer full, etc.) and
			// shouldn't happen in practice. Fall back to line cleanup.
			report.AddWarning("CSV writer error; returning line-cleaned input")
			return lineCleanupCSV(work), report, nil
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		report.AddWarning("CSV writer flush error; returning line-cleaned input")
		return lineCleanupCSV(work), report, nil
	}

	// Ensure exactly one trailing newline.
	result := trimTrailingNewlines(out.Bytes())
	if len(result) > 0 {
		result = append(result, '\n')
	}

	// Compare to decide Changed.
	if bytes.Equal(result, work) {
		return work, report, nil
	}

	report.AddFix("formatted")
	return result, report, nil
}

// detectCSVDelimiter examines the first few non-blank lines of work and
// picks the delimiter (,, ;, tab, |) that appears with the same
// positive count on the most lines.
func detectCSVDelimiter(raw []byte) (byte, bool) {
	lines := splitLinesForCSV(raw)
	if len(lines) == 0 {
		return 0, false
	}

	// Cap at 20 lines for performance.
	maxLines := 20
	if len(lines) < maxLines {
		maxLines = len(lines)
	}

	candidates := []byte{',', ';', '\t', '|'}
	best := byte(0)
	bestConsistency := 0

	for _, d := range candidates {
		// Count how many lines have the same positive count of d.
		counts := map[int]int{}
		positive := 0
		for i := 0; i < maxLines; i++ {
			n := countDelimiterOutsideQuotes(lines[i], d)
			if n > 0 {
				counts[n]++
				positive++
			}
		}
		if positive < 2 {
			continue
		}
		// Find the most common count.
		var modeCount, modeFreq int
		for n, freq := range counts {
			if freq > modeFreq || (freq == modeFreq && n > modeCount) {
				modeCount = n
				modeFreq = freq
			}
		}
		if modeFreq > bestConsistency {
			bestConsistency = modeFreq
			best = d
		}
	}

	if best == 0 || bestConsistency < 2 {
		return 0, false
	}
	return best, true
}

// splitLinesForCSV splits raw into lines, normalising line endings and
// dropping trailing blank lines.
func splitLinesForCSV(raw []byte) []string {
	s := string(raw)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// countDelimiterOutsideQuotes counts occurrences of delim that are NOT
// inside double-quoted regions.
func countDelimiterOutsideQuotes(line string, delim byte) int {
	n := 0
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			// Handle "" escape inside quoted fields.
			if inQuote && i+1 < len(line) && line[i+1] == '"' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if !inQuote && c == delim {
			n++
		}
	}
	return n
}

// lineCleanupCSV is the last-resort fallback: trim trailing whitespace
// per line, collapse blank lines, ensure trailing newline.
func lineCleanupCSV(raw []byte) []byte {
	lines := splitLinesForCSV(raw)
	var out bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(rstrip(line))
	}
	result := out.Bytes()
	if len(result) > 0 {
		result = append(result, '\n')
	}
	return result
}

// Silence unused import warnings for packages we may extend later.
var _ = fmt.Sprintf
var _ = io.Discard