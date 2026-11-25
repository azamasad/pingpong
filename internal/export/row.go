package export

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

// Row is one exported job line.
type Row struct {
	Index            string
	Status           string
	Machine          string
	Model            string
	Lane             string
	Stage            string
	Prompt           string
	Output           string
	OutputTokens     string
	OutputTokensPerS string
	ErrorCode        string
	FinishedAt       string
	Thinking         string
}

// Stats from the last done event on a job.
type DoneStats struct {
	OutputTokens     int
	OutputTokensPerS float64
}

func Terminal(st store.JobStatus) bool {
	switch st {
	case store.StatusDone, store.StatusFailed, store.StatusCancelled:
		return true
	default:
		return false
	}
}

func PromptFromRequest(requestJSON string) string {
	var m struct {
		Prompt   string `json:"prompt"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal([]byte(requestJSON), &m) != nil {
		return ""
	}
	if m.Prompt != "" {
		return m.Prompt
	}
	for i := len(m.Messages) - 1; i >= 0; i-- {
		if m.Messages[i].Role == "user" {
			return m.Messages[i].Content
		}
	}
	return ""
}

// BuildRow assembles export fields for one job.
func BuildRow(j *store.Job, index int, hasIndex bool, stats *DoneStats, includeThinking bool) Row {
	r := Row{
		Status:    string(j.Status),
		Machine:   j.Machine,
		Model:     j.Model,
		Lane:      j.Lane,
		Stage:     j.Stage,
		Prompt:    PromptFromRequest(j.RequestJSON),
		Output:    j.OutputText,
		ErrorCode: j.ErrorCode,
	}
	if hasIndex {
		r.Index = strconv.Itoa(index)
	}
	if j.Stage == "reduce" {
		r.Index = ""
	}
	if Terminal(j.Status) {
		r.FinishedAt = j.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if stats != nil && stats.OutputTokens > 0 {
		r.OutputTokens = strconv.Itoa(stats.OutputTokens)
	}
	if stats != nil && stats.OutputTokensPerS > 0 {
		r.OutputTokensPerS = formatFloat(stats.OutputTokensPerS)
	}
	if includeThinking {
		r.Thinking = j.OutputThinking
	}
	return r
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// GuardFormula prefixes a leading apostrophe on spreadsheet-sensitive text.
func GuardFormula(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// FenceFor returns a markdown code fence at least 3 backticks, one longer than any run in text.
func FenceFor(text string) string {
	max := 0
	cur := 0
	for _, r := range text {
		if r == '`' {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	n := max + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
}
