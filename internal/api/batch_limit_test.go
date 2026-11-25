package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/fakeollama"
)

func TestBatchMaxItemsRejected(t *testing.T) {
	mS, fS, mURL, fURL := fakeollama.Pair(t, []string{"llama3.1:8b"}, []string{"qwen2.5:14b"})
	mS.ChatSteps = []fakeollama.Step{{Done: &fakeollama.DoneStats{}}}
	fS.ChatSteps = mS.ChatSteps
	ts := newHarnessFromURLs(t, mURL, fURL)
	defer ts.Close()

	var items []string
	for i := 0; i < 501; i++ {
		items = append(items, fmt.Sprintf(`{"prompt":"x%d"}`, i))
	}
	body := `{"items":[` + strings.Join(items, ",") + `]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/batches", strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}