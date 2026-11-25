//go:build linux

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

// V7.3: expanding 200 template rows should add little RSS (rows expanded one at a time).
func TestTemplateBatchRSSGate(t *testing.T) {
	done := []fakeollama.Step{{Content: "x"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	_, _ = http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/templates",
		`{"name":"bulk","body":"{{n}}"}`))

	before, err := rssKB()
	if err != nil {
		t.Skip("no /proc")
	}
	var rows []string
	for i := 0; i < 200; i++ {
		rows = append(rows, fmt.Sprintf(`{"n":"r%d"}`, i))
	}
	body := fmt.Sprintf(`{"template":"bulk","rows":[%s]}`, strings.Join(rows, ","))
	res, _ := http.DefaultClient.Do(mustReq(t, http.MethodPost, ts.URL+"/api/batches", body))
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
	after, _ := rssKB()
	deltaMB := float64(after-before) / 1024
	t.Logf("RSS growth after 200-row template batch: %.1f MB", deltaMB)
	if deltaMB > 5 {
		t.Fatalf("RSS grew %.1f MB (gate 5 MB)", deltaMB)
	}
}
