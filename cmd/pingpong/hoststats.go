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
	"time"

	"github.com/azamasad/pingpong/internal/hoststats"
)

func runHostStats(args []string) int {
	fs := flag.NewFlagSet("hoststats", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:8788", "address to serve hardware counters on")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := hoststats.CheckListen(*listen); err != nil {
		fmt.Fprintf(os.Stderr, "hoststats: %v\n", err)
		return 2
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           hoststats.NewHandler(hoststats.NewSampler()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("pingpong hoststats listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	return 0
}
