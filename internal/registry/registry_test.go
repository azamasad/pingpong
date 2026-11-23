package registry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
)

func TestRefreshAndDown(t *testing.T) {
	fs := &fakeollama.Server{Models: []string{"m:8b"}, TagsStatus: 503}
	ts := fakeollama.NewHTTPTestServer(fs)
	defer ts.Close()
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "a", Address: ts.URL, Roles: []string{"heavy"},
			DefaultModel: "m:8b", MaxConcurrent: 1, NumCtx: 8192,
		}},
		Routing: config.RoutingConfig{AutoRole: "heavy", FastRole: "fast"},
		Health: config.HealthConfig{
			FailuresToMarkDown: 1,
			Interval:           config.Duration{Duration: time.Hour},
		},
	}
	g := gate.New(map[string]int{"a": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "a")
	reg.MarkDownImmediate("a")
	if !reg.IsDown("a") {
		t.Fatal("down")
	}
	list := reg.ListMachines()
	if list[0].Reachable {
		t.Fatal("reachable")
	}
}

func TestHealthCheckLoadsModelsAfterLateStart(t *testing.T) {
	fs := &fakeollama.Server{Models: []string{"m:8b"}}
	var up atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fs.Handler().ServeHTTP(w, r)
	}))
	defer ts.Close()
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "a", Address: ts.URL, Roles: []string{"heavy"},
			DefaultModel: "m:8b", MaxConcurrent: 1, NumCtx: 8192,
		}},
		Routing: config.RoutingConfig{AutoRole: "heavy", FastRole: "fast"},
		Health: config.HealthConfig{
			FailuresToMarkDown:      2,
			Interval:                config.Duration{Duration: 20 * time.Millisecond},
			Timeout:                 config.Duration{Duration: time.Second},
			RegistryRefreshInterval: config.Duration{Duration: time.Hour},
		},
		Timeouts: config.TimeoutConfig{ConnectTimeout: config.Duration{Duration: time.Second}},
	}
	reg := registry.New(cfg, gate.New(map[string]int{"a": 1}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)
	time.Sleep(60 * time.Millisecond)
	up.Store(true)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if l := reg.ListMachines(); len(l) == 1 && len(l[0].Models) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("models were not loaded after the machine came up")
}
