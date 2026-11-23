package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

func TestJobRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	j := &store.Job{
		ID: "a", CreatedAt: now, UpdatedAt: now, Status: store.StatusWaiting,
		RequestJSON: `{"prompt":"x"}`, Machine: "mane", Model: "m", Mode: "fast",
	}
	if err := st.InsertJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetJob(ctx, "a")
	if err != nil || got.Model != "m" {
		t.Fatalf("%v %v", got, err)
	}
	seq, err := st.NextEventSeq(ctx, "a")
	if err != nil || seq != 1 {
		t.Fatalf("seq %d %v", seq, err)
	}
	_ = st.AppendEvent(ctx, "a", 1, "token", map[string]string{"text": "hi"})
	evs, err := st.EventsSince(ctx, "a", 1)
	if err != nil || len(evs) != 1 {
		t.Fatalf("%v %v", evs, err)
	}
}
