package fakeollama

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/azamasad/pingpong/internal/config"
)

// Pair starts two fake Ollama servers named mane (fast) and firmino (heavy) with
// different default models, for split-batch tests.
func Pair(t *testing.T, maneModels, firminoModels []string) (mane, firmino *Server, maneURL, firminoURL string) {
	t.Helper()
	mS := &Server{Models: maneModels}
	fS := &Server{Models: firminoModels}
	mTs := NewHTTPTestServer(mS)
	fTs := NewHTTPTestServer(fS)
	t.Cleanup(func() { mTs.Close(); fTs.Close() })
	return mS, fS, mTs.URL, fTs.URL
}

// TestConfigYAML builds a minimal router config pointing at the given machine URLs.
func TestConfigYAML(t *testing.T, maneURL, firminoURL string, extra string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
router:
  listen: "127.0.0.1:0"
  allowed_hosts: ["localhost", "127.0.0.1"]
machines:
  - name: mane
    address: "` + maneURL + `"
    roles: [fast]
    default_model: "llama3.1:8b"
  - name: firmino
    address: "` + firminoURL + `"
    roles: [heavy]
    default_model: "qwen2.5:14b"
routing:
  auto_role: heavy
  fast_role: fast
health:
  interval: 1h
  refresh_on_model_miss: true
timeouts:
  first_token_timeout: 30s
  idle_token_timeout: 5s
  heartbeat_interval: 1s
` + extra
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
