package api_test

import (
	"bufio"
	"context"
	"encoding/json"
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

func newHarnessJobs(t *testing.T, heavyURL, fastURL string) *httptest.Server {
	cfg := testConfig(t, heavyURL, fastURL)
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

func TestE2EJobSubmitAndComplete(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "job-ok"},
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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
	var created map[string]string
	_ = json.NewDecoder(res.Body).Decode(&created)
	id := created["id"]
	if id == "" {
		t.Fatal("empty id")
	}

	deadline := time.Now().Add(10 * time.Second)
	var done bool
	for time.Now().Before(deadline) {
		greq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id, nil)
		greq.Host = "localhost"
		gres, err := http.DefaultClient.Do(greq)
		if err != nil {
			t.Fatal(err)
		}
		var detail map[string]any
		_ = json.NewDecoder(gres.Body).Decode(&detail)
		_ = gres.Body.Close()
		if detail["status"] == "done" && detail["output_text"] == "job-ok" {
			done = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !done {
		t.Fatal("job did not complete")
	}

	evReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/jobs/"+id+"/events", nil)
	evReq.Host = "localhost"
	evRes, err := http.DefaultClient.Do(evReq)
	if err != nil {
		t.Fatal(err)
	}
	defer evRes.Body.Close()
	var sawToken bool
	sc := bufio.NewScanner(evRes.Body)
	for sc.Scan() {
		var ev map[string]any
		_ = json.Unmarshal(sc.Bytes(), &ev)
		if ev["type"] == "token" && ev["text"] == "job-ok" {
			sawToken = true
		}
	}
	if !sawToken {
		t.Fatal("expected token event")
	}
}
