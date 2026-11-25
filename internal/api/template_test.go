package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/logx"
)

func TestTemplateCRUDAndUse(t *testing.T) {
	done := []fakeollama.Step{{Content: "summary ok"}, {Done: &fakeollama.DoneStats{EvalCount: 2}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/templates", strings.NewReader(
		`{"name":"summary","body":"Summarise {{text}} in {{words}} words."}`))
	create.Host = "localhost"
	create.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(create)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d", res.StatusCode)
	}

	list, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/templates", nil)
	list.Host = "localhost"
	lr, _ := http.DefaultClient.Do(list)
	var names []map[string]any
	_ = decodeJSON(lr, &names)
	if len(names) != 1 || names[0]["name"] != "summary" {
		t.Fatalf("list %+v", names)
	}

	jobBody := `{"template":"summary","vars":{"text":"hello world","words":"10","extra":"x"}}`
	jreq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(jobBody))
	jreq.Host = "localhost"
	jreq.Header.Set("Content-Type", "application/json")
	jres, _ := http.DefaultClient.Do(jreq)
	var jobOut map[string]any
	_ = decodeJSON(jres, &jobOut)
	if jres.StatusCode != http.StatusAccepted {
		t.Fatalf("job %d", jres.StatusCode)
	}
	wraw, _ := jobOut["warnings"].([]any)
	if len(wraw) != 1 || wraw[0] != "unused var: extra" {
		t.Fatalf("warnings %v", jobOut["warnings"])
	}
	id := jobOut["id"].(string)
	waitJobDone(t, ts, id)
	calls := append(mS.Calls(), fS.Calls()...)
	if len(calls) != 1 {
		t.Fatalf("calls %d", len(calls))
	}
	got := calls[0].Messages[0].Content
	want := "Summarise hello world in 10 words."
	if got != want {
		t.Fatalf("prompt %q", got)
	}

	batch := `{"template":"summary","vars":{"words":"5"},"rows":[{"text":"a"},{"text":"b"},{"text":"c"}]}`
	breq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(batch))
	breq.Host = "localhost"
	breq.Header.Set("Content-Type", "application/json")
	bres, _ := http.DefaultClient.Do(breq)
	var bout map[string]any
	_ = decodeJSON(bres, &bout)
	if bres.StatusCode != http.StatusAccepted || len(bout["job_ids"].([]any)) != 3 {
		t.Fatalf("batch %+v", bout)
	}

	del, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/templates/summary", nil)
	del.Host = "localhost"
	dres, _ := http.DefaultClient.Do(del)
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("delete %d", dres.StatusCode)
	}
	g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
	g.Host = "localhost"
	gr, _ := http.DefaultClient.Do(g)
	var detail map[string]any
	_ = decodeJSON(gr, &detail)
	if detail["prompt_preview"] == "" {
		t.Fatal("job should keep final prompt after template delete")
	}
}

func TestTemplateMissingVars(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/templates", strings.NewReader(
		`{"name":"t1","body":"{{a}} {{b}}"}`))
	create.Host = "localhost"
	create.Header.Set("Content-Type", "application/json")
	_, _ = http.DefaultClient.Do(create)

	jreq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"template":"t1","vars":{"a":"only"}}`))
	jreq.Host = "localhost"
	jreq.Header.Set("Content-Type", "application/json")
	jres, _ := http.DefaultClient.Do(jreq)
	if jres.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", jres.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(jres.Body).Decode(&body)
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "TEMPLATE_VARS_MISSING" {
		t.Fatalf("code %v", errObj["code"])
	}
	miss := errObj["missing"].([]any)
	if len(miss) != 1 || miss[0] != "b" {
		t.Fatalf("missing %v", miss)
	}
}

func TestTemplateNoRescan(t *testing.T) {
	done := []fakeollama.Step{{Content: "x"}, {Done: &fakeollama.DoneStats{}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	_, _ = http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/templates", `{"name":"inj","body":"{{a}}"}`))
	jres, _ := http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/jobs",
		`{"template":"inj","vars":{"a":"{{not_a_blank}}"}}`))
	var jobOut map[string]any
	_ = decodeJSON(jres, &jobOut)
	waitJobDone(t, ts, jobOut["id"].(string))
	calls := append(mS.Calls(), fS.Calls()...)
	if len(calls) != 1 || calls[0].Messages[0].Content != "{{not_a_blank}}" {
		t.Fatalf("rescan %+v", calls)
	}
}

func TestTemplateChatRejected(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()
	res, _ := http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/chat",
		`{"prompt":"x","template":"nope"}`))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestTemplateAtomicBatchRow(t *testing.T) {
	_, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()
	_, _ = http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/templates",
		`{"name":"row","body":"{{x}}"}`))
	batch := `{"template":"row","rows":[{"x":"ok"},{"x":""}]}`
	res, _ := http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/batches", batch))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected atomic failure, got %d", res.StatusCode)
	}
}

func TestTemplateBodyNotLogged(t *testing.T) {
	var buf bytes.Buffer
	restore := logx.SetOutput(&buf)
	defer restore()

	_, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	secret := "SECRET_TEMPLATE_BODY_XYZ"
	create := `{"name":"logt","body":"{{t}} ` + secret + `"}`
	_, _ = http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/templates", create))
	_, _ = http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/jobs",
		`{"template":"logt","vars":{"t":"v"}}`))
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("template body in logs: %s", buf.String())
	}
}

func mustReq(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	return req
}