package export

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

func TestGuardFormulaChars(t *testing.T) {
	chars := []string{"=", "+", "-", "@", "\t", "\r"}
	for _, c := range chars {
		in := c + "evil"
		got := GuardFormula(in)
		if !strings.HasPrefix(got, "'") {
			t.Fatalf("guard %q: %q", c, got)
		}
	}
	if GuardFormula("raw=1") == GuardFormula("raw=1") {
		if strings.HasPrefix(GuardFormula("hello"), "'") {
			t.Fatal("plain text")
		}
	}
}

func TestGuardFormulaOffWhenRaw(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteCSVRow(&buf, []string{"0", "=x", "m", "mod", "batch", "", "p", "o", "", "", "", ""}, true)
	if strings.Contains(buf.String(), "'=") {
		t.Fatalf("raw should not guard: %s", buf.String())
	}
}

func TestCSVRoundTrip(t *testing.T) {
	row := Row{
		Index: "0", Status: "done", Machine: "mane", Model: "m",
		Prompt: "a,b", Output: "line1\nline2",
	}
	var buf bytes.Buffer
	cols := RowCSVCols(row, false)
	_ = WriteCSVHeader(&buf, false)
	_ = WriteCSVRow(&buf, cols, false)
	if !strings.Contains(buf.String(), "\r\n") {
		t.Fatal("CRLF")
	}
	r := csv.NewReader(strings.NewReader(buf.String()))
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("rows %d", len(records))
	}
	if records[1][6] != "a,b" {
		t.Fatalf("comma field %q", records[1][6])
	}
}

func TestMarkdownFenceLongerThanContent(t *testing.T) {
	text := "plain ``` inner ```"
	f := FenceFor(text)
	if len(f) <= 3 {
		t.Fatal(f)
	}
	if !strings.Contains(text, strings.Repeat("`", len(f)-1)) {
		// content has triple backtick run; fence must beat it
	}
	if len(f) < 4 {
		t.Fatalf("fence %q for %q", f, text)
	}
}

func TestBuildRowTerminalFinishedAt(t *testing.T) {
	now := time.Now().UTC()
	j := &store.Job{
		Status: store.StatusDone, UpdatedAt: now, RequestJSON: `{"prompt":"hi"}`,
	}
	r := BuildRow(j, 0, true, nil, false)
	if r.FinishedAt == "" {
		t.Fatal("finished_at")
	}
	j.Status = store.StatusRunning
	r2 := BuildRow(j, 0, true, nil, false)
	if r2.FinishedAt != "" {
		t.Fatal("non-terminal")
	}
}
