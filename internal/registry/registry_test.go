package registry_test

import (
	"context"
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
