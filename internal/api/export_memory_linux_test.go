//go:build linux

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

// V7.4: exporting a large batch should add little RSS (one row at a time).
func TestExportLargeBatchRSSGate(t *testing.T) {
	done := []fakeollama.Step{{Content: strings.Repeat("x", 2048)}, {Done: &fakeollama.DoneStats{EvalCount: 50}}}
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
	const n = 80
	for i := 0; i < n; i++ {
		items = append(items, fmt.Sprintf(`{"prompt":"item %d"}`, i))
	}
	body := `{"items":[` + strings.Join(items, ",") + `]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	var created struct{ ID string `json:"id"` }
	_ = decodeJSON(res, &created)
	waitBatchDone(t, ts, created.ID)

	mid, _ := rssKB()
	exp, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+created.ID+"/export?format=csv", nil)
	exp.Host = "localhost"
	_, _ = http.DefaultClient.Do(exp)
	after, _ := rssKB()
	_ = before
	deltaMB := float64(after-mid) / 1024
	t.Logf("RSS growth after %d-row export: %.1f MB", n, deltaMB)
	if deltaMB > 5 {
		t.Fatalf("export RSS growth %.1f MB", deltaMB)
	}
}