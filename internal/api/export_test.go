package api_test

import (
	"bytes"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/logx"
)

func TestBatchExportCSVAndMarkdown(t *testing.T) {
	var logBuf bytes.Buffer
	restore := logx.SetOutput(&logBuf)
	defer restore()

	ans := "the answer"
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

	body := `{"items":[{"prompt":"=formula"},{"prompt":"plain","check":{"mode":"review"}}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created struct {
		ID string `json:"id"`
	}
	_ = decodeJSON(res, &created)
	waitBatchDone(t, ts, created.ID)

	csvReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=csv", nil)
	csvReq.Host = "localhost"
	csvRes, err := http.DefaultClient.Do(csvReq)
	if err != nil {
		t.Fatal(err)
	}
	defer csvRes.Body.Close()
	if csvRes.StatusCode != http.StatusOK {
		t.Fatalf("csv status %d", csvRes.StatusCode)
	}
	if !strings.Contains(csvRes.Header.Get("Content-Disposition"), "pingpong-batch-") {
		t.Fatal("disposition")
	}
	csvBytes, _ := io.ReadAll(csvRes.Body)
	if !bytes.HasPrefix(csvBytes, []byte("index,")) && !bytes.HasPrefix(csvBytes, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("header %q", csvBytes[:40])
	}
	text := string(csvBytes)
	if !strings.Contains(text, "'=formula") {
		t.Fatalf("formula guard: %s", text)
	}
	if !strings.Contains(text, "review") || !strings.Contains(text, "answer") {
		t.Fatalf("stages: %s", text)
	}
	records, err := csv.NewReader(strings.NewReader(text)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 3 {
		t.Fatalf("expected answer+review rows, got %d", len(records))
	}

	mdReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=md", nil)
	mdReq.Host = "localhost"
	mdRes, _ := http.DefaultClient.Do(mdReq)
	mdBody, _ := io.ReadAll(mdRes.Body)
	mdRes.Body.Close()
	if !strings.Contains(string(mdBody), "## Item") {
		t.Fatal("markdown sections")
	}

	rawReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=csv&raw=1", nil)
	rawReq.Host = "localhost"
	rawRes, _ := http.DefaultClient.Do(rawReq)
	rawBytes, _ := io.ReadAll(rawRes.Body)
	rawRes.Body.Close()
	if strings.Contains(string(rawBytes), "'=formula") {
		t.Fatal("raw=1 should skip guard")
	}

	badFmt, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=pdf", nil)
	badFmt.Host = "localhost"
	badRes, _ := http.DefaultClient.Do(badFmt)
	if badRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad format %d", badRes.StatusCode)
	}
	logs := logBuf.String()
	if strings.Contains(logs, ans) || strings.Contains(logs, "=formula") {
		t.Fatalf("content in logs: %s", logs)
	}
	if !strings.Contains(logs, `"event":"export_done"`) {
		t.Fatalf("no export_done: %s", logs)
	}
}

func TestJobsExportSkippedHeader(t *testing.T) {
	mS, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = []fakeollama.Step{{Content: "x"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	jreq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"prompt":"solo"}`))
	jreq.Host = "localhost"
	jreq.Header.Set("Content-Type", "application/json")
	jres, _ := http.DefaultClient.Do(jreq)
	var job map[string]string
	_ = decodeJSON(jres, &job)
	waitJobDone(t, ts, job["id"])

	exp, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/export?ids="+job["id"]+",missing-id&format=csv", nil)
	exp.Host = "localhost"
	eres, _ := http.DefaultClient.Do(exp)
	if eres.Header.Get("X-Pingpong-Skipped") != "1" {
		t.Fatalf("skipped %s", eres.Header.Get("X-Pingpong-Skipped"))
	}
}

func TestExportPartialAndThinking(t *testing.T) {
	mS, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	steps := []fakeollama.Step{{Thinking: "secret thought", Content: "out"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	mS.ChatSteps = steps
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(`{"items":[{"prompt":"one"}]}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created struct {
		ID string `json:"id"`
	}
	_ = decodeJSON(res, &created)

	exp, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=csv", nil)
	exp.Host = "localhost"
	eres, _ := http.DefaultClient.Do(exp)
	body, _ := io.ReadAll(eres.Body)
	eres.Body.Close()
	if strings.Contains(string(body), "waiting") {
		t.Fatal("unfinished excluded by default")
	}

	exp2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=csv&partial=1&include_thinking=1", nil)
	exp2.Host = "localhost"
	eres2, _ := http.DefaultClient.Do(exp2)
	body2, _ := io.ReadAll(eres2.Body)
	eres2.Body.Close()
	if !strings.Contains(string(body2), "thinking") && !strings.Contains(string(body2), "secret") {
		// header has thinking; row may still be running
		if !strings.Contains(string(body2), "running") && !strings.Contains(string(body2), "waiting") {
			t.Fatalf("partial export %s", body2)
		}
	}
	waitBatchDone(t, ts, created.ID)
}

func waitBatchDone(t *testing.T, ts *httptest.Server, id string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var view map[string]any
		_ = decodeJSON(gr, &view)
		if view["pending"] == float64(0) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("batch timeout")
}
