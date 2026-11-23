package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/azamasad/pingpong/internal/doctor"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	out := map[string]any{
		"version":         Version,
		"uptime_seconds":  time.Since(s.startedAt).Seconds(),
		"memory_bytes":    ms.Sys,
		"machines":        s.Reg.ListMachines(),
		"jobs":            nil,
		"recent_failures": nil,
		"storage":         nil,
	}
	if s.Jobs != nil {
		st := s.Jobs.Store()
		ctx := r.Context()
		counts, _ := st.JobCounts(ctx)
		lanes, _ := st.LaneCounts(ctx)
		oldest, _ := st.OldestWaitingSeconds(ctx)
		jobs := map[string]any{
			"by_status": counts,
			"by_lane":   lanes,
		}
		if oldest != nil {
			jobs["oldest_waiting_seconds"] = *oldest
		}
		out["jobs"] = jobs
		failures, _ := st.RecentFailures(ctx, 20)
		out["recent_failures"] = failures
		freeFn := doctor.FreeBytesOnFS
		free, _ := freeFn(st.Dir())
		size, _ := st.DBSizeBytes()
		out["storage"] = map[string]any{
			"free_bytes":     free,
			"database_bytes": size,
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}
