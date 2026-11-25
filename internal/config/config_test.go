package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExample(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "config.example.yaml")
	cfg, warnings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Logf("warnings: %v", warnings)
	}
	if cfg.Router.Listen != "127.0.0.1:8787" {
		t.Fatalf("listen: %s", cfg.Router.Listen)
	}
	if cfg.Timeouts.FirstTokenTimeout.Duration != 30*60*1e9 {
		t.Fatalf("first_token_timeout: %v", cfg.Timeouts.FirstTokenTimeout.Duration)
	}
}

func TestValidationRejectsBadListen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	content := `
machines:
  - name: a
    address: "http://127.0.0.1:11434"
    roles: [fast, heavy]
    default_model: "m:8b"
routing:
  auto_role: heavy
  fast_role: fast
router:
  listen: "0.0.0.0:8787"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("expected localhost bind error, got %v", err)
	}
}

func TestDuplicateMachineName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	content := `
machines:
  - name: x
    address: "http://127.0.0.1:1"
    roles: [fast, heavy]
    default_model: "m"
  - name: x
    address: "http://127.0.0.1:2"
    roles: [heavy]
    default_model: "m"
routing:
  auto_role: heavy
  fast_role: fast
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("got %v", err)
	}
}

func TestDefaultProfiles(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "config.example.yaml")
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Profiles["fast"].Fast {
		t.Fatal("fast profile should set fast")
	}
	if _, ok := cfg.Profiles["balanced"]; !ok {
		t.Fatal("missing balanced")
	}
	if _, ok := cfg.Profiles["deep"]; !ok {
		t.Fatal("missing deep")
	}
	fast, model, machine, err := cfg.ApplyProfile("fast", false, "", "")
	if err != nil || !fast || model != "" || machine != "" {
		t.Fatalf("%v %v %s %s", err, fast, model, machine)
	}
	_, _, _, err = cfg.ApplyProfile("nope", false, "", "")
	if err == nil {
		t.Fatal("expected unknown profile")
	}
	fast, model, _, err = cfg.ApplyProfile("fast", false, "qwen2.5:14b", "")
	if err != nil || !fast || model != "qwen2.5:14b" {
		t.Fatalf("pin should win: %v %v %s", err, fast, model)
	}
}

func TestStorageDataDirExpandsHome(t *testing.T) {
	t.Setenv("HOME", "/home/pp")
	cases := map[string]string{
		"~/data":     "/home/pp/data",
		"~":          "/home/pp",
		"/abs/data":  "/abs/data",
		"rel/data":   "rel/data",
		"":           "",
		"~other/dir": "~other/dir",
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNotifyWebhookRejectsBadScheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	content := `
machines:
  - name: a
    address: "http://127.0.0.1:11434"
    roles: [fast]
    default_model: "m:8b"
routing:
  auto_role: fast
  fast_role: fast
notify:
  webhook_url: "ftp://example.com/topic"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "notify.webhook_url") {
		t.Fatalf("expected notify url error: %v", err)
	}
}

func TestStatsAddressRejectsUserInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	content := `
machines:
  - name: a
    address: "http://127.0.0.1:11434"
    stats_address: "http://user:pass@10.0.0.8:8788"
    roles: [fast]
    default_model: "m:8b"
routing:
  auto_role: fast
  fast_role: fast
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "stats_address") {
		t.Fatalf("expected stats_address error: %v", err)
	}
}

func TestRefreshOnMissDefaultsOn(t *testing.T) {
	var h HealthConfig
	if !h.RefreshOnMiss() {
		t.Fatal("omitted key should default to on")
	}
	off := false
	h.RefreshOnModelMiss = &off
	if h.RefreshOnMiss() {
		t.Fatal("explicit false should disable")
	}
}
