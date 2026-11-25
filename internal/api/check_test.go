package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/api"
	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func TestCheckDefaultPairingAndReviewPrompt(t *testing.T) {
	ans := "the draft answer"
	review := "OK"
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.Reply = func(call fakeollama.Call) []fakeollama.Step {
		return []fakeollama.Step{
			{Content: ans},
			{Done: &fakeollama.DoneStats{EvalCount: 5, PromptEvalCount: 3}},
		}
	}
	fS.Reply = func(call fakeollama.Call) []fakeollama.Step {
		return []fakeollama.Step{
			{Content: review},
			{Done: &fakeollama.DoneStats{EvalCount: 2}},
		}
	}
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	body := `{"prompt":"Explain Y","check":{"mode":"review"}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created map[string]string
	_ = decodeJSON(res, &created)
	id := created["id"]

	waitJobDone(t, ts, id)
	waitJobChildDone(t, ts, id)

	if len(mS.Calls()) != 1 || mS.Calls()[0].Model != "llama3.1:8b" {
		t.Fatalf("mane calls %+v", mS.Calls())
	}
	if len(fS.Calls()) != 1 || fS.Calls()[0].Model != "qwen2.5:14b" {
		t.Fatalf("firmino calls %+v", fS.Calls())
	}
	prompt := fS.Calls()[0].Messages[0].Content
	if !strings.Contains(prompt, "You are checking another model's answer") {
		t.Fatal("missing review header")
	}
	if !strings.Contains(prompt, "Explain Y") || !strings.Contains(prompt, ans) {
		t.Fatal("question or answer missing from review prompt")
	}

	g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
	g.Host = "localhost"
	gr, _ := http.DefaultClient.Do(g)
	var detail map[string]any
	_ = decodeJSON(gr, &detail)
	ch := detail["check"].(map[string]any)
	if ch["state"] != "done" {
		t.Fatalf("check state %v", ch["state"])
	}
}

func TestCheckChatRejected(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"x","check":{"mode":"review"}}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestCheckOversizeContext(t *testing.T) {
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = []fakeollama.Step{
		{Content: strings.Repeat("word ", 5000)},
		{Done: &fakeollama.DoneStats{EvalCount: 4000, PromptEvalCount: 4000}},
	}
	fS.ChatSteps = []fakeollama.Step{{Content: "ok"}, {Done: &fakeollama.DoneStats{}}}
	ts := newHarnessJobsSmallCtx(t, mURL, fURL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"prompt":"big","check":{"mode":"review"}}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created map[string]string
	_ = decodeJSON(res, &created)
	waitJobDone(t, ts, created["id"])
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+created["id"], nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var detail map[string]any
		_ = decodeJSON(gr, &detail)
		if ch, ok := detail["check"].(map[string]any); ok && ch["state"] == "unchecked" {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("expected unchecked after CONTEXT_TOO_LARGE")
}

func waitJobDone(t *testing.T, ts *httptest.Server, id string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var detail map[string]any
		_ = decodeJSON(gr, &detail)
		if detail["status"] == "done" {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("job %s not done", id)
}

func waitJobChildDone(t *testing.T, ts *httptest.Server, parentID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+parentID, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var detail map[string]any
		_ = decodeJSON(gr, &detail)
		ch, ok := detail["check"].(map[string]any)
		if !ok {
			time.Sleep(40 * time.Millisecond)
			continue
		}
		if ch["state"] == "done" || ch["state"] == "unchecked" {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("check child not finished")
}

func newHarnessJobsSmallCtx(t *testing.T, maneURL, firminoURL string) *httptest.Server {
	t.Helper()
	cfg := fakeollama.TestConfigYAML(t, maneURL, firminoURL, "")
	for i := range cfg.Machines {
		if cfg.Machines[i].Name == "firmino" {
			cfg.Machines[i].NumCtx = 512
		}
	}
	cfg.Storage.DataDir = t.TempDir()
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(context.Background(), "mane")
	reg.RefreshMachine(context.Background(), "firmino")
	st, err := store.Open(cfg.Storage.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	jobSvc := jobs.New(cfg, st, reg, g, clock.Real{})
	jobSvc.Start(context.Background())
	srv := api.NewServer(cfg, reg, g, jobSvc)
	return httptest.NewServer(srv.Handler())
}
