package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/schedules"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/azamasad/pingpong/internal/templates"
)

func (s *Server) handleScheduleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	sch, err := s.parseScheduleBody(r.Context(), body, true)
	if err != nil {
		writeScheduleErr(w, err)
		return
	}
	st := s.Jobs.Store()
	if _, err := st.GetScheduleByName(r.Context(), sch.Name); err == nil {
		writeErr(w, apperrors.New("CONFLICT", "schedule name already exists"))
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	n, err := st.CountSchedules(r.Context())
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	if n >= s.Cfg.Schedules.MaxCount {
		writeErr(w, apperrors.New("CONFLICT", "schedule count limit reached"))
		return
	}
	now := time.Now().UTC()
	sch.CreatedAt, sch.UpdatedAt = now, now
	if sch.Missed == "" {
		sch.Missed = "skip"
	}
	if sch.Overlap == "" {
		sch.Overlap = "skip"
	}
	if err := st.InsertSchedule(r.Context(), sch); err != nil {
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	s.Jobs.Scheduler().Notify()
	writeScheduleRecord(w, http.StatusCreated, s, r, sch)
}

func (s *Server) handleScheduleList(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	list, err := s.Jobs.Store().ListSchedules(r.Context())
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	now := time.Now()
	out := make([]any, 0, len(list))
	for _, sch := range list {
		out = append(out, scheduleSummary(s, r, sch, now))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleScheduleGet(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	sch, err := s.Jobs.Store().GetSchedule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "schedule not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "get failed"))
		return
	}
	writeScheduleRecord(w, http.StatusOK, s, r, sch)
}

func (s *Server) handleSchedulePatch(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	st := s.Jobs.Store()
	existing, err := st.GetSchedule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "schedule not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "update failed"))
		return
	}
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(body, &patch); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	merged, err := s.mergeSchedulePatch(r.Context(), existing, patch)
	if err != nil {
		writeScheduleErr(w, err)
		return
	}
	if nameRaw, ok := patch["name"]; ok {
		var newName string
		_ = json.Unmarshal(nameRaw, &newName)
		if newName != existing.Name {
			if !templates.ValidName(newName) {
				writeErr(w, apperrors.New("BAD_REQUEST", "invalid name"))
				return
			}
			if other, err := st.GetScheduleByName(r.Context(), newName); err == nil && other.ID != id {
				writeErr(w, apperrors.New("CONFLICT", "schedule name already exists"))
				return
			}
			merged.Name = newName
		}
	}
	merged.UpdatedAt = time.Now().UTC()
	merged.ID = id
	merged.LastDue = existing.LastDue
	if err := st.UpdateSchedule(r.Context(), merged); err != nil {
		writeErr(w, apperrors.New("INTERNAL", "update failed"))
		return
	}
	s.Jobs.Scheduler().Notify()
	writeScheduleRecord(w, http.StatusOK, s, r, merged)
}

func (s *Server) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.Jobs.Store().DeleteSchedule(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "schedule not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "delete failed"))
		return
	}
	s.Jobs.Scheduler().Notify()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScheduleRunNow(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	run, err := s.Jobs.Scheduler().RunNow(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "schedule not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "run failed"))
		return
	}
	out := map[string]any{"run_id": run.ID}
	if run.JobID != "" {
		out["job_id"] = run.JobID
	}
	if run.BatchID != "" {
		out["batch_id"] = run.BatchID
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleScheduleRuns(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := s.Jobs.Store().GetSchedule(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "schedule not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	runs, err := s.Jobs.Store().ListScheduleRuns(r.Context(), id, limit)
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	out := make([]any, 0, len(runs))
	for _, run := range runs {
		row := map[string]any{
			"id": run.ID, "due": run.Due.UTC().Format(time.RFC3339),
			"started": run.Started.UTC().Format(time.RFC3339),
			"status":  run.Status, "state": s.Jobs.RunStateForScheduleRun(r.Context(), run),
		}
		if run.Kind != "" {
			row["kind"] = run.Kind
		}
		if run.JobID != "" {
			row["job_id"] = run.JobID
		}
		if run.BatchID != "" {
			row["batch_id"] = run.BatchID
		}
		if run.ErrorCode != "" {
			row["error_code"] = run.ErrorCode
		}
		out = append(out, row)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) parseScheduleBody(ctx context.Context, body []byte, create bool) (*store.Schedule, error) {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, scheduleErr("BAD_REQUEST", "invalid json")
	}
	sch := &store.Schedule{}
	if v, ok := req["name"]; ok {
		_ = json.Unmarshal(v, &sch.Name)
	}
	if create && (!templates.ValidName(sch.Name) || sch.Name == "") {
		return nil, scheduleErr("BAD_REQUEST", "invalid name")
	}
	if v, ok := req["rule"]; ok {
		rule, err := schedules.ValidateRule(decodeRule(v))
		if err != nil {
			return nil, scheduleErr("BAD_REQUEST", "invalid rule")
		}
		b, _ := json.Marshal(rule)
		sch.RuleJSON = string(b)
	} else if create {
		return nil, scheduleErr("BAD_REQUEST", "rule required")
	}
	if v, ok := req["machine"]; ok {
		_ = json.Unmarshal(v, &sch.Machine)
	}
	if v, ok := req["model"]; ok {
		_ = json.Unmarshal(v, &sch.Model)
	}
	if create && (sch.Machine == "" || sch.Model == "") {
		return nil, scheduleErr("BAD_REQUEST", "machine and model required")
	}
	if sch.Machine != "" {
		if _, ok := s.Cfg.MachineByName(sch.Machine); !ok {
			return nil, scheduleErr("MACHINE_UNKNOWN", "unknown machine")
		}
	}
	var hasTpl, hasPrompt bool
	if v, ok := req["template"]; ok {
		_ = json.Unmarshal(v, &sch.Template)
		hasTpl = sch.Template != ""
	}
	if v, ok := req["prompt"]; ok {
		_ = json.Unmarshal(v, &sch.Prompt)
		hasPrompt = sch.Prompt != ""
	}
	if create {
		if hasTpl == hasPrompt {
			return nil, scheduleErr("BAD_REQUEST", "template or prompt required")
		}
		if hasTpl {
			if _, err := s.Jobs.Store().GetTemplate(ctx, sch.Template); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return nil, scheduleErr("TEMPLATE_NOT_FOUND", "template not found")
				}
				return nil, scheduleErr("INTERNAL", "create failed")
			}
		}
	}
	if v, ok := req["vars"]; ok {
		sch.VarsJSON = string(v)
	}
	if v, ok := req["rows"]; ok {
		sch.RowsJSON = string(v)
		if hasTpl {
			var rows []json.RawMessage
			if err := json.Unmarshal(v, &rows); err != nil {
				return nil, scheduleErr("BAD_REQUEST", "invalid rows")
			}
			if len(rows) > s.Cfg.Batches.MaxItems {
				return nil, scheduleErr("BAD_REQUEST", "too many rows")
			}
		} else if create {
			return nil, scheduleErr("BAD_REQUEST", "rows require template")
		}
	}
	if v, ok := req["missed"]; ok {
		_ = json.Unmarshal(v, &sch.Missed)
		if sch.Missed != "skip" && sch.Missed != "run_once" {
			return nil, scheduleErr("BAD_REQUEST", "invalid missed")
		}
	}
	if v, ok := req["overlap"]; ok {
		_ = json.Unmarshal(v, &sch.Overlap)
		if sch.Overlap != "skip" && sch.Overlap != "allow" {
			return nil, scheduleErr("BAD_REQUEST", "invalid overlap")
		}
	}
	if v, ok := req["enabled"]; ok {
		var en bool
		_ = json.Unmarshal(v, &en)
		sch.Enabled = en
	} else if create {
		sch.Enabled = true
	}
	return sch, nil
}

func decodeRule(raw json.RawMessage) schedules.Rule {
	var r schedules.Rule
	_ = json.Unmarshal(raw, &r)
	return r
}

type scheduleAPIErr struct {
	code, msg string
}

func scheduleErr(code, msg string) error { return &scheduleAPIErr{code, msg} }

func (e *scheduleAPIErr) Error() string { return e.msg }

func writeScheduleErr(w http.ResponseWriter, err error) {
	var se *scheduleAPIErr
	if errors.As(err, &se) {
		writeErr(w, apperrors.New(se.code, se.msg))
		return
	}
	var te *jobs.TemplateRejectError
	if errors.As(err, &te) {
		writeJobErr(w, te)
		return
	}
	writeErr(w, apperrors.New("BAD_REQUEST", err.Error()))
}

func (s *Server) mergeSchedulePatch(ctx context.Context, existing *store.Schedule, patch map[string]json.RawMessage) (*store.Schedule, error) {
	full := schedulePatchBase(existing)
	for k, v := range patch {
		full[k] = v
	}
	b, _ := json.Marshal(full)
	sch, err := s.parseScheduleBody(ctx, b, false)
	if err != nil {
		return nil, err
	}
	if sch.Name == "" {
		sch.Name = existing.Name
	}
	if sch.RuleJSON == "" {
		sch.RuleJSON = existing.RuleJSON
	}
	if sch.Machine == "" {
		sch.Machine = existing.Machine
	}
	if sch.Model == "" {
		sch.Model = existing.Model
	}
	if sch.Template == "" && sch.Prompt == "" {
		sch.Template, sch.Prompt = existing.Template, existing.Prompt
	}
	if sch.VarsJSON == "" {
		sch.VarsJSON = existing.VarsJSON
	}
	if sch.RowsJSON == "" {
		sch.RowsJSON = existing.RowsJSON
	}
	if sch.Missed == "" {
		sch.Missed = existing.Missed
	}
	if sch.Overlap == "" {
		sch.Overlap = existing.Overlap
	}
	if _, ok := patch["enabled"]; !ok {
		sch.Enabled = existing.Enabled
	}
	if sch.Template != "" {
		if _, err := s.Jobs.Store().GetTemplate(ctx, sch.Template); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, scheduleErr("TEMPLATE_NOT_FOUND", "template not found")
			}
			return nil, scheduleErr("INTERNAL", "update failed")
		}
	}
	if sch.RowsJSON != "" {
		var rows []json.RawMessage
		_ = json.Unmarshal([]byte(sch.RowsJSON), &rows)
		if len(rows) > s.Cfg.Batches.MaxItems {
			return nil, scheduleErr("BAD_REQUEST", "too many rows")
		}
	}
	if sch.Template == "" && sch.Prompt == "" {
		return nil, scheduleErr("BAD_REQUEST", "template or prompt required")
	}
	if sch.Template != "" && sch.Prompt != "" {
		return nil, scheduleErr("BAD_REQUEST", "template and prompt cannot both be set")
	}
	return sch, nil
}

