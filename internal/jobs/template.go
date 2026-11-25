package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/azamasad/pingpong/internal/templates"
)

// TemplateRejectError carries template-specific API error details.
type TemplateRejectError struct {
	Code    string
	Missing []string
}

func (e *TemplateRejectError) Error() string { return e.Code }

func parseStringVars(m map[string]any) (map[string]string, error) {
	if m == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("BAD_REQUEST")
		}
		out[k] = s
	}
	return out, nil
}

func stripTemplateFields(m map[string]any) {
	delete(m, "template")
	delete(m, "vars")
	delete(m, "rows")
	delete(m, "items")
}

func (s *Service) loadTemplate(ctx context.Context, name string) (*store.Template, error) {
	t, err := s.store.GetTemplate(ctx, name)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, &TemplateRejectError{Code: "TEMPLATE_NOT_FOUND"}
		}
		return nil, err
	}
	return t, nil
}

func templateDefaultsJSON(t *store.Template) map[string]any {
	if t.DefaultsJSON == "" {
		return map[string]any{}
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(t.DefaultsJSON), &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func mergeTemplateVars(shared, row map[string]string) map[string]string {
	out := make(map[string]string, len(shared)+len(row))
	for k, v := range shared {
		out[k] = v
	}
	for k, v := range row {
		out[k] = v
	}
	return out
}

var templateOverlayKeys = map[string]struct{}{
	"profile": {}, "machine": {}, "model": {}, "lane": {},
	"fast": {}, "think": {}, "options": {}, "check": {},
}

func (s *Service) expandOneTemplate(t *store.Template, tplName string, vars map[string]string, overlay map[string]any) (string, []string, error) {
	prompt, missing, unused := templates.Substitute(t.Body, vars)
	if len(missing) > 0 {
		return "", nil, &TemplateRejectError{Code: "TEMPLATE_VARS_MISSING", Missing: missing}
	}
	defs := templateDefaultsJSON(t)
	req := make(map[string]any)
	for k, v := range defs {
		req[k] = v
	}
	for k, v := range overlay {
		if _, ok := templateOverlayKeys[k]; ok {
			req[k] = v
		}
	}
	req["prompt"] = prompt
	req["template"] = tplName
	stripTemplateFields(req)
	b, err := json.Marshal(req)
	if err != nil {
		return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	var warnings []string
	for _, u := range unused {
		warnings = append(warnings, "unused var: "+u)
	}
	return string(b), warnings, nil
}

// ExpandJobTemplate expands a single-job body that names a template.
func (s *Service) ExpandJobTemplate(ctx context.Context, bodyJSON string) (string, []string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bodyJSON), &raw); err != nil {
		return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if _, ok := raw["template"]; !ok {
		return bodyJSON, nil, nil
	}
	var tplName string
	_ = json.Unmarshal(raw["template"], &tplName)
	if tplName == "" {
		return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if _, ok := raw["prompt"]; ok {
		return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if _, ok := raw["messages"]; ok {
		return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	vars := map[string]string{}
	if v, ok := raw["vars"]; ok {
		var vm map[string]any
		if err := json.Unmarshal(v, &vm); err != nil {
			return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		var err error
		vars, err = parseStringVars(vm)
		if err != nil {
			return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
	}
	t, err := s.loadTemplate(ctx, tplName)
	if err != nil {
		return "", nil, err
	}
	var overlay map[string]any
	_ = json.Unmarshal([]byte(bodyJSON), &overlay)
	delete(overlay, "template")
	delete(overlay, "vars")
	return s.expandOneTemplate(t, tplName, vars, overlay)
}

// TemplateBatchInput is the template-driven batch submit payload.
type TemplateBatchInput struct {
	Template string
	Vars     map[string]string
	Rows     []map[string]string
	Split    *SplitPlan
	Overlay  map[string]any
}

// ExpandTemplateBatch builds per-item job JSON bodies from a template and rows.
func (s *Service) ExpandTemplateBatch(ctx context.Context, in TemplateBatchInput) ([]string, []string, error) {
	t, err := s.loadTemplate(ctx, in.Template)
	if err != nil {
		return nil, nil, err
	}
	if len(in.Rows) == 0 {
		return nil, nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	max := s.MaxBatchItems()
	if len(in.Rows) > max {
		return nil, nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	var allWarnings []string
	bodies := make([]string, 0, len(in.Rows))
	for _, row := range in.Rows {
		vars := mergeTemplateVars(in.Vars, row)
		body, warns, err := s.expandOneTemplate(t, in.Template, vars, in.Overlay)
		if err != nil {
			return nil, nil, err
		}
		allWarnings = append(allWarnings, warns...)
		bodies = append(bodies, body)
	}
	return bodies, allWarnings, nil
}

// ValidateTemplateDefaults checks saved defaults on a template.
func ValidateTemplateDefaults(cfg *config.Config, defs map[string]any) error {
	if p, ok := defs["profile"].(string); ok && p != "" {
		if _, ok := cfg.Profiles[p]; !ok {
			return fmt.Errorf("unknown profile")
		}
	}
	if m, ok := defs["machine"].(string); ok && m != "" {
		if _, ok := cfg.MachineByName(m); !ok {
			return fmt.Errorf("unknown machine")
		}
	}
	if l, ok := defs["lane"].(string); ok && l != "" {
		if l != store.LaneInteractive && l != store.LaneBatch {
			return fmt.Errorf("bad lane")
		}
	}
	if ch, ok := defs["check"]; ok && ch != nil {
		b, _ := json.Marshal(map[string]any{"check": ch})
		spec, err := parseCheck(string(b))
		if err != nil || validateCheck(spec) != nil {
			return fmt.Errorf("bad check")
		}
	}
	return nil
}

func EncodeTemplateDefaults(defs map[string]any) string {
	if len(defs) == 0 {
		return ""
	}
	b, _ := json.Marshal(defs)
	return string(b)
}
