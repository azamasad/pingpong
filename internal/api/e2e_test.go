package api_test

// End-to-end flows against fake Ollama workers (automated acceptance from TESTING.md).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestE2EAcceptanceFlows(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "from-heavy"},
			{Done: &fakeollama.DoneStats{EvalCount: 3, PromptEvalCount: 10}},
		},
	}
	fast := &fakeollama.Server{
		Models: []string{"llama3.1:8b", "llama3.2:3b"},
		ChatSteps: []fakeollama.Step{
			{Content: "from-fast"},
			{Done: &fakeollama.DoneStats{EvalCount: 2}},
		},
	}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts, _ := newHarness(t, hTs.URL, fTs.URL)
	defer ts.Close()

	cases := []struct {
		name        string
		body        string
		wantMachine string
		wantText    string
	}{
		{"auto", `{"prompt":"hello"}`, "firmino", "from-heavy"},
		{"fast", `{"prompt":"hello","fast":true}`, "mane", "from-fast"},
		{"pin_3b", `{"prompt":"hello","model":"llama3.2:3b"}`, "mane", "from-fast"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(tc.body))
			req.Host = "localhost"
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				b, _ := io.ReadAll(res.Body)
				t.Fatalf("status %d: %s", res.StatusCode, b)
			}
			var machine, text string
			sc := bufio.NewScanner(res.Body)
			for sc.Scan() {
				var ev map[string]any
				_ = json.Unmarshal(sc.Bytes(), &ev)
				if ev["type"] == "route" {
					machine = ev["machine"].(string)
				}
				if ev["type"] == "token" {
					text += ev["text"].(string)
				}
			}
			if machine != tc.wantMachine {
				t.Fatalf("machine %q want %q", machine, tc.wantMachine)
			}
			if text != tc.wantText {
				t.Fatalf("text %q", text)
			}
		})
	}
}

func TestE2EPinNotInstalled(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", bytes.NewReader([]byte(`{"prompt":"x","model":"no-such:7b"}`)))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestE2EMachinesAndModels(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/machines", nil)
	req.Host = "localhost"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("machines %d", len(list))
	}
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/models", nil)
	req2.Host = "localhost"
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	var models []map[string]any
	_ = json.NewDecoder(res2.Body).Decode(&models)
	if len(models) < 2 {
		t.Fatalf("models %v", models)
	}
	_ = context.Background()
}
