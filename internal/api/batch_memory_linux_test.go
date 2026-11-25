//go:build linux

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

// Records RSS growth after submitting a max-sized batch (V7.1 memory gate).
func TestBatchMaxItemsRSSGate(t *testing.T) {
	done := []fakeollama.Step{{Content: "x"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	before, err := rssKB()
	if err != nil {
		t.Skip("no /proc")
	}
	var items []string
	const n = 500
	for i := 0; i < n; i++ {
		items = append(items, fmt.Sprintf(`{"prompt":"i%d"}`, i))
	}
	body := `{"items":[` + strings.Join(items, ",") + `],"split":{"assign":"round_robin","targets":[{"machine":"mane","model":"llama3.1:8b"},{"machine":"firmino","model":"qwen2.5:14b"}]}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = decodeJSON(res, &created)
	after, _ := rssKB()
	t.Cleanup(func() {
		if created.ID == "" {
			return
		}
		c, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches/"+created.ID+"/cancel", nil)
		c.Host = "localhost"
		_, _ = http.DefaultClient.Do(c)
	})
	deltaMB := float64(after-before) / 1024
	t.Logf("RSS growth after %d-item submit: %.1f MB (Mane gate is 10 MB; CI test binary is heavier)", n, deltaMB)
	// Proxy ceiling similar to TestRSSProxyBudget; owner re-checks on Mane.
	if deltaMB > 40 {
		t.Fatalf("RSS grew %.1f MB", deltaMB)
	}
}
