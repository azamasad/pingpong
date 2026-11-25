package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/store"
)

func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil || int64(len(body)) > s.Cfg.Router.MaxBodyBytes {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	_, hasTemplate := raw["template"]
	_, hasItems := raw["items"]
	_, hasRows := raw["rows"]
	if hasItems && hasRows {
		writeErr(w, apperrors.New("BAD_REQUEST", "rows and items cannot be used together"))
		return
	}
	if hasTemplate && !hasRows {
		writeErr(w, apperrors.New("BAD_REQUEST", "rows required with template"))
		return
	}
	if hasRows && !hasTemplate {
		writeErr(w, apperrors.New("BAD_REQUEST", "template required with rows"))
		return
	}
	var plan *jobs.SplitPlan
	if v, ok := raw["split"]; ok {
		plan = &jobs.SplitPlan{}
		if err := json.Unmarshal(v, plan); err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid split"))
			return
		}
	}
	var warnings []string
	var bodies []string
	source := ""
	if hasTemplate {
		var tplName string
		_ = json.Unmarshal(raw["template"], &tplName)
		vars, err := parseTemplateVars(raw)
		if err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid vars"))
			return
		}
		rawRows, _ := raw["rows"]
		var rowsRaw []json.RawMessage
		if err := json.Unmarshal(rawRows, &rowsRaw); err != nil || len(rowsRaw) == 0 {
			writeErr(w, apperrors.New("BAD_REQUEST", "rows required"))
			return
		}
		rows, err := parseTemplateRows(rowsRaw)
		if err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid rows"))
			return
		}
		bodies, warnings, err = s.Jobs.ExpandTemplateBatch(r.Context(), jobs.TemplateBatchInput{
			Template: tplName, Vars: vars, Rows: rows, Split: plan,
			Overlay: templateOverlayFromRaw(raw),
		})
		if err != nil {
			if writeJobErr(w, err) {
				return
			}
			writeErr(w, apperrors.New("INTERNAL", "could not expand template"))
			return
		}
		source = tplName
	} else {
		var req struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil || len(req.Items) == 0 {
			writeErr(w, apperrors.New("BAD_REQUEST", "items required"))
			return
		}
		bodies = make([]string, len(req.Items))
		for i, rawItem := range req.Items {
			bodies[i] = string(rawItem)
		}
	}
	res, err := s.Jobs.SubmitBatch(r.Context(), bodies, plan, source)
	if err != nil {
		if writeJobErr(w, err) {
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "could not create batch"))
		return
	}
	out := map[string]any{"id": res.ID, "job_ids": res.JobIDs, "not_split": res.NotSplit}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	if res.Split != nil {
		out["split"] = res.Split
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleBatchList(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
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
	batches, err := s.Jobs.Store().ListBatches(r.Context(), before, limit)
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	out := make([]any, 0, len(batches))
	for _, b := range batches {
		counts, _ := s.Jobs.Store().BatchCounts(r.Context(), b.ID)
		out = append(out, map[string]any{
			"id": b.ID, "kind": b.Kind, "created_at": b.CreatedAt.UTC().Format(time.RFC3339),
			"total": b.ItemCount, "counts": counts,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleBatchGet(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	limit := 200
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	st := s.Jobs.Store()
	batch, batchErr := st.GetBatch(r.Context(), id)
	hasJobs, _ := st.BatchJobIDsExist(r.Context(), id)
	if !hasJobs && batchErr != nil {
		if errors.Is(batchErr, sql.ErrNoRows) {
			writeErr(w, apperrors.New("NOT_FOUND", "batch not found"))
			return
		}
		writeErr(w, apperrors.New("NOT_FOUND", "batch not found"))
		return
	}
	total, _ := st.CountBatchJobs(r.Context(), id)
	counts, _ := st.BatchCounts(r.Context(), id)
	checks, _ := st.CheckBatchCounts(r.Context(), id)
	pending, _ := st.BatchPendingWithChecks(r.Context(), id)
	list, err := st.ListBatchJobs(r.Context(), id, offset, limit)
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "batch failed"))
		return
	}
	jobsOut := make([]any, 0, len(list))
	for _, j := range list {
		jobsOut = append(jobsOut, s.Jobs.JobDetail(r.Context(), j))
	}
	kind := "batch"
	createdAt := ""
	if batchErr == nil && batch != nil {
		kind = batch.Kind
		createdAt = batch.CreatedAt.UTC().Format(time.RFC3339)
	} else if len(list) > 0 {
		createdAt = list[0].CreatedAt.UTC().Format(time.RFC3339)
	}
	resp := map[string]any{
		"id": id, "kind": kind, "created_at": createdAt,
		"total": total, "pending": pending,
		"counts": counts, "checks": checks, "jobs": jobsOut,
	}
	var plan jobs.SplitPlan
	if batchErr == nil && batch != nil && batch.PlanJSON != "" {
		_ = json.Unmarshal([]byte(batch.PlanJSON), &plan)
		resp["split"] = batchSplitProgress(list, &plan)
	} else {
		resp["split"] = nil
	}
	var next any = nil
	if offset+len(list) < total {
		next = offset + len(list)
	}
	resp["next_offset"] = next
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func batchSplitProgress(jobs []*store.Job, plan *jobs.SplitPlan) any {
	if plan == nil || len(plan.Targets) == 0 {
		return nil
	}
	type key struct{ m, mod string }
	perTarget := map[key]map[string]int{}
	counts := map[key]int{}
	for _, t := range plan.Targets {
		k := key{t.Machine, t.Model}
		perTarget[k] = map[string]int{
			"waiting": 0, "running": 0, "retrying": 0,
			"done": 0, "failed": 0, "cancelled": 0,
		}
		counts[k] = 0
	}
	for _, j := range jobs {
		if j.Stage != "" && j.Stage != store.StageAnswer {
			continue
		}
		k := key{j.Machine, j.Model}
		st := string(j.Status)
		if _, ok := perTarget[k]; !ok {
			continue
		}
		perTarget[k][st]++
		counts[k]++
	}
	targets := make([]map[string]any, len(plan.Targets))
	for i, t := range plan.Targets {
		k := key{t.Machine, t.Model}
		c := perTarget[k]
		waiting := c["waiting"] + c["retrying"]
		targets[i] = map[string]any{
			"machine": t.Machine, "model": t.Model, "weight": t.Weight,
			"total": counts[k], "done": c["done"], "running": c["running"],
			"waiting": waiting, "failed": c["failed"], "cancelled": c["cancelled"],
		}
	}
	return map[string]any{"assign": plan.Assign, "targets": targets}
}

func (s *Server) handleBatchCancel(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	id := r.PathValue("id")
	n, err := s.Jobs.CancelBatch(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "batch not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "cancel failed"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"cancelled": n})
}