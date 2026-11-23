package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func decodeJSON(res *http.Response, v any) error {
	defer res.Body.Close()
	return json.NewDecoder(res.Body).Decode(v)
}

func readAll(res *http.Response) string {
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func TestJobEventsReplayFromSeq(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "a"},
			{Content: "b"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts := newHarnessJobs(t, hTs.URL, fTs.URL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"prompt":"hi"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created map[string]string
	if err := decodeJSON(res, &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"]

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var j map[string]any
		_ = decodeJSON(gr, &j)
		if j["status"] == "done" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	evReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id+"/events?from=9999", nil)
	evReq.Host = "localhost"
	evRes, err := http.DefaultClient.Do(evReq)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(evRes)
	_ = evRes.Body.Close()
	if strings.TrimSpace(body) != "" {
		t.Fatalf("high from should be empty: %q", body)
	}
	ev1, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id+"/events?from=1", nil)
	ev1.Host = "localhost"
	ev1Res, _ := http.DefaultClient.Do(ev1)
	body1 := readAll(ev1Res)
	_ = ev1Res.Body.Close()
	if !strings.Contains(body1, `"text":"a"`) || !strings.Contains(body1, `"text":"b"`) {
		t.Fatalf("full replay: %s", body1)
	}
}

func TestJobCancelWhileRunning(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Wait: 2 * time.Second},
			{Content: "late"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts := newHarnessJobs(t, hTs.URL, fTs.URL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"prompt":"hi"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created map[string]string
	_ = decodeJSON(res, &created)
	id := created["id"]

	time.Sleep(100 * time.Millisecond)
	cReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs/"+id+"/cancel", nil)
	cReq.Host = "localhost"
	cRes, _ := http.DefaultClient.Do(cReq)
	_ = cRes.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var j map[string]any
		_ = decodeJSON(gr, &j)
		if j["status"] == "cancelled" {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("not cancelled")
}

func TestJobCompletesWithoutClientAttached(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "solo"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{Models: []string{"llama3.1:8b"}}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts := newHarnessJobs(t, hTs.URL, fTs.URL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"prompt":"hi"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created map[string]string
	_ = decodeJSON(res, &created)
	id := created["id"]

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var j map[string]any
		_ = decodeJSON(gr, &j)
		if j["status"] == "done" && j["output_text"] == "solo" {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("job did not finish alone")
}
