//go:build linux

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

// V7.2 memory gate: 200 checked batch items queued.
func TestCheckMaxItemsRSSGate(t *testing.T) {
	done := []fakeollama.Step{{Content: "x"}, {Done: &fakeollama.DoneStats{EvalCount: 1}}}
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = done
	fS.ChatSteps = done
	ts := newHarnessFromURLs(t, mURL, fURL)
	t.Cleanup(func() { ts.Close() })

	before, err := rssKB()
	if err != nil {
		t.Skip("no /proc")
	}
	var items []string
	const n = 200
	for i := 0; i < n; i++ {
		items = append(items, fmt.Sprintf(`{"prompt":"c%d","lane":"batch","check":{"mode":"review"}}`, i))
	}
	body := `{"items":[` + strings.Join(items, ",") + `]}`
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
	t.Cleanup(func() { stopBatchBeforeClose(ts, created.ID) })
	after, _ := rssKB()
	delta := after - before
	if delta > 5120 {
		t.Fatalf("rss grew %d KB after %d checked items", delta, n)
	}
}
