package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/api"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
)

func newHarness(t *testing.T, heavyURL, fastURL string) (*httptest.Server, *registry.Registry) {
	cfg := testConfig(t, heavyURL, fastURL)
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "mane")
	reg.RefreshMachine(context.Background(), "firmino")
	srv := api.NewServer(cfg, reg, g)
	return httptest.NewServer(srv.Handler()), reg
}

func TestNotFoundAPI(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/nope", nil)
	req.Host = "localhost"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestOriginRejected(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"x"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestThinkingSeparate(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Thinking: "hmm"},
			{Content: "ans"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts, _ := newHarness(t, hTs.URL, fTs.URL)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"x"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	s := string(body)
	if !strings.Contains(s, `"type":"thinking"`) || !strings.Contains(s, `"type":"token"`) {
		t.Fatalf("body %s", body)
	}
}

func TestStreamFalse(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "ok"},
			{Done: &fakeollama.DoneStats{EvalCount: 1, PromptEvalCount: 2}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts, _ := newHarness(t, hTs.URL, fTs.URL)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", bytes.NewReader([]byte(`{"prompt":"x","stream":false}`)))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out["text"] != "ok" {
		t.Fatalf("%v", out)
	}
}

func TestHealthVersion(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/health", nil)
	req.Host = "localhost"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var j map[string]any
	_ = json.NewDecoder(res.Body).Decode(&j)
	if j["ok"] != true || j["version"] == "" {
		t.Fatalf("%v", j)
	}
}

func TestDefaultModelMissingHTTP(t *testing.T) {
	heavy := &fakeollama.Server{Models: []string{"other:model"}}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts, _ := newHarness(t, hTs.URL, fTs.URL)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"x"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func fakePair(t *testing.T) (string, string) {
	h := fakeollama.NewHTTPTestServer(&fakeollama.Server{Models: []string{"qwen2.5:14b"}})
	f := fakeollama.NewHTTPTestServer(&fakeollama.Server{Models: []string{"llama3.1:8b"}})
	t.Cleanup(h.Close)
	t.Cleanup(f.Close)
	return h.URL, f.URL
}
