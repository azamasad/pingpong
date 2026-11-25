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

func openAIServer(t *testing.T, heavy, fast *fakeollama.Server, aliases map[string]config.Alias) (*httptest.Server, *config.Config) {
	t.Helper()
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	t.Cleanup(hTs.Close)
	t.Cleanup(fTs.Close)
	cfg := testConfig(t, hTs.URL, fTs.URL)
	cfg.OpenAI.Enabled = true
	cfg.Aliases = aliases
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "mane")
	reg.RefreshMachine(context.Background(), "firmino")
	ts := httptest.NewServer(api.NewServer(cfg, reg, g, nil).Handler())
	t.Cleanup(ts.Close)
	return ts, cfg
}

func TestOpenAIDisabled404(t *testing.T) {
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	cfg := testConfig(t, hTs.URL, fTs.URL)
	ts := httptest.NewServer(api.NewServer(cfg, registry.New(cfg, gate.New(map[string]int{"mane": 1, "firmino": 1})), gate.New(map[string]int{"mane": 1, "firmino": 1}), nil).Handler())
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/models", nil)
	req.Host = "localhost"
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 404 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestOpenAIModelsList(t *testing.T) {
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	ts, _ := openAIServer(t, heavy, fast, map[string]config.Alias{"quick": {Model: "llama3.1:8b"}})
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/models", nil)
	req.Host = "localhost"
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	data := body["data"].([]any)
	ids := map[string]bool{}
	for _, row := range data {
		ids[row.(map[string]any)["id"].(string)] = true
	}
	for _, want := range []string{"qwen2.5:14b", "llama3.1:8b", "quick", "pingpong-auto", "pingpong-fast"} {
		if !ids[want] {
			t.Fatalf("missing model %q in %v", want, ids)
		}
	}
}

func TestOpenAIRejectsTools(t *testing.T) {
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	ts, _ := openAIServer(t, heavy, fast, nil)
	body := `{"model":"pingpong-auto","messages":[{"role":"user","content":"hi"}],"tools":[]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-dummy")
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var errBody map[string]any
	_ = json.NewDecoder(res.Body).Decode(&errBody)
	errObj := errBody["error"].(map[string]any)
	if errObj["type"] != "invalid_request_error" {
		t.Fatalf("type %v", errObj["type"])
	}
}

func TestOpenAINonStreamCompletion(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "hello"},
			{Done: &fakeollama.DoneStats{EvalCount: 3, PromptEvalCount: 5}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	ts, _ := openAIServer(t, heavy, fast, nil)
	body := `{"model":"pingpong-auto","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out["object"] != "chat.completion" {
		t.Fatalf("object %v", out["object"])
	}
	usage := out["usage"].(map[string]any)
	if int(usage["prompt_tokens"].(float64)) != 5 || int(usage["completion_tokens"].(float64)) != 3 {
		t.Fatalf("usage %v", usage)
	}
	pp := out["pingpong"].(map[string]any)
	if pp["machine"] != "firmino" {
		t.Fatalf("machine %v", pp["machine"])
	}
}

func TestOpenAIStreamSSE(t *testing.T) {
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fast := &fakeollama.Server{
		Models: []string{"llama3.1:8b"},
		ChatSteps: []fakeollama.Step{
			{Content: "a"},
			{Content: "b"},
			{Done: &fakeollama.DoneStats{EvalCount: 2, PromptEvalCount: 1}},
		},
	}
	ts, _ := openAIServer(t, heavy, fast, nil)
	body := `{"model":"pingpong-fast","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	sc := bufio.NewScanner(res.Body)
	var deltas []string
	var done bool
	for sc.Scan() {
		line := sc.Text()
		if line == "data: [DONE]" {
			done = true
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var chunk map[string]any
		_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk)
		if chunk["object"] != "chat.completion.chunk" {
			continue
		}
		choices := chunk["choices"].([]any)
		delta := choices[0].(map[string]any)["delta"].(map[string]any)
		if c, ok := delta["content"].(string); ok && c != "" {
			deltas = append(deltas, c)
		}
	}
	if !done {
		t.Fatal("missing [DONE]")
	}
	if strings.Join(deltas, "") != "ab" {
		t.Fatalf("deltas %v", deltas)
	}
}

func TestOpenAIMachineModelPin(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "ok"},
			{Done: &fakeollama.DoneStats{EvalCount: 1, PromptEvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	ts, _ := openAIServer(t, heavy, fast, nil)
	body := `{"model":"mane/llama3.1:8b","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	pp := out["pingpong"].(map[string]any)
	if pp["machine"] != "mane" {
		t.Fatalf("machine %v", pp["machine"])
	}
}

func TestOpenAITextPartsContent(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "xy"},
			{Done: &fakeollama.DoneStats{EvalCount: 1, PromptEvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	ts, _ := openAIServer(t, heavy, fast, nil)
	body := `{"model":"pingpong-auto","messages":[{"role":"user","content":[{"type":"text","text":"x"},{"type":"text","text":"y"}]}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
}
