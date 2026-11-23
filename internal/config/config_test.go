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
