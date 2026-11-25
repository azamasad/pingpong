package jobs

import (
	"context"
	"fmt"

	"github.com/azamasad/pingpong/internal/notify"
	"github.com/azamasad/pingpong/internal/store"
)

func id8(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func jobFailedNotifyEligible(j *store.Job) bool {
	if j.Stage != "" && j.Stage != store.StageAnswer {
		return true
	}
	return j.BatchID == ""
}

func (s *Service) maybeNotifyJobFailed(j *store.Job, code string) {
	if code == "" {
		code = j.ErrorCode
	}
	if code == "" {
		code = "FAILED"
	}
	if s.notify != nil && s.notify.Enabled() && jobFailedNotifyEligible(j) {
		body := fmt.Sprintf("Job %s failed (%s)", id8(j.ID), code)
		s.notify.Send(notify.EventJobFailed, body)
	}
	if j == nil || j.BatchID != "" || j.ID == "" {
		return
	}
	name, err := s.store.ScheduleNameByJobID(context.Background(), j.ID)
	if err != nil || name == "" {
		return
	}
	s.notifyScheduleFailed(name, code)
}

func (s *Service) notifyScheduleFailed(name, code string) {
	if s.notify == nil || !s.notify.Enabled() || name == "" {
		return
	}
	if code == "" {
		code = "FAILED"
	}
	body := fmt.Sprintf("Schedule %s run failed (%s)", name, code)
	s.notify.Send(notify.EventScheduleFailed, body)
}

func (s *Service) maybeNotifyBatchTerminal(ctx context.Context, batchID string) {
	if batchID == "" {
		return
	}
	pending, err := s.store.BatchPendingWithChecks(ctx, batchID)
	if err != nil || pending > 0 {
		return
	}
	s.batchNotifyMu.Lock()
	if _, ok := s.batchNotified[batchID]; ok {
		s.batchNotifyMu.Unlock()
		return
	}
	s.batchNotified[batchID] = struct{}{}
	s.batchNotifyMu.Unlock()

	counts, err := s.store.BatchCounts(ctx, batchID)
	if err != nil {
		return
	}
	done := counts[string(store.StatusDone)]
	failed := counts[string(store.StatusFailed)] + counts[string(store.StatusCancelled)]

	if s.notify != nil && s.notify.Enabled() {
		body := fmt.Sprintf("Batch %s finished: %d done, %d failed", id8(batchID), done, failed)
		s.notify.Send(notify.EventBatchDone, body)
	}

	if failed == 0 {
		return
	}
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil || batch.Kind != "schedule" || batch.Source == "" {
		return
	}
	sch, err := s.store.GetSchedule(ctx, batch.Source)
	if err != nil {
		return
	}
	s.notifyScheduleFailed(sch.Name, "BATCH_FAILED")
}
