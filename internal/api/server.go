package api

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"sync"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/registry"
)

//go:embed web/*
var webFS embed.FS

const Version = "0.1.0"

type Server struct {
	Cfg      *config.Config
	Reg      *registry.Registry
	Gate     *gate.Gate
	Mux      *http.ServeMux
	shutdown context.CancelFunc
	once     sync.Once
}

func NewServer(cfg *config.Config, reg *registry.Registry, g *gate.Gate) *Server {
	s := &Server{Cfg: cfg, Reg: reg, Gate: g, Mux: http.NewServeMux()}
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
		s.Mux.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": Version})
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
