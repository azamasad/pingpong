package jobs

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/notify"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func TestCancelBatchDropsPendingCheckAndReduce(t *testing.T) {
	svc, st := testNotifyService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	batchID := "batch-cancel-1"
	if err := st.InsertBatch(ctx, &store.Batch{
		ID: batchID, CreatedAt: now, Kind: "file", ItemCount: 1,
		ReduceJSON: `{"instruction":"sum {{results}}","machine":"m","model":"x:1b"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertJob(ctx, &store.Job{
		ID: "parent-1", CreatedAt: now, UpdatedAt: now, Status: store.StatusDone,
		BatchID: batchID, Lane: store.LaneBatch, Stage: store.StageAnswer, CheckPending: 1,
		RequestJSON: `{"prompt":"q","check":{"mode":"review"}}`,
		OutputText:  "a", Machine: "m", Model: "x:1b",
	}); err != nil {
		t.Fatal(err)
	}
	n, err := svc.CancelBatch(ctx, batchID)
	if err != nil || n != 0 {
		t.Fatalf("cancel n=%d err=%v", n, err)
	}
	j, err := st.GetJob(ctx, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != store.StatusDone || j.CheckPending != 0 {
		t.Fatalf("parent status=%s check_pending=%d", j.Status, j.CheckPending)
	}
	b, err := st.GetBatch(ctx, batchID)
	if err != nil || b.ReduceJSON != "" {
		t.Fatalf("reduce json %q err=%v", b.ReduceJSON, err)
	}
	svc.recoverPendingChecks(ctx)
	if err := svc.maybeCreateReduce(ctx, batchID); err != nil {
		t.Fatal(err)
	}
	kids, err := st.ChildrenOf(ctx, "parent-1")
	if err != nil || len(kids) != 0 {
		t.Fatalf("stage after cancel: %d %v", len(kids), err)
	}
	if _, err := st.ReduceJobByBatch(ctx, batchID); err == nil {
		t.Fatal("reduce job created after cancel")
	}
}

func TestScheduleJobHonorsTemplateCheck(t *testing.T) {
	ctx := context.Background()
	fake := &fakeollama.Server{Models: []string{"m1"}}
	fts := fakeollama.NewHTTPTestServer(fake)
	defer fts.Close()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := &config.Config{
		Machines: []config.Machine{
			{Name: "mane", Address: fts.URL, DefaultModel: "m1", Roles: []string{"fast"}, NumCtx: 8192, MaxConcurrent: 1},
			{Name: "firmino", Address: "http://127.0.0.1:1", DefaultModel: "m2", Roles: []string{"heavy"}, NumCtx: 8192, MaxConcurrent: 1},
		},
		Routing:  config.RoutingConfig{AutoRole: "heavy", FastRole: "fast"},
		Storage:  config.StorageConfig{DataDir: dir},
		Batches:  config.BatchesConfig{MaxItems: 10},
		Timeouts: config.TimeoutConfig{ConnectTimeout: config.Duration{Duration: 200 * time.Millisecond}},
		Retry: config.RetryConfig{
			RetryInterval: config.Duration{Duration: time.Hour},
			RetryWindow:   config.Duration{Duration: time.Hour},
		},
	}
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(ctx, "mane")
	svc := New(cfg, st, reg, g, clock.Real{})
	body := `{"prompt":"hi","check":{"mode":"review","machine":"firmino","model":"missing:model"}}`
	if _, err := svc.SubmitScheduleJob(ctx, body, "mane", "m1"); err == nil {
		t.Fatal("bad check target must reject the schedule job")
	}
	jobs, err := st.ListJobs(ctx, "", "", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("saved %d jobs", len(jobs))
	}
}

func TestFailTerminalNotifiesBatchAndSchedule(t *testing.T) {
	svc, st := testNotifyService(t)
	var got []string
	var mu sync.Mutex
	svc.notify.SetTestPost(func(event, body string) error {
		mu.Lock()
		got = append(got, event+":"+body)
		mu.Unlock()
		return nil
	})
	ctx := context.Background()
	now := time.Now().UTC()
	batchID := "batch-ffff-1111"
	if err := st.InsertBatch(ctx, &store.Batch{ID: batchID, CreatedAt: now, Kind: "batch", ItemCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertJob(ctx, &store.Job{
		ID: "job-batch", CreatedAt: now, UpdatedAt: now, Status: store.StatusWaiting,
		BatchID: batchID, Lane: store.LaneBatch, RequestJSON: `{"prompt":"secret prompt"}`,
		Machine: "m", Model: "x:1b",
	}); err != nil {
		t.Fatal(err)
	}
	bj, err := st.GetJob(ctx, "job-batch")
	if err != nil {
		t.Fatal(err)
	}
	svc.failTerminal(ctx, bj, "MODEL_NOT_INSTALLED", "routing failed", now)

	if err := st.InsertSchedule(ctx, &store.Schedule{
		ID: "sch-1", Name: "nightly", RuleJSON: `{"every":"day","at":"02:00"}`,
		Prompt: "notes", Machine: "m", Model: "x:1b", Missed: "skip", Overlap: "skip",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertJob(ctx, &store.Job{
		ID: "job-sched", CreatedAt: now, UpdatedAt: now, Status: store.StatusWaiting,
		Lane: store.LaneBatch, RequestJSON: `{"prompt":"notes"}`,
		Machine: "m", Model: "x:1b",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertScheduleRun(ctx, &store.ScheduleRun{
		ScheduleID: "sch-1", Due: now, Started: now, Kind: "job", JobID: "job-sched", Status: "submitted",
	}); err != nil {
		t.Fatal(err)
	}
	sj, err := st.GetJob(ctx, "job-sched")
	if err != nil {
		t.Fatal(err)
	}
	svc.failTerminal(ctx, sj, "TIMEOUT", "gave up", now)
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	var batchDone, scheduleFailed, jobFailed int
	for _, line := range got {
		switch {
		case containsAll(line, notify.EventBatchDone, "0 done, 1 failed"):
			batchDone++
		case containsAll(line, notify.EventScheduleFailed, "nightly", "TIMEOUT"):
			scheduleFailed++
		case containsAll(line, notify.EventJobFailed, "TIMEOUT"):
			jobFailed++
		}
		if containsAll(line, "secret prompt") || containsAll(line, "notes") {
			t.Fatalf("content leaked: %s", line)
		}
	}
	if batchDone != 1 || scheduleFailed != 1 || jobFailed != 1 {
		t.Fatalf("events batch=%d schedule=%d job=%d all=%v", batchDone, scheduleFailed, jobFailed, got)
	}
}

func TestOversizeCheckNotifies(t *testing.T) {
	ctx := context.Background()
	fake := &fakeollama.Server{Models: []string{"x:1b"}}
	fts := fakeollama.NewHTTPTestServer(fake)
	defer fts.Close()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := &config.Config{
		Machines: []config.Machine{{
			Name: "m", Address: fts.URL, Roles: []string{"fast", "heavy"},
			DefaultModel: "x:1b", MaxConcurrent: 1, NumCtx: 128,
		}},
		Routing:  config.RoutingConfig{AutoRole: "heavy", FastRole: "fast"},
		Storage:  config.StorageConfig{DataDir: dir},
		Timeouts: config.TimeoutConfig{ConnectTimeout: config.Duration{Duration: time.Second}},
		Notify: config.NotifyConfig{
			WebhookURL: "http://127.0.0.1:9/hook",
			Events:     []string{notify.EventBatchDone, notify.EventJobFailed, notify.EventScheduleFailed},
			Timeout:    config.Duration{Duration: time.Second},
		},
		Retry: config.RetryConfig{
			RetryInterval: config.Duration{Duration: time.Hour},
			RetryWindow:   config.Duration{Duration: time.Hour},
		},
	}
	g := gate.New(map[string]int{"m": 1})
	reg := registry.New(cfg, g)
	reg.RefreshMachine(ctx, "m")
	svc := New(cfg, st, reg, g, clock.Real{})
	var got []string
	var mu sync.Mutex
	svc.notify.SetTestPost(func(event, body string) error {
		mu.Lock()
		got = append(got, event+" "+body)
		mu.Unlock()
		return nil
	})
	now := time.Now().UTC()
	batchID := "batch-oversize"
	if err := st.InsertBatch(ctx, &store.Batch{ID: batchID, CreatedAt: now, Kind: "batch", ItemCount: 1}); err != nil {
		t.Fatal(err)
	}
	parent := &store.Job{
		ID: "parent-big", CreatedAt: now, UpdatedAt: now, Status: store.StatusDone,
		BatchID: batchID, Lane: store.LaneBatch, Stage: store.StageAnswer, CheckPending: 1,
		RequestJSON: `{"prompt":"q","check":{"mode":"review","machine":"m","model":"x:1b"}}`,
		OutputText:  "answer", Machine: "m", Model: "x:1b",
	}
	if err := st.InsertJob(ctx, parent); err != nil {
		t.Fatal(err)
	}
	spec, err := parseCheck(parent.RequestJSON)
	if err != nil || spec == nil {
		t.Fatal(err)
	}
	if err := svc.createCheckStage(ctx, parent, spec, nil); err != nil {
		t.Fatal(err)
	}
	child, err := st.ChildByParentStage(ctx, parent.ID, "review")
	if err != nil || child.ErrorCode != "CONTEXT_TOO_LARGE" || child.Status != store.StatusFailed {
		t.Fatalf("child %+v err=%v", child, err)
	}
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	var jobFailed, batchDone bool
	for _, line := range got {
		if containsAll(line, notify.EventJobFailed, "CONTEXT_TOO_LARGE") {
			jobFailed = true
		}
		if containsAll(line, notify.EventBatchDone) {
			batchDone = true
		}
		if containsAll(line, "answer") || containsAll(line, "QUESTION") {
			t.Fatalf("content leaked: %s", line)
		}
	}
	if !jobFailed || !batchDone {
		t.Fatalf("events %v", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
