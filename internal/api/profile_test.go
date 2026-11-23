package api_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestProfileFastRoutesToMane(t *testing.T) {
	heavy := &fakeollama.Server{
		Models: []string{"qwen2.5:14b"},
		ChatSteps: []fakeollama.Step{
			{Content: "heavy"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	fast := &fakeollama.Server{
		Models: []string{"llama3.1:8b"},
		ChatSteps: []fakeollama.Step{
			{Content: "quick"},
			{Done: &fakeollama.DoneStats{EvalCount: 1}},
		},
	}
	hTs := fakeollama.NewHTTPTestServer(heavy)
	fTs := fakeollama.NewHTTPTestServer(fast)
	defer hTs.Close()
	defer fTs.Close()
	ts, _ := newHarness(t, hTs.URL, fTs.URL)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"hi","profile":"fast"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var machine, text string
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		var ev map[string]any
		_ = json.Unmarshal(sc.Bytes(), &ev)
		if ev["type"] == "route" {
			machine, _ = ev["machine"].(string)
		}
		if ev["type"] == "token" {
			text += ev["text"].(string)
		}
	}
	if machine != "mane" || text != "quick" {
		t.Fatalf("machine %q text %q", machine, text)
	}
}

func TestUnknownProfile(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"prompt":"hi","profile":"nope"}`))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestListProfiles(t *testing.T) {
	h, f := fakePair(t)
	ts, _ := newHarness(t, h, f)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/profiles", nil)
	req.Host = "localhost"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) < 3 {
		t.Fatalf("%v", rows)
	}
}
