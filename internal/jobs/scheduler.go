package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/schedules"
	"github.com/azamasad/pingpong/internal/store"
)

// Scheduler runs due schedules with one timer goroutine.
type Scheduler struct {
	svc  *Service
	loc  *time.Location
	mu   sync.Mutex
	wake chan struct{}
}

func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{
		svc: svc, loc: time.Local,
		wake: make(chan struct{}, 1),
	}
}

func (sc *Scheduler) Notify() {
	select {
	case sc.wake <- struct{}{}:
	default:
	}
}

func (sc *Scheduler) Start(ctx context.Context) {
	go sc.loop(ctx)
}

func (sc *Scheduler) loop(ctx context.Context) {
	sc.handleStartupMissed(ctx)
	for {
		sleep := sc.sleepUntil(ctx)
		if sleep < 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-sc.svc.clk.NewTimer(sleep).C:
			sc.fireDue(ctx, sc.svc.clk.Now())
		case <-sc.wake:
		}
	}
}

func (sc *Scheduler) sleepUntil(ctx context.Context) time.Duration {
	now := sc.svc.clk.Now()
	next := sc.earliestNextDue(ctx, now)
	if next.IsZero() {
		return time.Minute
	}
	d := next.Sub(now)
	if d < time.Minute {
		d = time.Minute
	}
	return d
}

func (sc *Scheduler) earliestNextDue(ctx context.Context, now time.Time) time.Time {
	list, err := sc.svc.store.ListSchedules(ctx)
	if err != nil {
		return time.Time{}
	}
	var earliest time.Time
	for _, sch := range list {
		if !sch.Enabled {
			continue
		}
		nd := sc.scheduleNextDue(sch, now)
		if nd.IsZero() {
			continue
		}
		if earliest.IsZero() || nd.Before(earliest) {
			earliest = nd
		}
	}
	return earliest
}

func (sc *Scheduler) scheduleNextDue(sch *store.Schedule, now time.Time) time.Time {
	rule, err := schedules.ParseRuleJSON(sch.RuleJSON)
	if err != nil {
		return time.Time{}
	}
	ref := sch.CreatedAt
	if sch.LastDue != nil {
		ref = *sch.LastDue
	}
	return rule.NextDueAfter(ref, sc.loc)
}

func (sc *Scheduler) refTime(sch *store.Schedule) time.Time {
	if sch.LastDue != nil {
		return *sch.LastDue
	}
	return sch.CreatedAt
}

func (sc *Scheduler) handleStartupMissed(ctx context.Context) {
	now := sc.svc.clk.Now()
	list, _ := sc.svc.store.ListSchedules(ctx)
	for _, sch := range list {
		if !sch.Enabled {
			continue
		}
		sc.recoverMissed(ctx, sch, now)
	}
}

func (sc *Scheduler) recoverMissed(ctx context.Context, sch *store.Schedule, now time.Time) {
	rule, err := schedules.ParseRuleJSON(sch.RuleJSON)
	if err != nil {
		return
	}
	ref := sc.refTime(sch)
	past := rule.DueBetween(ref, now, sc.loc)
	if len(past) == 0 {
		return
	}
	latest := past[len(past)-1]
	grace := sc.svc.cfg.Schedules.Grace.Duration
	if sch.Missed == "run_once" && now.Sub(latest) <= grace {
		_, _ = sc.executeDue(ctx, sch, latest, false)
		return
	}
	sc.recordMissed(ctx, sch, latest, now)
}

func (sc *Scheduler) recordMissed(ctx context.Context, sch *store.Schedule, due, now time.Time) {
	run := &store.ScheduleRun{
		ScheduleID: sch.ID, Due: due, Started: now.UTC(),
		Status: "skipped_missed",
	}
	err := sc.svc.store.WithTx(ctx, func(tx *sql.Tx) error {
		if err := sc.svc.store.InsertScheduleRunTx(ctx, tx, run); err != nil {
			return err
		}
		return sc.svc.store.UpdateScheduleLastDueTx(ctx, tx, sch.ID, due, now.UTC())
	})
	if err != nil {
		return
	}
	sc.logRun(sch.Name, "skipped_missed", "", "", "")
	sc.Notify()
}

func (sc *Scheduler) fireDue(ctx context.Context, now time.Time) {
	list, _ := sc.svc.store.ListSchedules(ctx)
	for _, sch := range list {
		if !sch.Enabled {
			continue
		}
		nd := sc.scheduleNextDue(sch, now)
		if nd.IsZero() || nd.After(now) {
			continue
		}
		_, _ = sc.executeDue(ctx, sch, nd, false)
	}
}

// RunNow triggers a schedule immediately (no last_due change).
func (sc *Scheduler) RunNow(ctx context.Context, id string) (*store.ScheduleRun, error) {
	sch, err := sc.svc.store.GetSchedule(ctx, id)
	if err != nil {
		return nil, err
	}
	now := sc.svc.clk.Now()
	return sc.executeDue(ctx, sch, now, true)
}

