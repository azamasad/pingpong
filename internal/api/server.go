package api

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/registry"
)

//go:embed web/*
var webFS embed.FS

const Version = "7.0.0"

type Server struct {
	Cfg          *config.Config
	Reg          *registry.Registry
	Gate         *gate.Gate
	Jobs         *jobs.Service
	Mux          *http.ServeMux
	shuttingDown atomic.Bool
	startedAt    time.Time
	shutdown     context.CancelFunc
	once         sync.Once
}

func NewServer(cfg *config.Config, reg *registry.Registry, g *gate.Gate, jobSvc *jobs.Service) *Server {
	s := &Server{
		Cfg: cfg, Reg: reg, Gate: g, Jobs: jobSvc, Mux: http.NewServeMux(),
		startedAt: time.Now(),
	}
	s.routes()
	return s
}

// BeginShutdown refuses new mutating API requests until exit.
func (s *Server) BeginShutdown() { s.shuttingDown.Store(true) }

func (s *Server) routes() {
	static, _ := fs.Sub(webFS, "web")
	s.Mux.Handle("GET /", http.FileServer(http.FS(static)))
	s.Mux.HandleFunc("GET /api/health", s.handleHealth)
	s.Mux.HandleFunc("GET /api/machines", s.handleMachines)
	s.Mux.HandleFunc("GET /api/models", s.handleModels)
	s.Mux.HandleFunc("GET /api/profiles", s.handleProfiles)
	s.Mux.HandleFunc("GET /api/aliases", s.handleAliases)
	s.Mux.HandleFunc("GET /api/status", s.handleStatus)
	s.Mux.HandleFunc("POST /api/chat", s.handleChat)
	s.Mux.HandleFunc("POST /api/jobs", s.handleJobCreate)
	s.Mux.HandleFunc("GET /api/jobs", s.handleJobList)
	s.Mux.HandleFunc("GET /api/jobs/{id}", s.handleJobGet)
	s.Mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	s.Mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleJobCancel)
	s.Mux.HandleFunc("DELETE /api/jobs/{id}", s.handleJobDelete)
	s.Mux.HandleFunc("POST /api/batches", s.handleBatchCreate)
	s.Mux.HandleFunc("GET /api/batches/{id}", s.handleBatchGet)
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := checkHost(r, s.Cfg.Router.AllowedHosts); err != nil {
			writeErr(w, err.(apperrors.APIError))
			return
		}
		if err := checkOrigin(r, s.Cfg.Router.AllowedHosts); err != nil {
			writeErr(w, err.(apperrors.APIError))
			return
		}
		if s.shuttingDown.Load() && r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, apperrors.New("ROUTER_RESTARTING", "router is shutting down"))
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		s.Mux.ServeHTTP(rec, r)
		if rec.swallowed && !rec.wrote && strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, apperrors.New("NOT_FOUND", "not found"))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status    int
	swallowed bool
	wrote     bool
}

// The mux answers unknown /api/ paths with a plain-text 404. That is the only
// 404 held back, and only while no body has been written, so handlers that
// send their own JSON error with a 404 status keep it.
func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	if code == http.StatusNotFound && !r.wrote && r.Header().Get("Content-Type") != "application/json" {
		r.swallowed = true
		return
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.swallowed {
		return len(b), nil
	}
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok && !r.swallowed {
		f.Flush()
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"ok": true, "version": Version, "jobs": s.Jobs != nil}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleMachines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.Reg.ListMachines())
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.Reg.ListModels())
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Name    string `json:"name"`
		Fast    bool   `json:"fast"`
		Model   string `json:"model,omitempty"`
		Machine string `json:"machine,omitempty"`
	}
	names := make([]string, 0, len(s.Cfg.Profiles))
	for name := range s.Cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]row, 0, len(names))
	for _, name := range names {
		p := s.Cfg.Profiles[name]
		out = append(out, row{Name: name, Fast: p.Fast, Model: p.Model, Machine: p.Machine})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func writeErr(w http.ResponseWriter, e apperrors.APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code": e.Code, "message": e.Message, "retryable": e.Retryable,
		},
	})
}

func writeLine(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func readBodyLimited(r *http.Request, max int64) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, max+1))
}
