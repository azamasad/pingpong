package jobs

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/google/uuid"
)

// SplitTarget is one machine/model pair in a split plan.
type SplitTarget struct {
	Machine string `json:"machine"`
	Model   string `json:"model"`
	Weight  int    `json:"weight"`
}

// SplitPlan is stored on the batch and echoed in API responses.
type SplitPlan struct {
	Assign  string        `json:"assign"`
	Targets []SplitTarget `json:"targets"`
}

// BatchSubmitResult is the outcome of creating a batch.
type BatchSubmitResult struct {
	ID       string
	JobIDs   []string
	Split    map[string]any
	NotSplit int
}

func (s *Service) MaxBatchItems() int { return s.cfg.Batches.MaxItems }

func itemSelfRouted(cb chatBody) bool {
	return cb.Machine != "" || cb.Model != "" || cb.Fast || cb.Profile != ""
}

func itemLaneInvalid(cb chatBody) bool {
	return cb.Lane != "" && cb.Lane != store.LaneBatch
}

func pinBodyJSON(body string, machine, model string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", err
	}
	m["machine"] = machine
	m["model"] = model
	delete(m, "fast")
	delete(m, "profile")
	b, err := json.Marshal(m)
	return string(b), err
}

func validateSplitTargets(s *Service, plan *SplitPlan) error {
	if plan == nil {
		return nil
	}
	if plan.Assign == "" {
		plan.Assign = "round_robin"
	}
	if plan.Assign != "round_robin" && plan.Assign != "weights" {
		return &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if len(plan.Targets) < 2 {
		return &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if plan.Assign == "round_robin" {
		for _, t := range plan.Targets {
			if t.Weight > 1 {
				return &RouteRejectedError{Code: "BAD_REQUEST"}
			}
		}
	}
	seenMachine := map[string]struct{}{}
	for i := range plan.Targets {
		t := &plan.Targets[i]
		if t.Weight == 0 {
			t.Weight = 1
		}
		if t.Weight < 1 || t.Weight > 100 {
			return &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		if _, ok := s.cfg.MachineByName(t.Machine); !ok {
			return &RouteRejectedError{Code: route.CodeMachineUnknown}
		}
		if _, dup := seenMachine[t.Machine]; dup {
			return &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		seenMachine[t.Machine] = struct{}{}
		model := t.Model
		machine := t.Machine
		model, machine, _ = s.cfg.ResolveAlias(model, machine)
		t.Model = model
		res := s.resolve(route.Request{Model: model, Machine: machine})
		if !res.OK && !retryableCode(res.Code) {
			return &RouteRejectedError{Code: res.Code}
		}
	}
	return nil
}

func splitTargetIndex(k int, plan SplitPlan) int {
	n := len(plan.Targets)
	if n == 0 {
		return 0
	}
	if plan.Assign == "round_robin" {
		return k % n
	}
	W := 0
	for _, t := range plan.Targets {
		W += t.Weight
	}
	slot := k % W
	pos := 0
	for i, t := range plan.Targets {
		if slot < pos+t.Weight {
			return i
		}
		pos += t.Weight
	}
	return n - 1
}

func (s *Service) prepareBatchJob(bodyJSON, batchID string, pinMachine, pinModel string) (*store.Job, error) {
	return s.prepareCheckedJob(bodyJSON, batchID, pinMachine, pinModel)
}

// SubmitBatch validates every item, inserts the batch row and jobs in one transaction, then starts run loops.
func (s *Service) SubmitBatch(ctx context.Context, bodies []string, plan *SplitPlan) (*BatchSubmitResult, error) {
	max := s.MaxBatchItems()
	if len(bodies) == 0 || len(bodies) > max {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if plan != nil {
		if err := validateSplitTargets(s, plan); err != nil {
			return nil, err
		}
	}
	batchID := uuid.NewString()
	now := s.clk.Now().UTC()
	kind := "batch"
	var planJSON string
	if plan != nil {
		kind = "split"
		b, _ := json.Marshal(plan)
		planJSON = string(b)
	}
	splitIdx := 0
	notSplit := 0
	jobs := make([]*store.Job, 0, len(bodies))
	targetCounts := map[int]int{}
	if plan != nil {
		for i := range plan.Targets {
			targetCounts[i] = 0
		}
	}
	for _, body := range bodies {
		cb, _, err := parseChatBody(body)
		if err != nil {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		if itemLaneInvalid(cb) {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		pinM, pinMod := "", ""
		if plan != nil && !itemSelfRouted(cb) {
			ti := splitTargetIndex(splitIdx, *plan)
			splitIdx++
			pinM = plan.Targets[ti].Machine
			pinMod = plan.Targets[ti].Model
			targetCounts[ti]++
		} else if plan != nil && itemSelfRouted(cb) {
			notSplit++
		}
		j, err := s.prepareBatchJob(body, batchID, pinM, pinMod)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	batch := &store.Batch{
		ID: batchID, CreatedAt: now, Kind: kind,
		ItemCount: len(bodies), PlanJSON: planJSON,
	}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.store.InsertBatchTx(ctx, tx, batch); err != nil {
			return err
		}
		for _, j := range jobs {
			if err := s.store.InsertJobTx(ctx, tx, j); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
		go s.runLoop(j.ID)
	}
	source := "plain"
	if plan != nil {
		source = "split"
	}
	logx.Event("info", "batch_submitted", map[string]any{
		"batch_id": batchID, "items": len(bodies), "source": source,
	})
	var splitOut map[string]any
	if plan != nil {
		targets := make([]map[string]any, len(plan.Targets))
		for i, t := range plan.Targets {
			targets[i] = map[string]any{
				"machine": t.Machine, "model": t.Model,
				"weight": t.Weight, "count": targetCounts[i],
			}
		}
		splitOut = map[string]any{
			"assign": plan.Assign, "targets": targets,
		}
	}
	return &BatchSubmitResult{
		ID: batchID, JobIDs: ids, Split: splitOut, NotSplit: notSplit,
	}, nil
}

// CancelBatch cancels every active job in the batch.
func (s *Service) CancelBatch(ctx context.Context, batchID string) (int, error) {
	hasJobs, err := s.store.BatchJobIDsExist(ctx, batchID)
	if err != nil {
		return 0, err
	}
	_, batchErr := s.store.GetBatch(ctx, batchID)
	if !hasJobs && batchErr != nil {
		return 0, store.ErrNotFound
	}
	active, err := s.store.ActiveJobsByBatch(ctx, batchID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, j := range active {
		if err := s.Cancel(ctx, j.ID); err == nil {
			n++
		}
	}
	return n, nil
}