func (sc *Scheduler) executeDue(ctx context.Context, sch *store.Schedule, due time.Time, manual bool) (*store.ScheduleRun, error) {
	now := sc.svc.clk.Now().UTC()
	if !manual && sch.Overlap == "skip" {
		active, _ := sc.svc.LastSubmittedRunActive(ctx, sch.ID)
		if active {
			run := &store.ScheduleRun{
				ScheduleID: sch.ID, Due: due, Started: now,
				Status: "skipped_overlap",
			}
			_ = sc.svc.store.WithTx(ctx, func(tx *sql.Tx) error {
				if err := sc.svc.store.InsertScheduleRunTx(ctx, tx, run); err != nil {
					return err
				}
				return sc.svc.store.UpdateScheduleLastDueTx(ctx, tx, sch.ID, due, now)
			})
			sc.logRun(sch.Name, "skipped_overlap", "", "", "")
			sc.Notify()
			return run, nil
		}
	}
	run := &store.ScheduleRun{
		ScheduleID: sch.ID, Due: due, Started: now,
		Status: "submitted",
	}
	err := sc.svc.store.WithTx(ctx, func(tx *sql.Tx) error {
		if err := sc.svc.store.InsertScheduleRunTx(ctx, tx, run); err != nil {
			return err
		}
		if !manual {
			return sc.svc.store.UpdateScheduleLastDueTx(ctx, tx, sch.ID, due, now)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	outcome, jobID, batchID, errCode := sc.submitSchedule(ctx, sch)
	run.Status = outcome
	run.ErrorCode = errCode
	run.JobID, run.BatchID = jobID, batchID
	if jobID != "" {
		run.Kind = "job"
	}
	if batchID != "" {
		run.Kind = "batch"
	}
	_ = sc.svc.store.UpdateScheduleRun(ctx, run)
	sc.logRun(sch.Name, outcome, errCode, jobID, batchID)
	sc.Notify()
	return run, nil
}

func (sc *Scheduler) submitSchedule(ctx context.Context, sch *store.Schedule) (status, jobID, batchID, errCode string) {
	bodies, err := sc.svc.expandScheduleBodies(ctx, sch)
	if err != nil {
		var te *TemplateRejectError
		if ok := asTemplateReject(err, &te); ok {
			return "failed", "", "", te.Code
		}
		var re *RouteRejectedError
		if ok := asRouteReject(err, &re); ok {
			if retryableCode(re.Code) {
				// Should not happen from expansion
				return "failed", "", "", re.Code
			}
			return "failed", "", "", re.Code
		}
		return "failed", "", "", "INTERNAL"
	}
	if sc.hasRows(sch) {
		res, err := sc.svc.SubmitScheduleBatch(ctx, bodies, sch.ID, sch.Machine, sch.Model)
		if err != nil {
			var re *RouteRejectedError
			if ok := asRouteReject(err, &re); ok {
				return "failed", "", "", re.Code
			}
			return "failed", "", "", "INTERNAL"
		}
		return "submitted", "", res.ID, ""
	}
	id, err := sc.svc.SubmitScheduleJob(ctx, bodies[0], sch.Machine, sch.Model)
	if err != nil {
		var re *RouteRejectedError
		if ok := asRouteReject(err, &re); ok {
			if retryableCode(re.Code) {
				return "submitted", id, "", ""
			}
			return "failed", "", "", re.Code
		}
		return "failed", "", "", "INTERNAL"
	}
	return "submitted", id, "", ""
}

func (sc *Scheduler) hasRows(sch *store.Schedule) bool {
	return sch.RowsJSON != "" && sch.RowsJSON != "[]" && sch.RowsJSON != "null"
}

func (sc *Scheduler) logRun(name, outcome, errCode, jobID, batchID string) {
	fields := map[string]any{
		"schedule": name, "outcome": outcome,
	}
	if errCode != "" {
		fields["error_code"] = errCode
	}
	if batchID != "" {
		fields["batch_id"] = batchID
	}
	if jobID != "" {
		fields["request_id"] = jobID
	}
	logx.Event("info", "schedule_run", fields)
}

func asTemplateReject(err error, te **TemplateRejectError) bool {
	if e, ok := err.(*TemplateRejectError); ok {
		*te = e
		return true
	}
	return false
}

func asRouteReject(err error, re **RouteRejectedError) bool {
	if e, ok := err.(*RouteRejectedError); ok {
		*re = e
		return true
	}
	return false
}

func (s *Service) expandScheduleBodies(ctx context.Context, sch *store.Schedule) ([]string, error) {
	if sch.Template != "" {
		var vars map[string]string
		if sch.VarsJSON != "" {
			var vm map[string]any
			_ = json.Unmarshal([]byte(sch.VarsJSON), &vm)
			vars, _ = parseStringVars(vm)
		}
		if sc := sch; sc.RowsJSON != "" && sc.RowsJSON != "[]" {
			var rowsRaw []json.RawMessage
			if err := json.Unmarshal([]byte(sch.RowsJSON), &rowsRaw); err != nil {
				return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
			}
			rows := make([]map[string]string, 0, len(rowsRaw))
			for _, r := range rowsRaw {
				var m map[string]any
				_ = json.Unmarshal(r, &m)
				row := make(map[string]string, len(m))
				for k, v := range m {
					row[k], _ = v.(string)
				}
				rows = append(rows, row)
			}
			b, _, err := s.ExpandTemplateBatch(ctx, TemplateBatchInput{
				Template: sch.Template, Vars: vars, Rows: rows,
				Overlay: map[string]any{"lane": store.LaneBatch},
			})
			return b, err
		}
		t, err := s.loadTemplate(ctx, sch.Template)
		if err != nil {
			return nil, err
		}
		body, _, err := s.expandOneTemplate(t, sch.Template, vars, map[string]any{"lane": store.LaneBatch})
		if err != nil {
			return nil, err
		}
		return []string{body}, nil
	}
	body, err := pinBodyJSON(schedulePromptBody(sch), sch.Machine, sch.Model)
	if err != nil {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	lb, err := ensureLaneBatch(body)
	return []string{lb}, err
}

func schedulePromptBody(sch *store.Schedule) string {
	b, _ := json.Marshal(map[string]any{"prompt": sch.Prompt, "lane": store.LaneBatch})
	return string(b)
}
