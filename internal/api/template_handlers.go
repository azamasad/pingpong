package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/jobs"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/azamasad/pingpong/internal/templates"
)

var allowedTemplateDefaultKeys = map[string]struct{}{
	"profile": {}, "fast": {}, "model": {}, "machine": {}, "lane": {},
	"check": {}, "options": {}, "think": {},
}

func (s *Server) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var req struct {
		Name     string         `json:"name"`
		Body     string         `json:"body"`
		Defaults map[string]any `json:"defaults"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Name == "" || req.Body == "" {
		writeErr(w, apperrors.New("BAD_REQUEST", "name and body required"))
		return
	}
	if !templates.ValidName(req.Name) {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid name"))
		return
	}
	if len(req.Body) > s.Cfg.Templates.MaxBytes {
		writeErr(w, apperrors.New("BAD_REQUEST", "template too large"))
		return
	}
	defs := filterTemplateDefaults(req.Defaults)
	if err := jobs.ValidateTemplateDefaults(s.Cfg, defs); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid defaults"))
		return
	}
	st := s.Jobs.Store()
	if _, err := st.GetTemplate(r.Context(), req.Name); err == nil {
		writeErr(w, apperrors.New("CONFLICT", "template name already exists"))
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	n, err := st.CountTemplates(r.Context())
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	if n >= s.Cfg.Templates.MaxCount {
		writeErr(w, apperrors.New("CONFLICT", "template count limit reached"))
		return
	}
	now := time.Now().UTC()
	t := &store.Template{
		Name: req.Name, Body: req.Body,
		DefaultsJSON: jobs.EncodeTemplateDefaults(defs),
		CreatedAt:    now, UpdatedAt: now,
	}
	if err := st.InsertTemplate(r.Context(), t); err != nil {
		writeErr(w, apperrors.New("INTERNAL", "create failed"))
		return
	}
	writeTemplate(w, http.StatusCreated, t, true)
}

func (s *Server) handleTemplateList(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	list, err := s.Jobs.Store().ListTemplates(r.Context())
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "list failed"))
		return
	}
	out := make([]any, 0, len(list))
	for _, t := range list {
		out = append(out, templateSummary(t))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleTemplateGet(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	name := r.PathValue("name")
	t, err := s.Jobs.Store().GetTemplate(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "template not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "get failed"))
		return
	}
	writeTemplate(w, http.StatusOK, t, true)
}

func (s *Server) handleTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	name := r.PathValue("name")
	body, err := readBodyLimited(r, s.Cfg.Router.MaxBodyBytes)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	var req struct {
		Body     string         `json:"body"`
		Defaults map[string]any `json:"defaults"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Body == "" {
		writeErr(w, apperrors.New("BAD_REQUEST", "body required"))
		return
	}
	if len(req.Body) > s.Cfg.Templates.MaxBytes {
		writeErr(w, apperrors.New("BAD_REQUEST", "template too large"))
		return
	}
	defs := filterTemplateDefaults(req.Defaults)
	if err := jobs.ValidateTemplateDefaults(s.Cfg, defs); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid defaults"))
		return
	}
	st := s.Jobs.Store()
	t, err := st.GetTemplate(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "template not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "update failed"))
		return
	}
	t.Body = req.Body
	t.DefaultsJSON = jobs.EncodeTemplateDefaults(defs)
	t.UpdatedAt = time.Now().UTC()
	if err := st.UpdateTemplate(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "template not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "update failed"))
		return
	}
	writeTemplate(w, http.StatusOK, t, true)
}

func (s *Server) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	name := r.PathValue("name")
	if err := s.Jobs.Store().DeleteTemplate(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, apperrors.New("NOT_FOUND", "template not found"))
			return
		}
		writeErr(w, apperrors.New("INTERNAL", "delete failed"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func filterTemplateDefaults(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any)
	for k, v := range in {
		if _, ok := allowedTemplateDefaultKeys[k]; ok {
			out[k] = v
		}
	}
	return out
}

func templateSummary(t *store.Template) map[string]any {
	var defs map[string]any
	if t.DefaultsJSON != "" {
		_ = json.Unmarshal([]byte(t.DefaultsJSON), &defs)
	}
	return map[string]any{
		"name": t.Name, "vars": templates.BlanksInOrder(t.Body),
		"bytes": len(t.Body), "defaults": defs,
		"created_at": t.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func writeTemplate(w http.ResponseWriter, code int, t *store.Template, withBody bool) {
	m := templateSummary(t)
	if withBody {
		m["body"] = t.Body
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(m)
}

func writeJobErr(w http.ResponseWriter, err error) bool {
	var te *jobs.TemplateRejectError
	if errors.As(err, &te) {
		msg := te.Code
		if te.Code == "TEMPLATE_VARS_MISSING" {
			msg = "missing template variables"
		} else if te.Code == "TEMPLATE_NOT_FOUND" {
			msg = "template not found"
		}
		e := apperrors.New(te.Code, msg)
		if len(te.Missing) > 0 {
			writeErrExtra(w, e, map[string]any{"missing": te.Missing})
		} else {
			writeErr(w, e)
		}
		return true
	}
	var re *jobs.RouteRejectedError
	if errors.As(err, &re) {
		writeErr(w, apperrors.New(re.Code, routeMessages(re.Code)))
		return true
	}
	return false
}

func writeErrExtra(w http.ResponseWriter, e apperrors.APIError, extra map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	errObj := map[string]any{
		"code": e.Code, "message": e.Message, "retryable": e.Retryable,
	}
	for k, v := range extra {
		errObj[k] = v
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": errObj})
}

func parseTemplateVars(raw map[string]json.RawMessage) (map[string]string, error) {
	v, ok := raw["vars"]
	if !ok {
		return map[string]string{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(v, &m); err != nil {
		return nil, errors.New("bad vars")
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			return nil, errors.New("bad vars")
		}
		out[k] = s
	}
	return out, nil
}

func templateOverlayFromRaw(raw map[string]json.RawMessage) map[string]any {
	skip := map[string]struct{}{
		"template": {}, "vars": {}, "rows": {}, "items": {}, "split": {},
	}
	out := map[string]any{}
	for k, v := range raw {
		if _, ok := skip[k]; ok {
			continue
		}
		var anyVal any
		_ = json.Unmarshal(v, &anyVal)
		out[k] = anyVal
	}
	return out
}

func parseTemplateRows(raw []json.RawMessage) ([]map[string]string, error) {
	rows := make([]map[string]string, 0, len(raw))
	for _, r := range raw {
		var m map[string]any
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, errors.New("bad row")
		}
		row := make(map[string]string, len(m))
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				return nil, errors.New("bad row")
			}
			row[k] = s
		}
		rows = append(rows, row)
	}
	return rows, nil
}
