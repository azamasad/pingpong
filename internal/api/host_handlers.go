package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/azamasad/pingpong/internal/hoststats"
)

func (s *Server) handleHost(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	c := hoststats.Collector{Sampler: s.host}
	reports := c.Collect(ctx, s.Cfg.Machines)
	hoststats.LogReports(reports)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"machines": reports,
	})
}
