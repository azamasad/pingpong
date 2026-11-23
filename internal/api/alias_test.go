package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/api"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
)

func TestAliasRoutesAndRouteEvent(t *testing.T) {
	fast := &fakeollama.Server{
		Models: []string{"llama3.2:3b"},
		ChatSteps: []fakeollama.Step{
			{Content: "ok"},
			{Done: &fakeollama.DoneStats{EvalCount: 1, PromptEvalCount: 1}},
		},
	}
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fTs := fakeollama.NewHTTPTestServer(fast)
	hTs := fakeollama.NewHTTPTestServer(heavy)
	defer fTs.Close()
	defer hTs.Close()

	cfg := testConfig(t, hTs.URL, fTs.URL)
	cfg.Aliases = map[string]config.Alias{
		"quick": {Model: "llama3.2:3b"},
	}
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "mane")
	reg.RefreshMachine(context.Background(), "firmino")
	ts := httptest.NewServer(api.NewServer(cfg, reg, g, nil).Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"hi","model":"quick"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var sawAlias bool
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		var ev map[string]any
		_ = json.Unmarshal(sc.Bytes(), &ev)
		if ev["type"] == "route" && ev["alias"] == "quick" {
			sawAlias = true
		}
	}
	if !sawAlias {
		t.Fatal("expected alias on route event")
	}
}
