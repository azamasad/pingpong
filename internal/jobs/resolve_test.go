package jobs

import (
	"context"
	"testing"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
)

func TestResolveMissingModel(t *testing.T) {
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
	g := gate.New(map[string]int{"m": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "m")
	st, _ := store.Open(t.TempDir())
	svc := New(cfg, st, reg, g, nil)
	res := svc.resolve(route.Request{Machine: "m", Model: "missing:7b"})
	if res.OK || res.Code != route.CodeModelNotInstalled {
		t.Fatalf("res=%+v", res)
	}
}
