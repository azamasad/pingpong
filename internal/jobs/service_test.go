package jobs_test

import (
	"context"
	"errors"
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

func TestSubmitRejectedNonRetryable(t *testing.T) {
	fake := &fakeollama.Server{Models: []string{"installed:7b"}}
	ts := fakeollama.NewHTTPTestServer(fake)
	defer ts.Close()

	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "m", Address: ts.URL, Roles: []string{"heavy"},
			DefaultModel: "installed:7b", MaxConcurrent: 1, NumCtx: 4096,
		}},
		Routing: config.RoutingConfig{AutoRole: "heavy", FastRole: "heavy"},
		Health:  config.HealthConfig{RefreshOnModelMiss: true},
	}
	cfg.Retry.RetryInterval = config.Duration{Duration: 2 * time.Minute}
	cfg.Retry.RetryWindow = config.Duration{Duration: 3 * time.Hour}
	st, _ := store.Open(t.TempDir())
	defer st.Close()
	g := gate.New(map[string]int{"m": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "m")
	svc := jobs.New(cfg, st, reg, g, clock.Real{})
	_, err := svc.Submit(context.Background(), `{"prompt":"hi","machine":"m","model":"missing:7b"}`)
	var rej *jobs.RouteRejectedError
	if err == nil || !errors.As(err, &rej) || rej.Code != "MODEL_NOT_INSTALLED" {
		t.Fatalf("got %v", err)
	}
}
