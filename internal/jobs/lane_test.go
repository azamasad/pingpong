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

func TestWaitTurnPrefersInteractive(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "m", Address: "http://127.0.0.1:9", Roles: []string{"heavy"},
			DefaultModel: "x", MaxConcurrent: 1, NumCtx: 1024,
		}},
		Routing: config.RoutingConfig{AutoRole: "heavy", FastRole: "heavy"},
	}
	cfg.Storage.RetentionSweepInterval = config.Duration{Duration: time.Hour}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UTC()
	ctx := context.Background()
	_ = st.InsertJob(ctx, &store.Job{
		ID: "batch", CreatedAt: now, UpdatedAt: now, Status: store.StatusWaiting,
		RequestJSON: `{"prompt":"b"}`, Machine: "m", Lane: store.LaneBatch,
	})
	_ = st.InsertJob(ctx, &store.Job{
		ID: "live", CreatedAt: now.Add(time.Second), UpdatedAt: now, Status: store.StatusWaiting,
		RequestJSON: `{"prompt":"i"}`, Machine: "m", Lane: store.LaneInteractive,
	})
	svc := New(cfg, st, registry.New(cfg, gate.New(map[string]int{"m": 1})), gate.New(map[string]int{"m": 1}), clock.Real{})

	live := make(chan struct{})
	batch := make(chan struct{})
	go func() {
		if err := svc.waitTurn(context.Background(), "live", "m"); err != nil {
			t.Error(err)
		}
		close(live)
	}()
	go func() {
		if err := svc.waitTurn(context.Background(), "batch", "m"); err != nil {
			t.Error(err)
		}
		close(batch)
	}()

	select {
	case <-live:
	case <-time.After(time.Second):
		t.Fatal("interactive was not admitted")
	}
	select {
	case <-batch:
		t.Fatal("batch was admitted ahead of interactive")
	case <-time.After(40 * time.Millisecond):
	}
	liveJob, err := st.GetJob(ctx, "live")
	if err != nil {
		t.Fatal(err)
	}
	liveJob.Status = store.StatusRunning
	if err := st.UpdateJob(ctx, liveJob); err != nil {
		t.Fatal(err)
	}
	svc.releaseTurn("m")
	select {
	case <-batch:
	case <-time.After(time.Second):
		t.Fatal("batch was not admitted after release")
	}
}
