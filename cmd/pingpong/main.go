package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/azamasad/pingpong/internal/config"
)

func main() {
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := cfg.Router.Listen
	log.Printf("pingpong listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
