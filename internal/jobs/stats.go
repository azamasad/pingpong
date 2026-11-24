package jobs

import (
	"math"

	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/store"
)

// streamStats converts Ollama's nanosecond timings into the same fields the
// chat API reports on a done event.
func streamStats(ch ollama.StreamChunk) map[string]any {
	out := map[string]any{}
	if ch.TotalDuration > 0 {
		out["total_ms"] = ch.TotalDuration / 1e6
	}
	if ch.LoadDuration > 0 {
		out["load_ms"] = ch.LoadDuration / 1e6
	}
	if ch.PromptEvalCount > 0 {
		out["prompt_tokens"] = ch.PromptEvalCount
	}
	if ch.EvalCount > 0 {
		out["output_tokens"] = ch.EvalCount
	}
	if ch.PromptEvalDuration > 0 && ch.PromptEvalCount > 0 {
		out["prompt_tokens_per_s"] = float64(ch.PromptEvalCount) / (float64(ch.PromptEvalDuration) / 1e9)
	}
	if ch.EvalDuration > 0 && ch.EvalCount > 0 {
		out["output_tokens_per_s"] = float64(ch.EvalCount) / (float64(ch.EvalDuration) / 1e9)
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// logJob writes one line per finished attempt. It never carries the prompt or
// the answer.
func logJob(j *store.Job, outcome, code string, stats map[string]any) {
	f := map[string]any{
		"source": "job", "outcome": outcome, "request_id": j.ID,
		"machine": j.Machine, "model": j.Model, "mode": j.Mode, "lane": j.Lane,
	}
	if code != "" {
		f["error_code"] = code
	}
	for _, k := range []string{"total_ms", "load_ms", "prompt_tokens", "output_tokens"} {
		if v, ok := stats[k]; ok {
			f[k] = v
		}
	}
	if v, ok := stats["total_ms"]; ok {
		f["duration_ms"] = v
	}
	for _, k := range []string{"prompt_tokens_per_s", "output_tokens_per_s"} {
		if v, ok := stats[k].(float64); ok {
			f[k] = round2(v)
		}
	}
	level := "info"
	if outcome == "failed" || outcome == "retrying" {
		level = "warn"
	}
	logx.Event(level, "job_"+outcome, f)
}