func schedulePatchBase(existing *store.Schedule) map[string]json.RawMessage {
	out := map[string]json.RawMessage{
		"name":    mustRaw(existing.Name),
		"rule":    json.RawMessage(existing.RuleJSON),
		"machine": mustRaw(existing.Machine),
		"model":   mustRaw(existing.Model),
		"missed":  mustRaw(existing.Missed),
		"overlap": mustRaw(existing.Overlap),
		"enabled": json.RawMessage("true"),
	}
	if !existing.Enabled {
		out["enabled"] = json.RawMessage("false")
	}
	if existing.Template != "" {
		out["template"] = mustRaw(existing.Template)
	}
	if existing.Prompt != "" {
		out["prompt"] = mustRaw(existing.Prompt)
	}
	if existing.VarsJSON != "" {
		out["vars"] = json.RawMessage(existing.VarsJSON)
	}
	if existing.RowsJSON != "" {
		out["rows"] = json.RawMessage(existing.RowsJSON)
	}
	return out
}

func mustRaw(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func scheduleSummary(s *Server, r *http.Request, sch *store.Schedule, now time.Time) map[string]any {
	var rule map[string]any
	_ = json.Unmarshal([]byte(sch.RuleJSON), &rule)
	out := map[string]any{
		"id": sch.ID, "name": sch.Name, "rule": rule,
		"machine": sch.Machine, "model": sch.Model,
		"missed": sch.Missed, "overlap": sch.Overlap, "enabled": sch.Enabled,
		"created_at": sch.CreatedAt.UTC().Format(time.RFC3339),
	}
	if sch.Enabled {
		ruleObj, _ := schedules.ParseRuleJSON(sch.RuleJSON)
		ref := sch.CreatedAt
		if sch.LastDue != nil {
			ref = *sch.LastDue
		}
		out["next_due"] = ruleObj.NextDueAfter(ref, time.Local).UTC().Format(time.RFC3339)
	}
	if sch.LastDue != nil {
		out["last_due"] = sch.LastDue.UTC().Format(time.RFC3339)
	}
	if run, err := s.Jobs.Store().LastScheduleRun(r.Context(), sch.ID); err == nil {
		out["last_state"] = s.Jobs.RunStateForScheduleRun(r.Context(), run)
	}
	target := sch.Template
	if target == "" {
		target = "(prompt)"
	}
	out["target"] = target
	_ = now
	return out
}

func writeScheduleRecord(w http.ResponseWriter, code int, s *Server, r *http.Request, sch *store.Schedule) {
	m := scheduleSummary(s, r, sch, time.Now())
	var rule map[string]any
	_ = json.Unmarshal([]byte(sch.RuleJSON), &rule)
	m["rule"] = rule
	if sch.Template != "" {
		m["template"] = sch.Template
	}
	if sch.Prompt != "" {
		m["prompt"] = sch.Prompt
	}
	if sch.VarsJSON != "" {
		var vars map[string]any
		_ = json.Unmarshal([]byte(sch.VarsJSON), &vars)
		m["vars"] = vars
	}
	if sch.RowsJSON != "" {
		var rows []any
		_ = json.Unmarshal([]byte(sch.RowsJSON), &rows)
		m["rows"] = rows
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(m)
}
