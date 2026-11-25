package notify_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/notify"
)

func testCfg(url string) config.NotifyConfig {
	return config.NotifyConfig{
		WebhookURL: url,
		Events:     []string{notify.EventBatchDone, notify.EventScheduleFailed, notify.EventJobFailed},
		Timeout:    config.Duration{Duration: 2 * time.Second},
	}
}

func TestPostHeadersAndBody(t *testing.T) {
	var gotEvent, gotTitle, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEvent = r.Header.Get("X-Pingpong-Event")
		gotTitle = r.Header.Get("Title")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := notify.New(testCfg(ts.URL))
	if err := c.PostSync(notify.EventJobFailed, "Job abcdef12 failed (TIMEOUT)"); err != nil {
		t.Fatal(err)
	}
	if gotEvent != notify.EventJobFailed || gotTitle != "Pingpong" {
		t.Fatalf("headers event=%q title=%q", gotEvent, gotTitle)
	}
	if gotBody != "Job abcdef12 failed (TIMEOUT)" {
		t.Fatalf("body %q", gotBody)
	}
}

func TestEmptyURLNoSend(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer ts.Close()
	c := notify.New(config.NotifyConfig{WebhookURL: ""})
	if err := c.PostSync(notify.EventJobFailed, "x"); err != nil || called {
		t.Fatalf("expected no call, called=%v", called)
	}
}

func TestFailureLogsWithoutURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	var buf strings.Builder
	restore := logx.SetOutput(&buf)
	defer restore()
	c := notify.New(testCfg(ts.URL))
	c.Send(notify.EventBatchDone, "Batch x finished: 1 done, 0 failed")
	time.Sleep(200 * time.Millisecond)
	out := buf.String()
	if !strings.Contains(out, "notify_failed") {
		t.Fatalf("expected notify_failed log: %s", out)
	}
	if strings.Contains(out, ts.URL) {
		t.Fatalf("url leaked in log: %s", out)
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/", http.StatusFound)
	}))
	defer ts.Close()
	c := notify.New(testCfg(ts.URL))
	if err := c.PostSync(notify.EventBatchDone, "x"); err == nil {
		t.Fatal("expected redirect error")
	}
}

func TestValidWebhookURL(t *testing.T) {
	if !notify.ValidWebhookURL("https://ntfy.sh/my-topic") || notify.ValidWebhookURL("ftp://x") {
		t.Fatal("url validation")
	}
	if !notify.ValidWebhookURL("") {
		t.Fatal("empty should be valid (off)")
	}
}
