package jobs

import (
	"context"
	"encoding/json"

	"github.com/azamasad/pingpong/internal/store"
)

// JobDetail is API-facing job detail including check metadata.
func (s *Service) JobDetail(ctx context.Context, j *store.Job) map[string]any {
	out := map[string]any{
		"id": j.ID, "created_at": j.CreatedAt, "updated_at": j.UpdatedAt,
		"status": j.Status, "machine": j.Machine, "model": j.Model, "mode": j.Mode,
		"lane": j.Lane, "batch_id": j.BatchID,
		"stage": j.Stage, "parent_id": j.ParentID,
	}
	if j.ParentID == "" {
		out["parent_id"] = nil
	}
	if j.Stage == "" {
		out["stage"] = nil
	}
	var cb map[string]any
	_ = json.Unmarshal([]byte(j.RequestJSON), &cb)
	delete(cb, "messages")
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
	children, _ := s.store.ChildrenOf(ctx, j.ID)
	childIDs := make([]string, len(children))
	for i, c := range children {
		childIDs[i] = c.ID
	}
	out["children"] = childIDs
	spec, _ := parseCheck(j.RequestJSON)
	if spec != nil && j.Stage == store.StageAnswer {
		ch := map[string]any{"mode": spec.Mode}
		if spec.Machine != "" {
			ch["machine"] = spec.Machine
		}
		if spec.Model != "" {
			ch["model"] = spec.Model
		}
		st := checkState(j, children)
		ch["state"] = st.state
		if st.childID != "" {
			ch["child_id"] = st.childID
		}
		out["check"] = ch
	}
	return out
}

type checkStateOut struct {
	state   string
	childID string
}

func checkState(parent *store.Job, children []*store.Job) checkStateOut {
	if parent.Status != store.StatusDone {
		return checkStateOut{state: "pending"}
	}
	var child *store.Job
	for _, c := range children {
		if c.Stage == "review" || c.Stage == "revise" {
			child = c
			break
		}
	}
	if child == nil {
		if parent.CheckPending == 1 {
			return checkStateOut{state: "pending"}
		}
		return checkStateOut{state: "pending"}
	}
	if child.Status == store.StatusFailed || child.Status == store.StatusCancelled {
		return checkStateOut{state: "unchecked", childID: child.ID}
	}
	return checkStateOut{state: string(child.Status), childID: child.ID}
}