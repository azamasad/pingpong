package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

func TestListJobsBefore(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"old", "mid", "new"} {
		ts := base.Add(time.Duration(i) * time.Hour)
		_ = st.InsertJob(ctx, &store.Job{
			ID: id, CreatedAt: ts, UpdatedAt: ts, Status: store.StatusDone,
			RequestJSON: `{}`,
		})
	}
	before := base.Add(2 * time.Hour)
	list, err := st.ListJobs(ctx, "", "", "", &before, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "mid" || list[1].ID != "old" {
		t.Fatalf("%v", list)
	}
}
