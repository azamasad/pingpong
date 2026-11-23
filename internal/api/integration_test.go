package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/api"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
)

func testConfig(t *testing.T, heavyURL, fastURL string) *config.Config {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
router:
  listen: "127.0.0.1:0"
  allowed_hosts: ["localhost", "127.0.0.1"]
machines:
  - name: mane
    address: "` + fastURL + `"
    roles: [fast]
    default_model: "llama3.1:8b"
  - name: firmino
    address: "` + heavyURL + `"
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
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestChatAutoStream(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "ok"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()

	cfg := testConfig(t, hTs.URL, fTs.URL)
	limits := map[string]int{"mane": 1, "firmino": 1}
	g := gate.New(limits)
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "mane")
	reg.RefreshMachine(context.Background(), "firmino")
	srv := api.NewServer(cfg, reg, g, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"prompt":"hi"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	sc := bufio.NewScanner(res.Body)
	var types []string
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		types = append(types, m["type"].(string))
	}
	if types[0] != "route" || types[len(types)-1] != "done" {
		t.Fatalf("events %v", types)
	}
}

func TestHostRejected(t *testing.T) {
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	cfg := testConfig(t, hTs.URL, fTs.URL)
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	srv := api.NewServer(cfg, registry.New(cfg, g), g, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/health", nil)
	req.Host = "evil.example"
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 403 {
		t.Fatalf("got %d", res.StatusCode)
	}
}
