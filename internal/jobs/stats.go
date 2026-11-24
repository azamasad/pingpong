package jobs

import "github.com/azamasad/pingpong/internal/ollama"

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
