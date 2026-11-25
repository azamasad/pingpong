package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/store"
)

func (s *Server) handleThreadCreate(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var req struct {
		Title   string `json:"title"`
		Profile string `json:"profile"`
		Fast    bool   `json:"fast"`
		Model   string `json:"model"`
		Machine string `json:"machine"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	th, err := s.Jobs.CreateThread(r.Context(), req.Title, req.Profile, req.Fast, req.Model, req.Machine)
	if err != nil {
		if writeThreadErr(w, err) {
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(jobs.ThreadSummary(th, 0))
}

func (s *Server) handleThreadList(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	var before *time.Time
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid before timestamp"))
			return
		}
		before = &t
	}
	list, err := s.Jobs.ListThreads(r.Context(), before, limit)
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleThreadGet(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	detail, err := s.Jobs.ThreadDetail(r.Context(), id, limit)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "thread not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "get failed"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (s *Server) handleThreadMessage(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var req jobs.ThreadMessageReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	jobID, turn, dropped, err := s.Jobs.SubmitThreadMessage(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "thread not found"))
			return
		}
		if writeThreadErr(w, err) {
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "message failed"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"job_id": jobID, "turn": turn, "dropped_turns": dropped,
	})
}

func (s *Server) handleThreadPatch(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var req struct {
		Title   *string `json:"title"`
		Machine *string `json:"machine"`
		Model   *string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	title := ""
	if req.Title != nil {
		title = *req.Title
	}
	pinChange := req.Machine != nil || req.Model != nil
	if pinChange && (req.Machine == nil || req.Model == nil) {
		writeErr(w, apperrors.New("BAD_REQUEST", "machine and model required together"))
		return
	}
	machine, model := "", ""
	if pinChange {
		machine, model = *req.Machine, *req.Model
	}
	th, err := s.Jobs.PatchThread(r.Context(), id, title, machine, model, pinChange)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "thread not found"))
			return
		}
		if writeThreadErr(w, err) {
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "patch failed"))
		return
	}
	n, _ := s.Jobs.Store().ThreadTurnCount(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs.ThreadSummary(th, n))
}

func (s *Server) handleThreadDelete(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.Jobs.DeleteThread(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "thread not found"))
			return
		}
		if writeThreadErr(w, err) {
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "delete failed"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeThreadErr(w http.ResponseWriter, err error) bool {
	var busy *jobs.ThreadBusyError
	if errors.As(err, &busy) {
		writeErr(w, apperrors.New("THREAD_BUSY", "thread has an active turn"))
		return true
	}
	var long *jobs.ThreadTooLongError
	if errors.As(err, &long) {
		writeErrExtra(w, apperrors.New("THREAD_TOO_LONG", "thread history too long"),
			map[string]any{"estimated_tokens": long.Estimated, "limit": long.Limit})
		return true
	}
	return writeJobErr(w, err)
}