package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/store"
)

func (s *Server) jobsEnabled(w http.ResponseWriter) bool {
	if s.Jobs == nil {
		writeErr(w, apperrors.New("NOT_FOUND", "jobs not configured"))
		return false
	}
	return true
}

func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	max := s.Cfg.Router.MaxBodyBytes
	body, err := readBodyLimited(r, max)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	if int64(len(body)) > max {
		writeErr(w, apperrors.New("BODY_TOO_LARGE", "body too large"))
		return
	}
	var cb ChatBody
	if err := json.Unmarshal(body, &cb); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	if _, err := cb.messages(); err != nil {
		writeErr(w, err.(apperrors.APIError))
		return
	}
	id, err := s.Jobs.Submit(r.Context(), string(body))
	if err != nil {
		var re *jobs.RouteRejectedError
		if errors.As(err, &re) {
			writeErr(w, apperrors.New(re.Code, routeMessages(re.Code)))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "could not create job"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (s *Server) handleJobList(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	status := r.URL.Query().Get("status")
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
	lane := r.URL.Query().Get("lane")
	batchID := r.URL.Query().Get("batch_id")
	list, err := s.Jobs.List(r.Context(), status, lane, batchID, before, limit)
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	out := make([]any, 0, len(list))
	for _, j := range list {
		out = append(out, jobSummary(j))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	j, err := s.Jobs.Get(r.Context(), id)
	if err != nil {
		writeErr(w, apperrors.New("NOT_FOUND", "job not found"))
		return
	}
	detail := jobDetail(j)
	if events, err := s.Jobs.EventsSince(r.Context(), id, 1); err == nil {
		detail["status_history"] = statusHistoryFromEvents(events)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func statusHistoryFromEvents(events []store.Event) []map[string]any {
	var hist []map[string]any
	for _, e := range events {
		if e.Type != "status" && e.Type != "error" {
			continue
		}
		var data map[string]any
		_ = json.Unmarshal([]byte(e.DataJSON), &data)
		if data == nil {
			data = map[string]any{}
		}
		entry := map[string]any{"at": e.CreatedAt, "type": e.Type}
		for k, v := range data {
			entry[k] = v
		}
		hist = append(hist, entry)
	}
	return hist
}

func (s *Server) handleJobDelete(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.Jobs.Delete(r.Context(), id); err != nil {
		writeErr(w, apperrors.New("NOT_FOUND", "job not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.Jobs.Cancel(r.Context(), id); err != nil {
		writeErr(w, apperrors.New("CONFLICT", "cannot cancel"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	fromSeq := int64(1)
	if v := r.URL.Query().Get("from"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			fromSeq = n
		}
	}
	j, err := s.Jobs.Get(r.Context(), id)
	if err != nil {
		writeErr(w, apperrors.New("NOT_FOUND", "job not found"))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	seen := fromSeq - 1
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	ctx := r.Context()
	for {
		evs, err := s.Jobs.EventsSince(ctx, id, fromSeq)
		if err != nil {
			return
		}
		for _, e := range evs {
			if e.Seq <= seen {
				continue
			}
			seen = e.Seq
			if err := writeJobEvent(w, e); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		j, _ = s.Jobs.Get(ctx, id)
		if j != nil && terminalStatus(j.Status) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func terminalStatus(st store.JobStatus) bool {
	switch st {
	case store.StatusDone, store.StatusFailed, store.StatusCancelled:
		return true
	default:
		return false
	}
}

func writeJobEvent(w http.ResponseWriter, e store.Event) error {
	var data map[string]any
	if e.DataJSON != "" {
		_ = json.Unmarshal([]byte(e.DataJSON), &data)
	}
	if data == nil {
		data = map[string]any{}
	}
	data["seq"] = e.Seq
	data["type"] = e.Type
	return writeLine(w, data)
}

func jobSummary(j *store.Job) map[string]any {
	prompt := promptFromRequest(j.RequestJSON)
	return map[string]any{
		"id": j.ID, "created_at": j.CreatedAt, "updated_at": j.UpdatedAt,
		"status": j.Status, "machine": j.Machine, "model": j.Model, "mode": j.Mode,
		"lane": j.Lane, "batch_id": j.BatchID,
		"prompt_preview": truncate(prompt, 120),
	}
}

func jobDetail(j *store.Job) map[string]any {
	cb := map[string]any{}
	_ = json.Unmarshal([]byte(j.RequestJSON), &cb)
	delete(cb, "messages")
	out := jobSummary(j)
	out["request"] = cb
	out["output_text"] = j.OutputText
	out["output_thinking"] = j.OutputThinking
	out["attempt"] = j.Attempt
	out["error"] = nil
	if j.ErrorCode != "" {
		out["error"] = map[string]string{"code": j.ErrorCode, "message": j.ErrorMessage}
	}
	if j.FirstFailureAt != nil {
		out["first_failure_at"] = j.FirstFailureAt
	}
	if j.NextAttemptAt != nil {
		out["next_attempt_at"] = j.NextAttemptAt
	}
	return out
}

func promptFromRequest(raw string) string {
	var m map[string]any
	_ = json.Unmarshal([]byte(raw), &m)
	if p, ok := m["prompt"].(string); ok && p != "" {
		return p
	}
	msgs, ok := m["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return ""
	}
	last := msgs[len(msgs)-1]
	if mm, ok := last.(map[string]any); ok {
		if c, ok := mm["content"].(string); ok {
			return c
		}
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

