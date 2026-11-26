//go:build linux

package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/api"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
)

// stopBatchBeforeClose cancels a batch and waits until it is idle, so the
// next test does not share the CPU with hundreds of leftover job goroutines.
func stopBatchBeforeClose(ts *httptest.Server, batchID string) {
	if batchID != "" && ts != nil {
		c, err := http.NewRequest(http.MethodPost, ts.URL+"/api/batches/"+batchID+"/cancel", nil)
		if err == nil {
			c.Host = "localhost"
			if resp, doErr := http.DefaultClient.Do(c); doErr == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			g, err := http.NewRequest(http.MethodGet, ts.URL+"/api/batches/"+batchID, nil)
			if err != nil {
				break
			}
			g.Host = "localhost"
			gr, err := http.DefaultClient.Do(g)
			if err != nil {
				time.Sleep(30 * time.Millisecond)
				continue
			}
			var view map[string]any
			_ = decodeJSON(gr, &view)
			if view["pending"] == float64(0) {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	time.Sleep(100 * time.Millisecond)
}

func rssKB() (int, error) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	re := regexp.MustCompile(`VmRSS:\s+(\d+)`)
	m := re.FindSubmatch(b)
	if len(m) < 2 {
		return 0, nil
	}
	return strconv.Atoi(string(m[1]))
}

// M9a proxy: router stack should stay well under the 50 MB budget at rest in-process.
func TestRSSProxyBudget(t *testing.T) {
	before, err := rssKB()
	if err != nil {
		t.Skip("no /proc")
	}
	fs := &fakeollama.Server{Models: []string{"m:8b"}}
	ts := fakeollama.NewHTTPTestServer(fs)
	defer ts.Close()
	cfg := testConfig(t, ts.URL, ts.URL)
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	_ = api.NewServer(cfg, reg, g, nil)
	after, _ := rssKB()
	delta := after - before
	// Generous ceiling for test binary overhead; owner re-checks on Mane.
	if delta > 40*1024 {
		t.Fatalf("rss grew %d KB", delta)
	}
	_ = strings.Join(cfg.Machines[0].Roles, "")
}
