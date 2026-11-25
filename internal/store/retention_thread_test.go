package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

func TestThreadJobRetentionExempt(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	recent := time.Now().UTC()
	th := &store.Thread{
		ID: "th1", Machine: "firmino", Model: "m",
		PinChangesJSON: "[]", CreatedAt: recent, UpdatedAt: recent,
	}
	if err := st.InsertThread(ctx, th); err != nil {
		t.Fatal(err)
	}
	_ = st.InsertJob(ctx, &store.Job{
		ID: "turn1", CreatedAt: old, UpdatedAt: old, Status: store.StatusDone,
		RequestJSON: `{}`, ThreadID: "th1", Turn: 1,
	})
	cut := time.Now().UTC().Add(-90 * 24 * time.Hour)
	n, err := st.PurgeOlderThan(ctx, cut)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("swept thread job n=%d", n)
	}
	if _, err := st.GetJob(ctx, "turn1"); err != nil {
		t.Fatal("turn job swept")
	}
}

func TestThreadPurgeAfterLastMessage(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	th := &store.Thread{
		ID: "th2", Machine: "firmino", Model: "m",
		PinChangesJSON: "[]", CreatedAt: old, UpdatedAt: old,
	}
	if err := st.InsertThread(ctx, th); err != nil {
		t.Fatal(err)
	}
	_ = st.InsertJob(ctx, &store.Job{
		ID: "turn2", CreatedAt: old, UpdatedAt: old, Status: store.StatusDone,
		RequestJSON: `{}`, ThreadID: "th2", Turn: 1,
	})
	cut := time.Now().UTC().Add(-90 * 24 * time.Hour)
	n, err := st.PurgeOldThreads(ctx, cut)
	if err != nil || n != 1 {
		t.Fatalf("purge n=%d err=%v", n, err)
	}
	if _, err := st.GetThread(ctx, "th2"); err == nil {
		t.Fatal("thread should be gone")
	}
	if _, err := st.GetJob(ctx, "turn2"); err == nil {
		t.Fatal("turn job should be gone with thread")
	}
}

func TestLiveThreadKeepsOldTurnJobs(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	oldTurn := time.Now().UTC().Add(-100 * 24 * time.Hour)
	recent := time.Now().UTC()
	th := &store.Thread{
		ID: "th3", Machine: "firmino", Model: "m",
		PinChangesJSON: "[]", CreatedAt: recent, UpdatedAt: recent,
	}
	if err := st.InsertThread(ctx, th); err != nil {
		t.Fatal(err)
	}
	_ = st.InsertJob(ctx, &store.Job{
		ID: "oldturn", CreatedAt: oldTurn, UpdatedAt: oldTurn, Status: store.StatusDone,
		RequestJSON: `{}`, ThreadID: "th3", Turn: 1,
	})
	cut := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if _, err := st.PurgeOlderThan(ctx, cut); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetJob(ctx, "oldturn"); err != nil {
		t.Fatal("old turn swept from live thread")
	}
	if _, err := st.PurgeOldThreads(ctx, cut); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetThread(ctx, "th3"); err != nil {
		t.Fatal("live thread purged too early")
	}
}
