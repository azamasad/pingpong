package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/doctor"
	"github.com/azamasad/pingpong/internal/fakeollama"
)

func baseCfg(tsURL, name string) *config.Config {
	return &config.Config{
		Machines: []config.Machine{{
			Name: name, Address: tsURL, Roles: []string{"fast"},
			DefaultModel: "m:8b", MaxConcurrent: 1, NumCtx: 8192,
		}},
		Routing:  config.RoutingConfig{AutoRole: "fast", FastRole: "fast"},
		Profiles: map[string]config.Profile{"fast": {Fast: true}},
		Health: config.HealthConfig{
			Timeout: config.Duration{Duration: 2 * time.Second},
		},
		Timeouts: config.TimeoutConfig{
			ConnectTimeout: config.Duration{Duration: 2 * time.Second},
		},
	}
}

func find(checks []doctor.Check, name string) (doctor.Check, bool) {
	for _, c := range checks {
		if c.Name == name {
			return c, true
		}
	}
	return doctor.Check{}, false
}

func TestProfilesUnknownMachineFails(t *testing.T) {
	cfg := baseCfg("http://127.0.0.1:1", "a")
	cfg.Profiles["bad"] = config.Profile{Machine: "missing"}
	chk := doctor.Run(context.Background(), cfg, nil, doctor.Options{})
	c, ok := find(chk, "config.profiles")
	if !ok || c.Status != doctor.StatusFail {
		t.Fatalf("profiles check %+v", c)
	}
}

func TestMachineReachableAndDefaultModel(t *testing.T) {
	fs := &fakeollama.Server{Models: []string{"m:8b", "extra:1b"}}
	ts := fakeollama.NewHTTPTestServer(fs)
	defer ts.Close()
	cfg := baseCfg(ts.URL, "a")
	out := doctor.Run(context.Background(), cfg, nil, doctor.Options{
		DiskFree: func(string) (uint64, error) { return 10 << 30, nil },
	})
	for _, name := range []string{"machine.a.reachable", "machine.a.default_model"} {
		c, ok := find(out, name)
		if !ok || c.Status != doctor.StatusOK {
			t.Fatalf("%s: %+v", name, c)
		}
	}
	c, _ := find(out, "machine.a.models_unused")
	if c.Status != doctor.StatusInfo || c.Message == "" {
		t.Fatalf("unused info %+v", c)
	}
}

func TestUnreachableMachineFails(t *testing.T) {
	cfg := baseCfg("http://127.0.0.1:1", "a")
	out := doctor.Run(context.Background(), cfg, nil, doctor.Options{})
	c, ok := find(out, "machine.a.reachable")
	if !ok || c.Status != doctor.StatusFail {
		t.Fatalf("reachable %+v", c)
	}
	if doctor.Failed(out) {
		return
	}
	t.Fatal("expected failed")
}

func TestDefaultModelMissingFails(t *testing.T) {
	fs := &fakeollama.Server{Models: []string{"other:7b"}}
	ts := fakeollama.NewHTTPTestServer(fs)
	defer ts.Close()
	cfg := baseCfg(ts.URL, "a")
	out := doctor.Run(context.Background(), cfg, nil, doctor.Options{
		DiskFree: func(string) (uint64, error) { return 10 << 30, nil },
	})
	c, ok := find(out, "machine.a.default_model")
	if !ok || c.Status != doctor.StatusFail {
		t.Fatalf("default_model %+v", c)
	}
}

func TestStorageWarnWhenJobsOff(t *testing.T) {
	cfg := baseCfg("http://127.0.0.1:1", "a")
	out := doctor.Run(context.Background(), cfg, nil, doctor.Options{})
	c, ok := find(out, "storage.data_dir")
	if !ok || c.Status != doctor.StatusWarn {
		t.Fatalf("data_dir %+v", c)
	}
}

func TestStorageDiskFailAndWarn(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg("http://127.0.0.1:1", "a")
	cfg.Storage.DataDir = dir

	low := doctor.Run(context.Background(), cfg, nil, doctor.Options{
		DiskFree: func(string) (uint64, error) { return 100 << 20, nil },
	})
	c, _ := find(low, "storage.disk")
	if c.Status != doctor.StatusFail {
		t.Fatalf("low disk want fail got %s", c.Status)
	}

	mid := doctor.Run(context.Background(), cfg, nil, doctor.Options{
		DiskFree: func(string) (uint64, error) { return 2 << 30, nil },
	})
	c, _ = find(mid, "storage.disk")
	if c.Status != doctor.StatusWarn {
		t.Fatalf("mid disk want warn got %s", c.Status)
	}
}

func TestExitCodes(t *testing.T) {
	if doctor.ExitCode(nil, os.ErrNotExist) != 2 {
		t.Fatal("command err")
	}
	if doctor.ExitCode([]doctor.Check{{Status: doctor.StatusWarn}}, nil) != 0 {
		t.Fatal("warn only")
	}
	if doctor.ExitCode([]doctor.Check{{Status: doctor.StatusFail}}, nil) != 1 {
		t.Fatal("fail")
	}
}

func TestWritableDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	cfg := baseCfg("http://127.0.0.1:1", "a")
	cfg.Storage.DataDir = dir
	out := doctor.Run(context.Background(), cfg, nil, doctor.Options{
		DiskFree: func(string) (uint64, error) { return 10 << 30, nil },
	})
	c, ok := find(out, "storage.data_dir")
	if !ok || c.Status != doctor.StatusOK {
		t.Fatalf("data_dir %+v", c)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
