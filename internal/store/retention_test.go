package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

func TestPurgeRetention(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	new := time.Now().UTC()
	_ = st.InsertJob(ctx, &store.Job{ID: "old", CreatedAt: old, UpdatedAt: old, Status: store.StatusDone, RequestJSON: `{}`})
	_ = st.InsertJob(ctx, &store.Job{ID: "new", CreatedAt: new, UpdatedAt: new, Status: store.StatusDone, RequestJSON: `{}`})
	n, err := st.PurgeOlderThan(ctx, time.Now().UTC().Add(-90*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, err = st.GetJob(ctx, "old")
	if err == nil {
		t.Fatal("old should be gone")
	}
	if _, err := st.GetJob(ctx, "new"); err != nil {
		t.Fatal(err)
	}
}
