package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/google/uuid"
)

// Fixed check prompts (D-108). Change only with a DECISIONS entry.
const reviewPromptTemplate = `You are checking another model's answer to a question. Read both.
Start your reply with OK if the answer is correct and complete, or PROBLEMS if it is not.
If PROBLEMS, list each mistake or gap as a short numbered line. Do not rewrite the answer.

QUESTION:
%s

ANSWER:
%s`

const revisePromptTemplate = `Below are a question and a draft answer. Write the corrected final answer to the question.
Output only the final answer, with no commentary.

QUESTION:
%s

DRAFT ANSWER:
%s`

// CheckSpec is the user-visible check step on a job or batch item.
type CheckSpec struct {
	Mode    string         `json:"mode"`
	Machine string         `json:"machine,omitempty"`
	Model   string         `json:"model,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

func parseCheck(raw string) (*CheckSpec, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("BAD_REQUEST")
	}
	ch, ok := m["check"]
	if !ok {
		return nil, nil
	}
	var spec CheckSpec
	if err := json.Unmarshal(ch, &spec); err != nil {
		return nil, fmt.Errorf("BAD_REQUEST")
	}
	return &spec, nil
}

func validateCheck(spec *CheckSpec) error {
	if spec == nil {
		return nil
	}
	if spec.Mode != "review" && spec.Mode != "revise" {
		return &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	return nil
}

func (s *Service) resolveCheckStage2(spec CheckSpec) route.Result {
	req := route.Request{Fast: false, Machine: spec.Machine, Model: spec.Model}
	req.Model, req.Machine, _ = s.cfg.ResolveAlias(req.Model, req.Machine)
	return s.resolve(req)
}

func applyStage1DefaultFast(cb *chatBody, hasCheck bool) {
	if !hasCheck {
		return
	}
	if cb.Profile != "" || cb.Machine != "" || cb.Model != "" || cb.Fast {
		return
	}
	cb.Fast = true
}

func questionText(bodyJSON string) (string, error) {
	cb, _, err := parseChatBody(bodyJSON)
	if err != nil {
		return "", err
	}
	if cb.Prompt != "" {
		return cb.Prompt, nil
	}
	var b strings.Builder
	for _, m := range cb.Messages {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s: %s", m.Role, m.Content)
	}
	return b.String(), nil
}

func buildCheckPrompt(mode, question, answer string) string {
	if mode == "revise" {
		return fmt.Sprintf(revisePromptTemplate, question, answer)
	}
	return fmt.Sprintf(reviewPromptTemplate, question, answer)
}

func contextEstimate(parent *store.Job, stats map[string]any, question, answer string) int {
	pt, ot := 0, 0
	if stats != nil {
		if v, ok := stats["prompt_tokens"].(int); ok {
			pt = v
		}
		if v, ok := stats["prompt_tokens"].(float64); ok {
			pt = int(v)
		}
		if v, ok := stats["output_tokens"].(int); ok {
			ot = v
		}
		if v, ok := stats["output_tokens"].(float64); ok {
			ot = int(v)
		}
	}
	if pt == 0 && parent.PromptTokens > 0 {
		pt = parent.PromptTokens
	}
	if ot == 0 && parent.OutputTokens > 0 {
		ot = parent.OutputTokens
	}
	if pt+ot > 0 {
		return pt + ot + 150
	}
	return (len(question)+len(answer))/3 + 150
}

func (s *Service) onAnswerFinished(ctx context.Context, parent *store.Job, stats map[string]any) {
	if parent.CheckPending == 0 || parent.Stage != store.StageAnswer {
		return
	}
	spec, err := parseCheck(parent.RequestJSON)
	if err != nil || spec == nil {
		_ = s.store.ClearCheckPending(ctx, parent.ID)
		return
	}
	_ = s.createCheckStage(ctx, parent, spec, stats)
}

func (s *Service) recoverPendingChecks(ctx context.Context) {
	list, err := s.store.JobsNeedingCheckStage(ctx)
	if err != nil {
		return
	}
	for _, parent := range list {
		spec, err := parseCheck(parent.RequestJSON)
		if err != nil || spec == nil {
			_ = s.store.ClearCheckPending(ctx, parent.ID)
			continue
		}
		_ = s.createCheckStage(ctx, parent, spec, nil)
	}
}

func (s *Service) createCheckStage(ctx context.Context, parent *store.Job, spec *CheckSpec, stats map[string]any) error {
	if _, err := s.store.ChildByParentStage(ctx, parent.ID, spec.Mode); err == nil {
		return nil
	} else if err != store.ErrNotFound {
		return err
	}
	question, err := questionText(parent.RequestJSON)
	if err != nil {
		return err
	}
	answer := parent.OutputText
	prompt := buildCheckPrompt(spec.Mode, question, answer)

	res := s.resolveCheckStage2(*spec)
	now := s.clk.Now().UTC()
	childID := uuid.NewString()
	child := &store.Job{
		ID: childID, CreatedAt: now, UpdatedAt: now,
		ParentID: parent.ID, Stage: spec.Mode,
		Lane: parent.Lane, BatchID: parent.BatchID,
		RequestJSON: mustJSON(map[string]any{
			"prompt": prompt, "machine": res.Machine, "model": res.Model,
			"options": filterOptions(spec.Options),
		}),
	}
	oversize := false
	if res.OK {
		mc, _ := s.cfg.MachineByName(res.Machine)
		est := contextEstimate(parent, stats, question, answer)
		if est+512 > mc.NumCtx {
			oversize = true
		}
		child.Machine, child.Model, child.Mode = res.Machine, res.Model, res.Mode
		child.Status = store.StatusWaiting
	} else if retryableCode(res.Code) {
		child.Status = store.StatusRetrying
		child.ErrorCode = res.Code
		t := now
		child.FirstFailureAt = &t
		na := now.Add(s.cfg.Retry.RetryInterval.Duration)
		child.NextAttemptAt = &na
	} else {
		child.Status = store.StatusFailed
		child.ErrorCode = res.Code
	}
	if oversize {
		child.Status = store.StatusFailed
		child.ErrorCode = "CONTEXT_TOO_LARGE"
		child.ErrorMessage = "review prompt does not fit context"
	}

	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		dup, err := s.store.ChildByParentStageTx(ctx, tx, parent.ID, spec.Mode)
		if err != nil && err != store.ErrNotFound {
			return err
		}
		if dup != nil {
			return nil
		}
		if err := s.store.InsertJobTx(ctx, tx, child); err != nil {
			return err
		}
		return s.store.ClearCheckPendingTx(ctx, tx, parent.ID)
	})
	if err != nil {
		return err
	}
	if dup, _ := s.store.ChildByParentStage(ctx, parent.ID, spec.Mode); dup != nil && dup.ID != child.ID {
		return nil
	}
	if oversize || (child.Status == store.StatusFailed && child.ErrorCode != "") {
		logx.Event("warn", "check_failed", map[string]any{
			"request_id": parent.ID, "stage": spec.Mode, "error_code": child.ErrorCode,
		})
	} else {
		logx.Event("info", "check_created", map[string]any{
			"request_id": parent.ID, "stage": spec.Mode,
		})
		if child.Status == store.StatusWaiting || child.Status == store.StatusRetrying {
			go s.runLoop(childID)
		}
	}
	return nil
}

func (s *Service) clearCheckOnTerminal(ctx context.Context, j *store.Job) {
	if j.CheckPending == 0 || j.Stage != store.StageAnswer {
		return
	}
	_ = s.store.ClearCheckPending(ctx, j.ID)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func ensureFastFlag(bodyJSON string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(bodyJSON), &m); err != nil {
		return bodyJSON
	}
	m["fast"] = true
	b, err := json.Marshal(m)
	if err != nil {
		return bodyJSON
	}
	return string(b)
}

func (s *Service) prepareCheckedJob(bodyJSON, batchID string, pinMachine, pinModel string) (*store.Job, error) {
	spec, err := parseCheck(bodyJSON)
	if err != nil {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if err := validateCheck(spec); err != nil {
		return nil, err
	}
	if pinMachine != "" && pinModel != "" {
		var err error
		bodyJSON, err = pinBodyJSON(bodyJSON, pinMachine, pinModel)
		if err != nil {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
	}
	cb, _, err := parseChatBody(bodyJSON)
	if err != nil {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	applyStage1DefaultFast(&cb, spec != nil)
	if spec != nil && cb.Fast && !strings.Contains(bodyJSON, `"fast"`) {
		bodyJSON = ensureFastFlag(bodyJSON)
	}
	fast, model, machine, err := s.cfg.ApplyProfile(cb.Profile, cb.Fast, cb.Model, cb.Machine)
	if err != nil {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	cb.Fast, cb.Model, cb.Machine = fast, model, machine
	cb.Model, cb.Machine, _ = s.cfg.ResolveAlias(cb.Model, cb.Machine)
	res := s.resolve(route.Request{Fast: cb.Fast, Model: cb.Model, Machine: cb.Machine})
	if spec != nil {
		res2 := s.resolveCheckStage2(*spec)
		if !res2.OK && !retryableCode(res2.Code) {
			return nil, &RouteRejectedError{Code: res2.Code}
		}
	}
	now := s.clk.Now().UTC()
	id := uuid.NewString()
	j := &store.Job{
		ID: id, CreatedAt: now, UpdatedAt: now,
		RequestJSON: bodyJSON,
		Status:      store.StatusWaiting,
		Lane:        store.LaneBatch,
		BatchID:     batchID,
	}
	if spec != nil {
		j.Stage = store.StageAnswer
		j.CheckPending = 1
	}
	if res.OK {
		j.Machine, j.Model, j.Mode = res.Machine, res.Model, res.Mode
	} else {
		j.Status = store.StatusRetrying
		j.ErrorCode = res.Code
		t := now
		j.FirstFailureAt = &t
		na := now.Add(s.cfg.Retry.RetryInterval.Duration)
		j.NextAttemptAt = &na
	}
	return j, nil
}