package hoststats

import (
	"encoding/json"
	"net/http"
)

// NewHandler serves one JSON sample at GET /.
func NewHandler(s *Sampler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Sample())
	})
	return mux
}
