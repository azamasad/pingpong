package api_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestJobDoneEventCarriesStats(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "hello"},
			{Done: &fakeollama.DoneStats{EvalCount: 7, PromptEvalCount: 3}},
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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]string
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()

	var tokens float64
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && tokens == 0 {
		ereq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+created["id"]+"/events", nil)
		ereq.Host = "localhost"
		eres, err := http.DefaultClient.Do(ereq)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(eres.Body)
		for sc.Scan() {
			var ev map[string]any
			if json.Unmarshal(sc.Bytes(), &ev) != nil || ev["type"] != "done" {
				continue
			}
			if stats, ok := ev["stats"].(map[string]any); ok {
				tokens, _ = stats["output_tokens"].(float64)
			}
		}
		eres.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}
	if tokens != 7 {
		t.Fatalf("expected output_tokens 7 in the done event, got %v", tokens)
	}
}
