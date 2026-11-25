package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/azamasad/pingpong/internal/templates"
	"github.com/google/uuid"
)

// ReduceSpec is the user-named pair and instruction for a file-batch reduce step.
type ReduceSpec struct {
	Instruction string `json:"instruction"`
	Machine     string `json:"machine"`
	Model       string `json:"model"`
}

func parseReduceSpec(raw string) (*ReduceSpec, error) {
	var spec ReduceSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if spec.Instruction == "" || spec.Machine == "" || spec.Model == "" {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	if !strings.Contains(spec.Instruction, "{{results}}") {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	return &spec, nil
}

func (s *Service) onBatchItemTerminal(ctx context.Context, j *store.Job) {
	if j.BatchID != "" && j.Stage != store.StageReduce {
		_ = s.maybeCreateReduce(ctx, j.BatchID)
	}
	if j.BatchID != "" {
		s.maybeNotifyBatchTerminal(ctx, j.BatchID)
	}
}

func (s *Service) recoverPendingReduces(ctx context.Context) {
	list, err := s.store.BatchesNeedingReduce(ctx)
	if err != nil {
		return
	}
	for _, b := range list {
		_ = s.maybeCreateReduce(ctx, b.ID)
	}
}

func (s *Service) maybeCreateReduce(ctx context.Context, batchID string) error {
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if batch.ReduceJSON == "" || batch.ReduceJobID != "" {
		return nil
	}
	if _, err := s.store.ReduceJobByBatch(ctx, batchID); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	terminal, done, anyDone, err := s.store.BatchChunkJobsTerminal(ctx, batchID)
	if err != nil || !terminal || !anyDone {
		return err
	}
	spec, err := parseReduceSpec(batch.ReduceJSON)
	if err != nil {
		return err
	}
	chunks, err := s.store.ListBatchChunkJobs(ctx, batchID)
	if err != nil {
		return err
	}
	var b strings.Builder
	for i, ch := range chunks {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "### Part %d\n", i+1)
		if ch.Status == store.StatusDone {
			b.WriteString(ch.OutputText)
		} else {
			fmt.Fprintf(&b, "[no result: %s]", ch.Status)
		}
	}
	prompt, missing, _ := templates.Substitute(spec.Instruction, map[string]string{"results": b.String()})
	if len(missing) > 0 {
		return &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	model := spec.Model
	machine := spec.Machine
	model, machine, _ = s.cfg.ResolveAlias(model, machine)
	if _, ok := s.cfg.MachineByName(machine); !ok {
		return &RouteRejectedError{Code: route.CodeMachineUnknown}
	}
	res := s.resolve(route.Request{Model: model, Machine: machine})
	now := s.clk.Now().UTC()
	childID := uuid.NewString()
	child := &store.Job{
		ID: childID, CreatedAt: now, UpdatedAt: now,
		BatchID: batchID, Stage: store.StageReduce, Lane: store.LaneBatch,
		RequestJSON: mustJSON(map[string]any{
			"prompt": prompt, "machine": machine, "model": model,
		}),
	}
	oversize := false
	if res.OK {
		mc, _ := s.cfg.MachineByName(res.Machine)
		est := len(prompt)/3 + 150
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
		child.ErrorMessage = "reduce prompt does not fit context"
	}
	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		if dup, err := s.store.ReduceJobByBatch(ctx, batchID); err == nil && dup != nil {
			return nil
		}
		b, err := s.store.GetBatch(ctx, batchID)
		if err != nil {
			return err
		}
		if b.ReduceJobID != "" {
			return nil
		}
		if err := s.store.InsertJobTx(ctx, tx, child); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE batches SET reduce_job_id=? WHERE id=? AND (reduce_job_id='' OR reduce_job_id IS NULL)`,
			childID, batchID)
		return err
	})
	if err != nil {
		return err
	}
	dup, _ := s.store.ReduceJobByBatch(ctx, batchID)
	if dup != nil && dup.ID != childID {
		return nil
	}
	logx.Event("info", "reduce_created", map[string]any{"batch_id": batchID, "items": done})
	if oversize || (child.Status == store.StatusFailed && child.ErrorCode != "") {
		logx.Event("warn", "check_failed", map[string]any{
			"request_id": batchID, "stage": store.StageReduce, "error_code": child.ErrorCode,
		})
	} else if child.Status == store.StatusWaiting || child.Status == store.StatusRetrying {
		go s.runLoop(childID)
	}
	return nil
}
