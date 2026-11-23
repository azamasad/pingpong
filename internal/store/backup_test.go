package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/azamasad/pingpong/internal/store"
)

func TestBackupTo(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	j := &store.Job{
		ID: "j1", Status: store.StatusDone, RequestJSON: `{"prompt":"x"}`,
		Lane: store.LaneInteractive,
	}
	if err := st.InsertJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	st.Close()
	dest := filepath.Join(t.TempDir(), "snap.db")
	if err := store.BackupTo(dir, dest); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dest)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", fi.Mode())
	}
}
