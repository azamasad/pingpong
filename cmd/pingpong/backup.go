package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/store"
)

func runBackup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to YAML config (or set PINGPONG_CONFIG)")
	to := fs.String("to", "", "destination directory for the backup file")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *to == "" {
		fmt.Fprintln(os.Stderr, "backup: --to DIR is required")
		return 2
	}
	path, err := config.PathFromEnvOrFlag(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 2
	}
	cfg, _, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: config: %v\n", err)
		return 2
	}
	if !cfg.JobsEnabled() {
		fmt.Fprintln(os.Stderr, "backup: storage.data_dir is not configured")
		return 2
	}
	name := fmt.Sprintf("pingpong-%s.db", time.Now().UTC().Format("20060102-150405"))
	dest := filepath.Join(*to, name)
	if err := store.BackupTo(cfg.Storage.DataDir, dest); err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}
	fi, _ := os.Stat(dest)
	fmt.Println(dest)
	if fi != nil {
		fmt.Fprintf(os.Stderr, "backup: wrote %d bytes\n", fi.Size())
	}
	return 0
}
