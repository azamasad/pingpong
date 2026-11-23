package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func TestMarkRetryWindowNotReset(t *testing.T) {
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.Fake{NowTime: t0}
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "m", Address: "http://127.0.0.1:9", Roles: []string{"heavy"},
			DefaultModel: "x", MaxConcurrent: 1, NumCtx: 4096,
		}},
		Routing: config.RoutingConfig{AutoRole: "heavy", FastRole: "heavy"},
	}
	cfg.Retry.RetryInterval = config.Duration{Duration: 2 * time.Minute}
	cfg.Retry.RetryWindow = config.Duration{Duration: 3 * time.Hour}
	cfg.Storage.RetentionSweepInterval = config.Duration{Duration: time.Hour}
	st, _ := store.Open(t.TempDir())
	defer st.Close()
	svc := New(cfg, st, registry.New(cfg, gate.New(map[string]int{"m": 1})), gate.New(map[string]int{"m": 1}), clk)
	_ = st.InsertJob(context.Background(), &store.Job{
		ID: "j", CreatedAt: t0, UpdatedAt: t0, Status: store.StatusRunning,
		RequestJSON: `{"prompt":"a"}`, Attempt: 1,
	})
	j, _ := st.GetJob(context.Background(), "j")
	svc.markRetry(j, "TIMEOUT", "first", t0)
	j, _ = st.GetJob(context.Background(), "j")
	if j.Status != store.StatusRetrying {
		t.Fatalf("after first: %s", j.Status)
	}
	clk.NowTime = t0.Add(2 * time.Hour)
	j, _ = st.GetJob(context.Background(), "j")
	svc.markRetry(j, "TIMEOUT", "second", clk.NowTime)
	j, _ = st.GetJob(context.Background(), "j")
	if j.Status != store.StatusRetrying {
		t.Fatalf("mid window: %s", j.Status)
	}
	clk.NowTime = t0.Add(3*time.Hour + time.Minute)
	j, _ = st.GetJob(context.Background(), "j")
	svc.markRetry(j, "TIMEOUT", "final", clk.NowTime)
	j, _ = st.GetJob(context.Background(), "j")
	if j.Status != store.StatusFailed {
		t.Fatalf("want failed got %s", j.Status)
	}
}
