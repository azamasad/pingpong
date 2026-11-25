package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func testScheduleSvc(t *testing.T, now time.Time) (*Service, *store.Store) {
	t.Helper()
	cfg := &config.Config{
		Machines: []config.Machine{
			{Name: "mane", Address: "http://127.0.0.1:1", DefaultModel: "m1", Roles: []string{"fast"}},
			{Name: "firmino", Address: "http://127.0.0.1:2", DefaultModel: "m2", Roles: []string{"heavy"}},
		},
		Routing:   config.RoutingConfig{AutoRole: "heavy", FastRole: "fast"},
		Storage:   config.StorageConfig{DataDir: t.TempDir()},
		Batches:   config.BatchesConfig{MaxItems: 100},
		Schedules: config.SchedulesConfig{MaxCount: 50, Grace: config.Duration{Duration: time.Hour}},
		Retry:     config.RetryConfig{RetryInterval: config.Duration{Duration: time.Minute}, RetryWindow: config.Duration{Duration: 3 * time.Hour}},
		Timeouts: config.TimeoutConfig{
			ConnectTimeout: config.Duration{Duration: 5 * time.Second},
		},
		Health: config.HealthConfig{
			Interval: config.Duration{Duration: time.Second}, Timeout: config.Duration{Duration: time.Second},
			FailuresToMarkDown: 1,
		},
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := gate.New(map[string]int{"mane": 1, "firmino": 1})
	reg := registry.New(cfg, g)
	svc := New(cfg, st, reg, g, clock.Fake{NowTime: now})
	return svc, st
}

func TestScheduleRunNowNoLastDue(t *testing.T) {
	now := time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC)
	svc, st := testScheduleSvc(t, now)
	ctx := context.Background()
	_ = st.InsertTemplate(ctx, &store.Template{
		Name: "t", Body: "hello", CreatedAt: now, UpdatedAt: now,
	})
	sch := &store.Schedule{
		Name: "s1", RuleJSON: `{"every":"day","at":"02:00"}`,
		Template: "t", Machine: "firmino", Model: "m2",
		Missed: "skip", Overlap: "skip", Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = st.InsertSchedule(ctx, sch)
	run, err := svc.Scheduler().RunNow(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" && run.Status != "submitted" {
		t.Fatalf("status %s", run.Status)
	}
	got, _ := st.GetSchedule(ctx, sch.ID)
	if got.LastDue != nil {
		t.Fatal("run now must not set last_due")
	}
}

func TestScheduleMissedSkipOnStartup(t *testing.T) {
	loc := time.Local
	now := time.Date(2025, 6, 11, 10, 0, 0, 0, loc)
	svc, st := testScheduleSvc(t, now)
	ctx := context.Background()
	last := time.Date(2025, 6, 10, 2, 0, 0, 0, loc)
	sch := &store.Schedule{
		Name: "missed", RuleJSON: `{"every":"day","at":"02:00"}`,
		Prompt: "hi", Machine: "firmino", Model: "m2",
		Missed: "skip", Overlap: "skip", Enabled: true,
		LastDue:   &last,
		CreatedAt: last, UpdatedAt: last,
	}
	_ = st.InsertSchedule(ctx, sch)
	sc := NewScheduler(svc)
	sc.handleStartupMissed(ctx)
	runs, _ := st.ListScheduleRuns(ctx, sch.ID, 10)
	if len(runs) != 1 || runs[0].Status != "skipped_missed" {
		t.Fatalf("runs %+v", runs)
	}
}
