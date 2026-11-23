package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/azamasad/pingpong/internal/api"
	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "batch":
			os.Exit(runBatch(os.Args[2:]))
		case "doctor":
			os.Exit(runDoctor(os.Args[2:]))
		}
	}
	configPath := flag.String("config", "", "path to YAML config (or set PINGPONG_CONFIG)")
	flag.Parse()

	path, err := config.PathFromEnvOrFlag(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pingpong: %v\n", err)
		os.Exit(2)
	}

	cfg, warnings, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pingpong: config: %v\n", err)
		os.Exit(1)
	}
	for _, w := range warnings {
		log.Printf("config warning: %s", w)
	}

	limits := make(map[string]int)
	for _, m := range cfg.Machines {
		limits[m.Name] = m.MaxConcurrent
	}
	g := gate.New(limits)
	reg := registry.New(cfg, g)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	var jobSvc *jobs.Service
	if cfg.JobsEnabled() {
		st, err := store.Open(cfg.Storage.DataDir)
		if err != nil {
			log.Fatalf("store: %v", err)
		}
		defer st.Close()
		jobSvc = jobs.New(cfg, st, reg, g, clock.Real{})
		jobSvc.Start(ctx)
	}

	srv := api.NewServer(cfg, reg, g, jobSvc)
	httpSrv := &http.Server{
		Addr:    cfg.Router.Listen,
		Handler: srv.Handler(),
	}

	go func() {
		log.Printf("pingpong %s listening on %s", api.Version, cfg.Router.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	cancel()
	_ = httpSrv.Shutdown(context.Background())
	reg.Stop()
}
