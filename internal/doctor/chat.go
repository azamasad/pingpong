package doctor

import (
	"context"
	"fmt"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/ollama"
)

const chatPrompt = "Reply with the single word OK."

// ChatChecks runs the opt-in stream contract test against each machine.
func ChatChecks(ctx context.Context, cfg *config.Config) []Check {
	var out []Check
	for _, m := range cfg.Machines {
		out = append(out, chatOne(ctx, cfg, m)...)
	}
	return out
}

func chatOne(ctx context.Context, cfg *config.Config, m config.Machine) []Check {
	prefix := "machine." + m.Name + ".chat"
	cli := ollama.NewClient(m.Address, cfg.Timeouts.FirstTokenTimeout.Duration)
	limit := 16
	stream, err := cli.ChatStream(ctx, ollama.ChatRequest{
		Model: m.DefaultModel,
		Messages: []ollama.ChatMessage{
			{Role: "user", Content: chatPrompt},
		},
		Options: map[string]any{"num_predict": limit},
	})
	if err != nil {
		return []Check{{
			Name: prefix, Status: StatusFail,
			Message: fmt.Sprintf("chat stream failed: %v", err),
			Hint:    "check Ollama on this machine",
		}}
	}
	defer stream.Close()

	start := time.Now()
	var firstAt time.Time
	var sawContent, sawDone bool
	var evalN, promptN int

	for {
		if ctx.Err() != nil {
			return []Check{{Name: prefix, Status: StatusFail, Message: "timed out"}}
		}
		ch, ok, err := stream.Next()
		if err != nil {
			return []Check{{Name: prefix, Status: StatusFail, Message: err.Error()}}
		}
		if !ok {
			break
		}
		if ch.Message.Content != "" || ch.Message.Thinking != "" {
			if firstAt.IsZero() {
				firstAt = time.Now()
			}
			sawContent = true
		}
		if ch.Done {
			sawDone = true
			evalN = ch.EvalCount
			promptN = ch.PromptEvalCount
			break
		}
	}

	if !sawDone {
		return []Check{{
			Name: prefix, Status: StatusFail,
			Message: "stream ended without done:true",
			Hint:    "Ollama stream shape may differ from what the router expects",
		}}
	}
	if !sawContent {
		return []Check{{
			Name: prefix, Status: StatusFail,
			Message: "stream had no message.content or message.thinking",
		}}
	}
	var out []Check
	out = append(out, Check{
		Name: prefix, Status: StatusOK,
		Message: "stream contract ok",
	})
	if !firstAt.IsZero() {
		tts := firstAt.Sub(start).Seconds()
		out = append(out, Check{
			Name: prefix + ".first_token", Status: StatusInfo,
			Message: fmt.Sprintf("time to first token %.2fs", tts),
		})
	}
	if evalN > 0 {
		dur := time.Since(start).Seconds()
		if dur > 0 {
			out = append(out, Check{
				Name: prefix + ".speed", Status: StatusInfo,
				Message: fmt.Sprintf("%.2f tok/s (eval_count=%d prompt_eval_count=%d)", float64(evalN)/dur, evalN, promptN),
			})
		}
	}
	return out
}
