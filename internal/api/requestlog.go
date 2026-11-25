package api

import (
	"math"
	"time"

	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/route"
)

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// logRequestDone writes one line with speed and outcome. It never carries the
// prompt or the answer.
func logRequestDone(source string, res route.Result, st Stats, elapsed time.Duration) {
	f := map[string]any{
		"source": source, "outcome": "done",
		"machine": res.Machine, "model": res.Model, "mode": res.Mode,
		"load_ms": st.LoadMs, "queue_wait_ms": st.QueueWaitMs,
		"prompt_tokens": st.PromptTokens, "output_tokens": st.OutputTokens,
		"prompt_tokens_per_s": round2(st.PromptTokensPerS),
		"output_tokens_per_s": round2(st.OutputTokensPerS),
	}
	if st.TotalMs > 0 {
		f["duration_ms"] = st.TotalMs
	} else if elapsed > 0 {
		f["duration_ms"] = elapsed.Milliseconds()
	}
	logx.Event("info", "request_done", f)
}
