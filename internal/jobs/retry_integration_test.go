package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func jobTestCfg(t *testing.T, url string) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "m", Address: url, Roles: []string{"heavy"},
			DefaultModel: "qwen:14b", MaxConcurrent: 1, NumCtx: 4096,
		}},
		Routing: config.RoutingConfig{AutoRole: "heavy", FastRole: "heavy"},
		Health:  config.HealthConfig{RefreshOnModelMiss: true},
		Storage: config.StorageConfig{OutputFlushInterval: config.Duration{Duration: time.Millisecond}},
	}
	cfg.Retry.RetryInterval = config.Duration{Duration: 30 * time.Millisecond}
	cfg.Retry.RetryWindow = config.Duration{Duration: 120 * time.Millisecond}
	cfg.Timeouts.ConnectTimeout = config.Duration{Duration: 50 * time.Millisecond}
	cfg.Timeouts.FirstTokenTimeout = config.Duration{Duration: time.Second}
	cfg.Timeouts.IdleTokenTimeout = config.Duration{Duration: time.Second}
	cfg.Timeouts.MaxRunTime = config.Duration{Duration: 2 * time.Second}
	cfg.Storage.RetentionSweepInterval = config.Duration{Duration: time.Hour}
	cfg.Storage.RetentionDays = 90
	return cfg
}

func TestRetryWindowEventuallyFailed(t *testing.T) {
	fake := &fakeollama.Server{
		Models:    []string{"qwen:14b"},
		ChatSteps: []fakeollama.Step{{Close: true}},
	}
	ts := fakeollama.NewHTTPTestServer(fake)
	defer ts.Close()
	cfg := jobTestCfg(t, ts.URL)
	st, _ := store.Open(t.TempDir())
	defer st.Close()
	g := gate.New(map[string]int{"m": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "m")
	svc := jobs.New(cfg, st, reg, g, clock.Real{})
	svc.Start(context.Background())
	id, err := svc.Submit(context.Background(), `{"prompt":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetJob(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == store.StatusFailed {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("job did not fail within retry window")
}

func TestRecoveryRunningBecomesRetrying(t *testing.T) {
	cfg := jobTestCfg(t, "http://127.0.0.1:9")
	st, _ := store.Open(t.TempDir())
	now := time.Now().UTC()
	_ = st.InsertJob(context.Background(), &store.Job{
		ID: "r1", CreatedAt: now, UpdatedAt: now, Status: store.StatusRunning,
		RequestJSON: `{"prompt":"x"}`, Machine: "m", Model: "qwen:14b",
	})
	g := gate.New(map[string]int{"m": 1})
	reg := registry.New(cfg, g)
	svc := jobs.New(cfg, st, reg, g, clock.Real{})
	svc.Start(context.Background())
	j, _ := st.GetJob(context.Background(), "r1")
	if j.Status != store.StatusRetrying {
		t.Fatalf("got %s", j.Status)
	}
}
