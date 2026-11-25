package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestBatchSubmitAndFetch(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "item"},
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

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(`{"items":[{"prompt":"one"},{"prompt":"two"}]}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID     string   `json:"id"`
		JobIDs []string `json:"job_ids"`
	}
	if err := decodeJSON(res, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.JobIDs) != 2 {
		t.Fatalf("%+v", created)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID, nil)
		g.Host = "localhost"
		gr, err := http.DefaultClient.Do(g)
		if err != nil {
			t.Fatal(err)
		}
		var view map[string]any
		_ = decodeJSON(gr, &view)
		if view["pending"] == float64(0) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("batch did not finish")
}
