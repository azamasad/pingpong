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

// SubmitScheduleBatch submits a template batch pinned to the schedule pair (kind schedule).
func (s *Service) SubmitScheduleBatch(ctx context.Context, bodies []string, source, machine, model string) (*BatchSubmitResult, error) {
	max := s.MaxBatchItems()
	if len(bodies) == 0 || len(bodies) > max {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	model, machine, _ = s.cfg.ResolveAlias(model, machine)
	res := s.resolve(route.Request{Model: model, Machine: machine})
	if !res.OK && !retryableCode(res.Code) {
		return nil, &RouteRejectedError{Code: res.Code}
	}
	pinned := make([]string, len(bodies))
	for i, body := range bodies {
		b, err := pinBodyJSON(body, machine, model)
		if err != nil {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		lb, err := ensureLaneBatch(b)
		if err != nil {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		pinned[i] = lb
	}
	batchID := uuid.NewString()
	now := s.clk.Now().UTC()
	jobs := make([]*store.Job, 0, len(pinned))
	for _, body := range pinned {
		j, err := s.prepareBatchJob(body, batchID, machine, model)
		if err != nil {
			return nil, err
		}
		if !res.OK {
			j.Status = store.StatusRetrying
			j.ErrorCode = res.Code
			t := now
			j.FirstFailureAt = &t
			na := now.Add(s.cfg.Retry.RetryInterval.Duration)
			j.NextAttemptAt = &na
			j.Machine, j.Model = machine, model
		}
		jobs = append(jobs, j)
	}
	batch := &store.Batch{
		ID: batchID, CreatedAt: now, Kind: "schedule",
		ItemCount: len(pinned), Source: source,
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
	logx.Event("info", "batch_submitted", map[string]any{
		"batch_id": batchID, "items": len(pinned), "source": "schedule",
	})
	return &BatchSubmitResult{ID: batchID, JobIDs: ids}, nil
}

// SubmitScheduleJob submits one pinned batch-lane job for a schedule.
// A template default check is honoured, the same as a batch item.
func (s *Service) SubmitScheduleJob(ctx context.Context, bodyJSON, machine, model string) (string, error) {
	j, err := s.prepareCheckedJob(bodyJSON, "", machine, model)
	if err != nil {
		return "", err
	}
	j.Lane = store.LaneBatch
	if err := s.store.InsertJob(ctx, j); err != nil {
		return "", err
	}
	go s.runLoop(j.ID)
	return j.ID, nil
}

func ensureLaneBatch(body string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", err
	}
	m["lane"] = store.LaneBatch
	delete(m, "fast")
	delete(m, "profile")
	b, err := json.Marshal(m)
	return string(b), err
}

// RunStateForScheduleRun derives display state for a schedule run row.
func (s *Service) RunStateForScheduleRun(ctx context.Context, run *store.ScheduleRun) string {
	if run.JobID != "" {
		j, err := s.store.GetJob(ctx, run.JobID)
		if err != nil {
			return run.Status
		}
		return string(j.Status)
	}
	if run.BatchID != "" {
		return s.batchDerivedState(ctx, run.BatchID)
	}
	return run.Status
}

func (s *Service) batchDerivedState(ctx context.Context, batchID string) string {
	counts, err := s.store.BatchCounts(ctx, batchID)
	if err != nil {
		return "running"
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		return "running"
	}
	pending, err := s.store.BatchPendingWithChecks(ctx, batchID)
	if err != nil || pending > 0 {
		return "running"
	}
	if counts[string(store.StatusFailed)] > 0 || counts[string(store.StatusCancelled)] > 0 {
		return "failed"
	}
	return "done"
}

// LastSubmittedRunActive reports whether the last submitted run is still in flight.
func (s *Service) LastSubmittedRunActive(ctx context.Context, scheduleID string) (bool, error) {
	run, err := s.store.LastSubmittedScheduleRun(ctx, scheduleID)
	if err != nil {
		if err == store.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	if run.Status != "submitted" {
		return false, nil
	}
	if run.JobID != "" {
		j, err := s.store.GetJob(ctx, run.JobID)
		if err != nil {
			return false, nil
		}
		switch j.Status {
		case store.StatusDone, store.StatusFailed, store.StatusCancelled:
			if j.CheckPending == 1 {
				return true, nil
			}
			children, err := s.store.ChildrenOf(ctx, j.ID)
			if err != nil {
				return false, err
			}
			for _, c := range children {
				switch c.Status {
				case store.StatusDone, store.StatusFailed, store.StatusCancelled:
				default:
					return true, nil
				}
			}
			return false, nil
		default:
			return true, nil
		}
	}
	if run.BatchID != "" {
		n, err := s.store.BatchPendingWithChecks(ctx, run.BatchID)
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
	return false, nil
}
