package export

import (
	"bytes"
	"io"
	"strings"
)

const csvEOL = "\r\n"

// CSVHeader returns the header row for CSV export.
func CSVHeader(includeThinking bool) []string {
	cols := []string{
		"index", "status", "machine", "model", "lane", "stage",
		"prompt", "output", "output_tokens", "output_tokens_per_s",
		"error_code", "finished_at",
	}
	if includeThinking {
		cols = append(cols, "thinking")
	}
	return cols
}

// WriteCSVHeader writes the header row.
func WriteCSVHeader(w io.Writer, includeThinking bool) error {
	return WriteCSVRow(w, CSVHeader(includeThinking), false)
}

// RowCSVCols returns column values in export order.
func RowCSVCols(r Row, includeThinking bool) []string {
	cols := []string{
		r.Index, r.Status, r.Machine, r.Model, r.Lane, r.Stage,
		r.Prompt, r.Output, r.OutputTokens, r.OutputTokensPerS,
		r.ErrorCode, r.FinishedAt,
	}
	if includeThinking {
		cols = append(cols, r.Thinking)
	}
	return cols
}

// WriteCSVRow writes one RFC 4180 row with CRLF.
func WriteCSVRow(w io.Writer, cols []string, raw bool) error {
	guarded := make([]string, len(cols))
	for i, c := range cols {
		guarded[i] = c
		if !raw && csvFieldGuarded(i, len(cols)) {
			guarded[i] = GuardFormula(c)
		}
	}
	var b strings.Builder
	for i, c := range guarded {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(quoteCSV(c))
	}
	b.WriteString(csvEOL)
	_, err := io.Copy(w, strings.NewReader(b.String()))
	return err
}

func csvFieldGuarded(idx, ncols int) bool {
	// Guard text from status through error_code; finished_at is ISO time; thinking optional last.
	switch idx {
	case 1, 2, 3, 4, 5, 6, 7, 10:
		return true
	case 12:
		return ncols > 12 // thinking
	default:
		return false
	}
}

func quoteCSV(s string) string {
	if s == "" {
		return ""
	}
	needQuote := strings.ContainsAny(s, ",\"\r\n")
	if !needQuote {
		return s
	}
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' {
			b.WriteString(`""`)
			continue
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
