package doctor_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/doctor"
	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestChatCheckOk(t *testing.T) {
	fs := &fakeollama.Server{
		Models: []string{"m:8b"},
		ChatSteps: []fakeollama.Step{
			{Content: "OK"},
			{Done: &fakeollama.DoneStats{EvalCount: 1, PromptEvalCount: 2}},
		},
	}
	ts := fakeollama.NewHTTPTestServer(fs)
	defer ts.Close()
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "a", Address: ts.URL, DefaultModel: "m:8b", MaxConcurrent: 1, NumCtx: 8192,
		}},
		Timeouts: config.TimeoutConfig{
			FirstTokenTimeout: config.Duration{Duration: 30 * time.Second},
			ConnectTimeout:    config.Duration{Duration: time.Second},
		},
	}
	out := doctor.ChatChecks(context.Background(), cfg)
	var ok bool
	for _, c := range out {
		if c.Name == "machine.a.chat" && c.Status == doctor.StatusOK {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("checks %+v", out)
	}
}
