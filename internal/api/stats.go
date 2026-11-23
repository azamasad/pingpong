package api

import "github.com/azamasad/pingpong/internal/ollama"

type Stats struct {
	QueueWaitMs      int64   `json:"queue_wait_ms,omitempty"`
	TotalMs          int64   `json:"total_ms,omitempty"`
	LoadMs           int64   `json:"load_ms,omitempty"`
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	PromptTokensPerS float64 `json:"prompt_tokens_per_s,omitempty"`
	OutputTokens     int     `json:"output_tokens,omitempty"`
	OutputTokensPerS float64 `json:"output_tokens_per_s,omitempty"`
}

func statsFromChunk(ch ollama.StreamChunk, queueWaitMs int64) Stats {
	s := Stats{QueueWaitMs: queueWaitMs}
	if ch.TotalDuration > 0 {
		s.TotalMs = ch.TotalDuration / int64(1e6)
	}
	if ch.LoadDuration > 0 {
		s.LoadMs = ch.LoadDuration / int64(1e6)
	}
	if ch.PromptEvalCount > 0 {
		s.PromptTokens = ch.PromptEvalCount
	}
	if ch.EvalCount > 0 {
		s.OutputTokens = ch.EvalCount
	}
	if ch.PromptEvalDuration > 0 && ch.PromptEvalCount > 0 {
		s.PromptTokensPerS = float64(ch.PromptEvalCount) / (float64(ch.PromptEvalDuration) / 1e9)
	}
	if ch.EvalDuration > 0 && ch.EvalCount > 0 {
		s.OutputTokensPerS = float64(ch.EvalCount) / (float64(ch.EvalDuration) / 1e9)
	}
	return s
}

func contextWarning(numCtx int, ch ollama.StreamChunk, s Stats) bool {
	if ch.DoneReason == "length" {
		return true
	}
	if s.PromptTokens > 0 && numCtx > 0 {
		threshold := float64(numCtx) * 0.95
		if float64(s.PromptTokens) >= threshold {
			return true
		}
	}
	if s.PromptTokens > 0 && s.OutputTokens > 0 && numCtx > 0 {
		if s.PromptTokens+s.OutputTokens >= numCtx {
			return true
		}
	}
	return false
}
