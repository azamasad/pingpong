package jobs

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/notify"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func testNotifyService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "m", Address: "http://127.0.0.1:9", Roles: []string{"fast"},
			DefaultModel: "x:1b", MaxConcurrent: 1, NumCtx: 8192,
		}},
		Routing:  config.RoutingConfig{AutoRole: "fast", FastRole: "fast"},
		Profiles: map[string]config.Profile{"fast": {Fast: true}},
		Storage:  config.StorageConfig{DataDir: dir},
		Notify: config.NotifyConfig{
			WebhookURL: "http://127.0.0.1:9/hook",
			Events:     []string{notify.EventBatchDone, notify.EventJobFailed, notify.EventScheduleFailed},
			Timeout:    config.Duration{Duration: time.Second},
		},
		Retry: config.RetryConfig{
			RetryInterval: config.Duration{time.Millisecond},
			RetryWindow:   config.Duration{time.Millisecond},
		},
	}
	g := gate.New(map[string]int{"m": 1})
	reg := registry.New(cfg, g)
	return New(cfg, st, reg, g, clock.Real{}), st
}

func TestNotifyBatchDoneOnce(t *testing.T) {
	svc, st := testNotifyService(t)
	var bodies []string
	var mu sync.Mutex
	svc.notify.SetTestPost(func(event, body string) error {
		mu.Lock()
		bodies = append(bodies, event+":"+body)
		mu.Unlock()
		return nil
	})
	ctx := context.Background()
	now := time.Now().UTC()
	batchID := "batch-1111-aaaa-bbbb-ccccdddd"
	_ = st.InsertBatch(ctx, &store.Batch{ID: batchID, CreatedAt: now, Kind: "plain", ItemCount: 2})
	for _, id := range []string{"job-a111", "job-b222"} {
		_ = st.InsertJob(ctx, &store.Job{
			ID: id, CreatedAt: now, UpdatedAt: now, Status: store.StatusDone,
			BatchID: batchID, Lane: store.LaneBatch, RequestJSON: `{"prompt":"hi"}`,
			Machine: "m", Model: "x:1b",
		})
	}
	j, _ := st.GetJob(ctx, "job-a111")
	svc.onBatchItemTerminal(ctx, j)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 || !strings.Contains(bodies[0], notify.EventBatchDone) {
		t.Fatalf("bodies %v", bodies)
	}
	if !strings.Contains(bodies[0], "batch-11") || !strings.Contains(bodies[0], "2 done") {
		t.Fatalf("body shape %q", bodies[0])
	}
	if strings.Contains(bodies[0], "hi") {
		t.Fatal("body leaked prompt")
	}
	svc.onBatchItemTerminal(ctx, j)
	time.Sleep(50 * time.Millisecond)
	if len(bodies) != 1 {
		t.Fatalf("resent: %v", bodies)
	}
}

func TestNotifyJobFailedSkipsPlainBatchItem(t *testing.T) {
	svc, _ := testNotifyService(t)
	called := false
	svc.notify.SetTestPost(func(event, body string) error {
		called = true
		return nil
	})
	j := &store.Job{ID: "x", BatchID: "b", Lane: store.LaneBatch, ErrorCode: "TIMEOUT"}
	svc.maybeNotifyJobFailed(j, "TIMEOUT")
	if called {
		t.Fatal("batch item should not notify job_failed")
	}
	j.Stage = store.StageReduce
	svc.maybeNotifyJobFailed(j, "TIMEOUT")
	time.Sleep(50 * time.Millisecond)
	if !called {
		t.Fatal("stage reduce should notify")
	}
}
