package api

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/registry"
)

//go:embed web/*
var webFS embed.FS

const Version = "2.0.0"

type Server struct {
	Cfg      *config.Config
	Reg      *registry.Registry
	Gate     *gate.Gate
	Jobs     *jobs.Service
	Mux      *http.ServeMux
	shutdown context.CancelFunc
	once     sync.Once
}

func NewServer(cfg *config.Config, reg *registry.Registry, g *gate.Gate, jobSvc *jobs.Service) *Server {
	s := &Server{Cfg: cfg, Reg: reg, Gate: g, Jobs: jobSvc, Mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	static, _ := fs.Sub(webFS, "web")
	s.Mux.Handle("GET /", http.FileServer(http.FS(static)))
	s.Mux.HandleFunc("GET /api/health", s.handleHealth)
	s.Mux.HandleFunc("GET /api/machines", s.handleMachines)
	s.Mux.HandleFunc("GET /api/models", s.handleModels)
	s.Mux.HandleFunc("POST /api/chat", s.handleChat)
	s.Mux.HandleFunc("POST /api/jobs", s.handleJobCreate)
	s.Mux.HandleFunc("GET /api/jobs", s.handleJobList)
	s.Mux.HandleFunc("GET /api/jobs/{id}", s.handleJobGet)
	s.Mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	s.Mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleJobCancel)
	s.Mux.HandleFunc("DELETE /api/jobs/{id}", s.handleJobDelete)
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
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		s.Mux.ServeHTTP(rec, r)
		if rec.status == http.StatusNotFound && strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, apperrors.New("NOT_FOUND", "not found"))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	if code == http.StatusNotFound {
		return
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == http.StatusNotFound {
		return len(b), nil
	}
	return r.ResponseWriter.Write(b)
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
