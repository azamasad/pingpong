package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestStressFinishesJobsOnMane(t *testing.T) {
	fast := &fakeollama.Server{
		Models: []string{"llama3.1:8b"},
		ChatSteps: []fakeollama.Step{
			{Content: "ok"},
			{Done: &fakeollama.DoneStats{EvalCount: 4, PromptEvalCount: 8}},
		},
	}
	heavy := &fakeollama.Server{Models: []string{"qwen2.5:14b"}}
	fTs := fakeollama.NewHTTPTestServer(fast)
	hTs := fakeollama.NewHTTPTestServer(heavy)
	defer fTs.Close()
	defer hTs.Close()
	ts := newHarnessJobs(t, hTs.URL, fTs.URL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/stress", strings.NewReader(`{"target":"mane","seconds":2}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("start %d", res.StatusCode)
	}

	deadline := time.Now().Add(8 * time.Second)
	var done float64
	for time.Now().Before(deadline) {
		greq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/stress", nil)
		greq.Host = "localhost"
		gres, err := http.DefaultClient.Do(greq)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(gres.Body).Decode(&body)
		gres.Body.Close()
		if machines, ok := body["machines"].(map[string]any); ok {
			if mane, ok := machines["mane"].(map[string]any); ok {
				done, _ = mane["done"].(float64)
			}
		}
		if done >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if done < 1 {
		t.Fatalf("expected a finished mane job, got %v", done)
	}

	sreq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/stress", nil)
	sreq.Host = "localhost"
	sres, err := http.DefaultClient.Do(sreq)
	if err != nil {
		t.Fatal(err)
	}
	sres.Body.Close()

	deadline = time.Now().Add(3 * time.Second)
	var running bool
	for time.Now().Before(deadline) {
		greq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/stress", nil)
		greq.Host = "localhost"
		gres, err := http.DefaultClient.Do(greq)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(gres.Body).Decode(&body)
		gres.Body.Close()
		running, _ = body["running"].(bool)
		if !running {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if running {
		t.Fatal("still running after stop")
	}
}
