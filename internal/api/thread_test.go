package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestThreadConversation(t *testing.T) {
	done := []fakeollama.Step{{Content: "reply one"}, {Done: &fakeollama.DoneStats{EvalCount: 3, PromptEvalCount: 10}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads", strings.NewReader(`{"machine":"firmino","model":"qwen2.5:14b"}`))
	create.Host = "localhost"
	create.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(create)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d", res.StatusCode)
	}
	var th map[string]any
	_ = decodeJSON(res, &th)
	tid := th["id"].(string)
	if th["machine"] != "firmino" || th["model"] != "qwen2.5:14b" {
		t.Fatalf("pair %+v", th)
	}

	sendTurn(t, ts, tid, "hello", false)
	waitThreadDone(t, ts, tid, 1)
	calls := append(fS.Calls(), mS.Calls()...)
	if len(calls) != 1 || len(calls[0].Messages) != 1 || calls[0].Messages[0].Content != "hello" {
		t.Fatalf("turn1 messages %+v", calls[0].Messages)
	}

	sendTurn(t, ts, tid, "follow up", false)
	waitThreadDone(t, ts, tid, 2)
	calls = append(fS.Calls(), mS.Calls()...)
	last := calls[len(calls)-1]
	if len(last.Messages) != 3 {
		t.Fatalf("turn2 history %d", len(last.Messages))
	}
	if last.Messages[0].Role != "user" || last.Messages[1].Role != "assistant" || last.Messages[2].Role != "user" {
		t.Fatal("bad roles")
	}

	get, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/threads/"+tid, nil)
	get.Host = "localhost"
	gres, _ := http.DefaultClient.Do(get)
	var detail map[string]any
	_ = decodeJSON(gres, &detail)
	turns := detail["turns"].([]any)
	if len(turns) != 2 {
		t.Fatalf("turns %d", len(turns))
	}
}

func TestThreadBusyAndTooLong(t *testing.T) {
	mS, _, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = []fakeollama.Step{{Content: "slow"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads", strings.NewReader(`{"machine":"mane","model":"llama3.1:8b"}`))
	create.Host = "localhost"
	create.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(create)
	var th map[string]any
	_ = decodeJSON(res, &th)
	tid := th["id"].(string)

	msg1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads/"+tid+"/messages", strings.NewReader(`{"content":"first"}`))
	msg1.Host = "localhost"
	msg1.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(msg1)

	msg2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads/"+tid+"/messages", strings.NewReader(`{"content":"second"}`))
	msg2.Host = "localhost"
	msg2.Header.Set("Content-Type", "application/json")
	r2, _ := http.DefaultClient.Do(msg2)
	var errBody map[string]any
	_ = decodeJSON(r2, &errBody)
	if r2.StatusCode != 409 || errBody["error"].(map[string]any)["code"] != "THREAD_BUSY" {
		t.Fatalf("busy %d %+v", r2.StatusCode, errBody)
	}
}

func TestThreadRetentionExempt(t *testing.T) {
	done := []fakeollama.Step{{Content: "ok"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	_, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads", strings.NewReader(`{"machine":"firmino","model":"qwen2.5:14b"}`))
	create.Host = "localhost"
	create.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(create)
	var th map[string]any
	_ = decodeJSON(res, &th)
	tid := th["id"].(string)
	sendTurn(t, ts, tid, "old msg", false)
	waitThreadDone(t, ts, tid, 1)

	// Bump thread updated_at via another message so retention is from last activity.
	sendTurn(t, ts, tid, "fresh", false)
	waitThreadDone(t, ts, tid, 2)

	get, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/threads/"+tid, nil)
	get.Host = "localhost"
	if gres, _ := http.DefaultClient.Do(get); gres.StatusCode != 200 {
		t.Fatalf("thread gone early %d", gres.StatusCode)
	}
}

func sendTurn(t *testing.T, ts *httptest.Server, tid, content string, trim bool) {
	t.Helper()
	body := `{"content":` + jsonString(content) + `}`
	if trim {
		body = `{"content":` + jsonString(content) + `,"trim":"oldest"}`
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads/"+tid+"/messages", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusAccepted {
		var e map[string]any
		_ = decodeJSON(res, &e)
		t.Fatalf("send %d %+v", res.StatusCode, e)
	}
}

func waitThreadDone(t *testing.T, ts *httptest.Server, tid string, turn int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		get, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/threads/"+tid, nil)
		get.Host = "localhost"
		res, _ := http.DefaultClient.Do(get)
		var detail map[string]any
		_ = decodeJSON(res, &detail)
		turns, _ := detail["turns"].([]any)
		for _, raw := range turns {
			row := raw.(map[string]any)
			if int(row["turn"].(float64)) == turn && row["status"] == "done" {
				return
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("turn %d not done", turn)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestThreadDeleteAndPin(t *testing.T) {
	done := []fakeollama.Step{{Content: "a"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/threads", strings.NewReader(`{"machine":"firmino","model":"qwen2.5:14b"}`))
	create.Host = "localhost"
	create.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(create)
	var th map[string]any
	_ = decodeJSON(res, &th)
	tid := th["id"].(string)
	sendTurn(t, ts, tid, "one", false)
	waitThreadDone(t, ts, tid, 1)

	patch, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/threads/"+tid, strings.NewReader(
		`{"machine":"mane","model":"llama3.1:8b"}`))
	patch.Host = "localhost"
	patch.Header.Set("Content-Type", "application/json")
	pres, _ := http.DefaultClient.Do(patch)
	if pres.StatusCode != 200 {
		t.Fatalf("patch pin %d", pres.StatusCode)
	}
	sendTurn(t, ts, tid, "two", false)
	waitThreadDone(t, ts, tid, 2)
	calls := mS.Calls()
	if len(calls) != 1 || calls[0].Model != "llama3.1:8b" {
		t.Fatalf("pin not applied to mane calls=%d", len(calls))
	}

	del, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/threads/"+tid, nil)
	del.Host = "localhost"
	dres, _ := http.DefaultClient.Do(del)
	if dres.StatusCode != 204 {
		t.Fatalf("delete %d", dres.StatusCode)
	}
	get, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/threads/"+tid, nil)
	get.Host = "localhost"
	gres, _ := http.DefaultClient.Do(get)
	if gres.StatusCode != 404 {
		t.Fatalf("still exists %d", gres.StatusCode)
	}
}
