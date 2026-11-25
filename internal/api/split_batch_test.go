package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestSplitBatchAssignmentAndAtomic(t *testing.T) {
	doneStep := []fakeollama.Step{{Content: "ok"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t,
		[]string{"llama3.1:8b"},
		[]string{"qwen2.5:14b"},
	)
	mS.ChatSteps = doneStep
	fS.ChatSteps = doneStep
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	body := `{"items":[`
	for i := 0; i < 10; i++ {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"prompt":"p%d"}`, i)
	}
	body += `],"split":{"assign":"weights","targets":[{"machine":"mane","model":"llama3.1:8b","weight":3},{"machine":"firmino","model":"qwen2.5:14b","weight":2}]}}`

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID    string `json:"id"`
		Split struct {
			Targets []struct {
				Machine string `json:"machine"`
				Count   int    `json:"count"`
			} `json:"targets"`
		} `json:"split"`
	}
	if err := decodeJSON(res, &created); err != nil {
		t.Fatal(err)
	}
	if created.Split.Targets[0].Count != 6 || created.Split.Targets[1].Count != 4 {
		t.Fatalf("split counts %+v", created.Split.Targets)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID, nil)
		g.Host = "localhost"
		gr, _ := http.DefaultClient.Do(g)
		var view map[string]any
		_ = decodeJSON(gr, &view)
		if view["pending"] == float64(0) {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if len(mS.Calls()) != 6 {
		t.Fatalf("mane calls %d", len(mS.Calls()))
	}
	if len(fS.Calls()) != 4 {
		t.Fatalf("firmino calls %d", len(fS.Calls()))
	}
	for _, c := range mS.Calls() {
		if c.Model != "llama3.1:8b" {
			t.Fatalf("mane saw model %q", c.Model)
		}
	}
	for _, c := range fS.Calls() {
		if c.Model != "qwen2.5:14b" {
			t.Fatalf("firmino saw model %q", c.Model)
		}
	}
}

func TestSplitValidationAtomic(t *testing.T) {
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = []fakeollama.Step{{Done: &fakeollama.DoneStats{}}}
	fS.ChatSteps = mS.ChatSteps
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	bad := `{"items":[{"prompt":"a"},{"prompt":"b"}],"split":{"assign":"weights","targets":[{"machine":"mane","model":"llama3.1:8b","weight":3},{"machine":"mane","model":"qwen2.5:14b","weight":2}]}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(bad))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
	list, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches", nil)
	list.Host = "localhost"
	lr, _ := http.DefaultClient.Do(list)
	var batches []any
	_ = decodeJSON(lr, &batches)
	if len(batches) != 0 {
		t.Fatalf("expected no batch row after failed submit")
	}
}

func TestBatchCancel(t *testing.T) {
	slow := []fakeollama.Step{{Wait: 2 * time.Second}, {Done: &fakeollama.DoneStats{}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = slow
	fS.ChatSteps = slow
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	body := `{"items":[{"prompt":"one"},{"prompt":"two"}],"split":{"assign":"round_robin","targets":[{"machine":"mane","model":"llama3.1:8b"},{"machine":"firmino","model":"qwen2.5:14b"}]}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created struct {
		ID string `json:"id"`
	}
	_ = decodeJSON(res, &created)

	cancel, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches/"+created.ID+"/cancel", nil)
	cancel.Host = "localhost"
	cr, _ := http.DefaultClient.Do(cancel)
	var out struct {
		Cancelled int `json:"cancelled"`
	}
	_ = decodeJSON(cr, &out)
	if out.Cancelled < 1 {
		t.Fatalf("cancelled %d", out.Cancelled)
	}
	cancel2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches/"+created.ID+"/cancel", nil)
	cancel2.Host = "localhost"
	cr2, _ := http.DefaultClient.Do(cancel2)
	var out2 struct{ Cancelled int }
	_ = decodeJSON(cr2, &out2)
	if out2.Cancelled != 0 {
		t.Fatalf("second cancel %d", out2.Cancelled)
	}
}

func newHarnessFromURLs(t *testing.T, maneURL, firminoURL string) *httptest.Server {
	t.Helper()
	return newHarnessJobs(t, firminoURL, maneURL)
}
