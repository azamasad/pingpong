package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azamasad/pingpong/internal/clock"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/registry"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/google/uuid"
)

type RouteRejectedError struct{ Code string }

func (e *RouteRejectedError) Error() string { return e.Code }

type Service struct {
	cfg   *config.Config
	store *store.Store
	reg   *registry.Registry
	gate  *gate.Gate
	clk   clock.Clock

	mu           sync.Mutex
	cancels      map[string]context.CancelFunc
	holdMu       sync.Mutex
	holding      map[string]int
	shuttingDown atomic.Bool
}

func New(cfg *config.Config, st *store.Store, reg *registry.Registry, g *gate.Gate, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.Real{}
	}
	return &Service{
		cfg: cfg, store: st, reg: reg, gate: g, clk: clk,
		cancels: map[string]context.CancelFunc{}, holding: map[string]int{},
	}
}

func (s *Service) Store() *store.Store { return s.store }

func (s *Service) List(ctx context.Context, status, lane, batchID string, before *time.Time, limit int) ([]*store.Job, error) {
	return s.store.ListJobs(ctx, status, lane, batchID, before, limit)
}

func (s *Service) Get(ctx context.Context, id string) (*store.Job, error) {
	return s.store.GetJob(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.DeleteJob(ctx, id)
}

func (s *Service) EventsSince(ctx context.Context, id string, fromSeq int64) ([]store.Event, error) {
	return s.store.EventsSince(ctx, id, fromSeq)
}

func (s *Service) resolve(req route.Request) route.Result {
	snap := s.reg.RouteSnapshot()
	res := route.Decide(req, snap)
	if res.NeedRefresh && s.cfg.Health.RefreshOnMiss() {
		ctx := context.Background()
		for _, m := range s.cfg.Machines {
			s.reg.RefreshMachine(ctx, m.Name)
		}
		snap = s.reg.RouteSnapshotRefreshed()
		res = route.Decide(req, snap)
	}
	if res.NeedRefresh {
		snap.RefreshedModels = true
		res = route.Decide(req, snap)
	}
	return res
}

func retryableCode(code string) bool {
	switch code {
	case route.CodeModelsUnknown, route.CodeMachineDown, "MACHINE_FAILED", "TIMEOUT", "UPSTREAM_ERROR", "ROUTER_RESTARTING":
		return true
	default:
		return false
	}
}

func (s *Service) Submit(ctx context.Context, bodyJSON string) (string, error) {
	return s.submit(ctx, bodyJSON, "")
}

func (s *Service) SubmitBatch(ctx context.Context, bodies []string) (string, []string, error) {
	if len(bodies) == 0 {
		return "", nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	batchID := uuid.NewString()
	ids := make([]string, 0, len(bodies))
	for _, body := range bodies {
		id, err := s.submit(ctx, body, batchID)
		if err != nil {
			return batchID, ids, err
		}
		ids = append(ids, id)
	}
	return batchID, ids, nil
}

func (s *Service) submit(ctx context.Context, bodyJSON, batchID string) (string, error) {
	cb, _, err := parseChatBody(bodyJSON)
	if err != nil {
		if err.Error() == "BAD_REQUEST" {
			return "", &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		return "", err
	}
	fast, model, machine, err := s.cfg.ApplyProfile(cb.Profile, cb.Fast, cb.Model, cb.Machine)
	if err != nil {
		return "", &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	cb.Fast, cb.Model, cb.Machine = fast, model, machine
	cb.Model, cb.Machine, _ = s.cfg.ResolveAlias(cb.Model, cb.Machine)
	routeReq := route.Request{Fast: cb.Fast, Model: cb.Model, Machine: cb.Machine}
	res := s.resolve(routeReq)
	if !res.OK && !retryableCode(res.Code) {
		return "", &RouteRejectedError{Code: res.Code}
	}
	if batchID != "" {
		cb.Lane = store.LaneBatch
	}
	lane := cb.Lane
	if lane == "" {
		lane = store.LaneInteractive
	}
	if lane != store.LaneInteractive && lane != store.LaneBatch {
		return "", &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	now := s.clk.Now().UTC()
	id := uuid.NewString()
	j := &store.Job{
		ID: id, CreatedAt: now, UpdatedAt: now,
		RequestJSON: bodyJSON,
		Status:      store.StatusWaiting,
		Lane:        lane,
		BatchID:     batchID,
	}
	if batchID != "" {
		j.Lane = store.LaneBatch
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
	if err := s.store.InsertJob(ctx, j); err != nil {
		return "", err
	}
	go s.runLoop(id)
	return id, nil
}

func (s *Service) runLoop(id string) {
	for {
		j, err := s.store.GetJob(context.Background(), id)
		if err != nil {
			return
		}
		switch j.Status {
		case store.StatusDone, store.StatusFailed, store.StatusCancelled:
			return
		case store.StatusRetrying:
			if j.NextAttemptAt != nil && s.clk.Now().Before(*j.NextAttemptAt) {
				wait := j.NextAttemptAt.Sub(s.clk.Now())
				if wait > 0 {
					timer := s.clk.NewTimer(wait)
					<-timer.C
				}
				continue
			}
			j.Status = store.StatusWaiting
			j.UpdatedAt = s.clk.Now().UTC()
			_ = s.store.UpdateJob(context.Background(), j)
		case store.StatusWaiting, store.StatusRunning:
			s.execute(id)
			return
		default:
			return
		}
	}
}

func (s *Service) execute(id string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, id)
		s.mu.Unlock()
		cancel()
	}()

	j, err := s.store.GetJob(ctx, id)
	if err != nil {
		return
	}
	cb, msgs, err := parseChatBody(j.RequestJSON)
	if err != nil {
		j.Status = store.StatusFailed
		j.ErrorCode = "BAD_REQUEST"
		j.UpdatedAt = s.clk.Now().UTC()
		_ = s.store.UpdateJob(ctx, j)
		return
	}
	fast, model, machine, err := s.cfg.ApplyProfile(cb.Profile, cb.Fast, cb.Model, cb.Machine)
	if err != nil {
		j.Status = store.StatusFailed
		j.ErrorCode = "BAD_REQUEST"
		j.UpdatedAt = s.clk.Now().UTC()
		_ = s.store.UpdateJob(ctx, j)
		return
	}
	cb.Fast, cb.Model, cb.Machine = fast, model, machine
	cb.Model, cb.Machine, _ = s.cfg.ResolveAlias(cb.Model, cb.Machine)
	routeReq := route.Request{Fast: cb.Fast, Model: cb.Model, Machine: cb.Machine}
	res := s.resolve(routeReq)
	now := s.clk.Now().UTC()
	if !res.OK {
		if retryableCode(res.Code) {
			s.markRetry(j, res.Code, "routing failed", now)
		} else {
			j.Status = store.StatusFailed
			j.ErrorCode = res.Code
			j.UpdatedAt = now
			_ = s.store.UpdateJob(ctx, j)
		}
		return
	}
	j.Machine, j.Model, j.Mode = res.Machine, res.Model, res.Mode
	j.UpdatedAt = now
	j.Status = store.StatusWaiting
	_ = s.store.UpdateJob(ctx, j)
	if err := s.waitTurn(ctx, id, res.Machine); err != nil {
		if errors.Is(err, context.Canceled) && s.shuttingDown.Load() {
			flushPartial := func() {
				j.UpdatedAt = s.clk.Now().UTC()
				_ = s.store.UpdateJob(ctx, j)
			}
			flushPartial()
			s.markRetry(j, "ROUTER_RESTARTING", "router shutting down", s.clk.Now().UTC())
		} else {
			j.Status = store.StatusCancelled
			j.UpdatedAt = s.clk.Now().UTC()
			_ = s.store.UpdateJob(ctx, j)
			s.emit(ctx, id, "status", map[string]any{"state": "cancelled"})
		}
		return
	}
	defer s.releaseTurn(res.Machine)

	j.Status = store.StatusRunning
	j.Attempt++
	j.UpdatedAt = s.clk.Now().UTC()
	j.OutputText, j.OutputThinking = "", ""
	_ = s.store.UpdateJob(ctx, j)
	routeEv := map[string]any{"machine": res.Machine, "model": res.Model, "mode": res.Mode}
	if alias := aliasForRequest(s.cfg, j.RequestJSON); alias != "" {
		routeEv["alias"] = alias
	}
	s.emit(ctx, id, "route", routeEv)
	s.emit(ctx, id, "status", map[string]any{"state": "running"})

	if err := s.gate.Acquire(ctx, res.Machine, func(pos int) {
		s.emit(ctx, id, "status", map[string]any{"state": "waiting", "position": pos})
	}, func() bool { return s.reg.IsDown(res.Machine) }); err != nil {
		if errors.Is(err, context.Canceled) {
			if s.shuttingDown.Load() {
				s.markRetry(j, "ROUTER_RESTARTING", "router shutting down", s.clk.Now().UTC())
			} else {
				j.Status = store.StatusCancelled
				j.UpdatedAt = s.clk.Now().UTC()
				_ = s.store.UpdateJob(ctx, j)
				s.emit(ctx, id, "status", map[string]any{"state": "cancelled"})
			}
		} else {
			s.markRetry(j, route.CodeMachineDown, "machine down", s.clk.Now().UTC())
		}
		return
	}
	defer s.gate.Release(res.Machine)

	mc, _ := s.cfg.MachineByName(res.Machine)
	cli := ollama.NewClient(mc.Address, s.cfg.Timeouts.ConnectTimeout.Duration)
	opt := filterOptions(cb.Options)
	if opt == nil {
		opt = map[string]any{}
	}
	opt["num_ctx"] = mc.NumCtx
	stream, err := cli.ChatStream(ctx, ollama.ChatRequest{
		Model: res.Model, Messages: msgs, Think: cb.Think, Options: opt,
	})
	if err != nil {
		if ctx.Err() != nil {
			s.finishStopped(j)
			return
		}
		code := route.CodeMachineDown
		if !connectErr(err) {
			code = "UPSTREAM_ERROR"
		} else {
			s.reg.MarkDownImmediate(res.Machine)
		}
		s.markRetry(j, code, err.Error(), s.clk.Now().UTC())
		return
	}
	defer stream.Close()

	start := s.clk.Now()
	gotFirst := false
	var lastChunk time.Time
	var buf, think strings.Builder
	firstTO := s.cfg.Timeouts.FirstTokenTimeout.Duration
	idleTO := s.cfg.Timeouts.IdleTokenTimeout.Duration
	maxRun := s.cfg.Timeouts.MaxRunTime.Duration
	flushEvery := s.cfg.Storage.OutputFlushInterval.Duration
	var lastFlush time.Time

	flushOutput := func() {
		j.OutputText = buf.String()
		j.OutputThinking = think.String()
		j.UpdatedAt = s.clk.Now().UTC()
		_ = s.store.UpdateJob(ctx, j)
		lastFlush = s.clk.Now()
	}

	for {
		select {
		case <-ctx.Done():
			j.OutputText = buf.String()
			j.OutputThinking = think.String()
			s.finishStopped(j)
			return
		default:
		}
		savePartial := func() {
			j.OutputText = buf.String()
			j.OutputThinking = think.String()
		}
		if s.clk.Now().Sub(start) > maxRun {
			savePartial()
			s.markRetry(j, "TIMEOUT", "max run time", s.clk.Now().UTC())
			return
		}
		if !gotFirst && s.clk.Now().Sub(start) > firstTO {
			savePartial()
			s.markRetry(j, "TIMEOUT", "first chunk timeout", s.clk.Now().UTC())
			return
		}
		if gotFirst && s.clk.Now().Sub(lastChunk) > idleTO {
			savePartial()
			s.markRetry(j, "TIMEOUT", "idle timeout", s.clk.Now().UTC())
			return
		}
		ch, ok, err := stream.Next()
		if err != nil {
			savePartial()
			if ctx.Err() != nil {
				s.finishStopped(j)
				return
			}
			code := "UPSTREAM_ERROR"
			if gotFirst {
				code = "MACHINE_FAILED"
			}
			s.markRetry(j, code, err.Error(), s.clk.Now().UTC())
			return
		}
		if !ok {
			if !gotFirst {
				savePartial()
				s.markRetry(j, "MACHINE_FAILED", "connection closed", s.clk.Now().UTC())
			} else {
				j.Status = store.StatusDone
				j.OutputText = buf.String()
				j.OutputThinking = think.String()
				j.UpdatedAt = s.clk.Now().UTC()
				_ = s.store.UpdateJob(ctx, j)
				s.emit(ctx, id, "done", map[string]any{"machine": res.Machine, "model": res.Model, "mode": res.Mode})
			}
			return
		}
		if !gotFirst && (ch.Message.Content != "" || ch.Message.Thinking != "") {
			gotFirst = true
			lastChunk = s.clk.Now()
		}
		if ch.Message.Thinking != "" {
			think.WriteString(ch.Message.Thinking)
			s.emit(ctx, id, "thinking", map[string]any{"text": ch.Message.Thinking})
			lastChunk = s.clk.Now()
		}
		if ch.Message.Content != "" {
			buf.WriteString(ch.Message.Content)
			s.emit(ctx, id, "token", map[string]any{"text": ch.Message.Content})
			lastChunk = s.clk.Now()
		}
		if flushEvery > 0 && (buf.Len() > 0 || think.Len() > 0) && s.clk.Now().Sub(lastFlush) >= flushEvery {
			flushOutput()
		}
		if ch.Done {
			if ctx.Err() != nil {
				s.finishStopped(j)
				return
			}
			j.Status = store.StatusDone
			j.OutputText = buf.String()
			j.OutputThinking = think.String()
			j.UpdatedAt = s.clk.Now().UTC()
			_ = s.store.UpdateJob(ctx, j)
			s.emit(ctx, id, "done", map[string]any{
				"machine": res.Machine, "model": res.Model, "mode": res.Mode,
			})
			return
		}
	}
}

func connectErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "dial")
}

func (s *Service) markRetry(j *store.Job, code, msg string, now time.Time) {
	ctx := context.Background()
	if j.FirstFailureAt == nil {
		j.FirstFailureAt = &now
	}
	if j.FirstFailureAt != nil && now.Sub(*j.FirstFailureAt) > s.cfg.Retry.RetryWindow.Duration {
		j.Status = store.StatusFailed
		j.ErrorCode = code
		j.ErrorMessage = msg
		j.UpdatedAt = now
		_ = s.store.UpdateJob(ctx, j)
		s.emit(ctx, j.ID, "error", map[string]any{"code": code, "message": msg, "retryable": false})
		return
	}
	na := now
	if code != "ROUTER_RESTARTING" {
		na = now.Add(s.cfg.Retry.RetryInterval.Duration)
	}
	j.Status = store.StatusRetrying
	j.ErrorCode = code
	j.ErrorMessage = msg
	j.NextAttemptAt = &na
	j.UpdatedAt = now
	_ = s.store.UpdateJob(ctx, j) // keeps partial output_text when set
	s.emit(ctx, j.ID, "status", map[string]any{
		"state": "retrying", "attempt": j.Attempt, "next_attempt_at": na.Format(time.RFC3339),
	})
	go s.runLoop(j.ID)
}

func (s *Service) emit(ctx context.Context, jobID, typ string, data any) {
	seq, err := s.store.NextEventSeq(ctx, jobID)
	if err != nil {
		return
	}
	_ = s.store.AppendEvent(ctx, jobID, seq, typ, data)
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	j, err := s.store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if j.Status == store.StatusDone || j.Status == store.StatusFailed {
		return fmt.Errorf("cannot cancel")
	}
	s.mu.Lock()
	if c, ok := s.cancels[id]; ok {
		c()
	}
	s.mu.Unlock()
	if j.Status != store.StatusCancelled {
		j.Status = store.StatusCancelled
		j.UpdatedAt = s.clk.Now().UTC()
		return s.store.UpdateJob(ctx, j)
	}
	return nil
}

// finishStopped records a user cancel, or a planned restart, when the job context ends.
// A user cancel must not be retried. A restart counts toward the retry window (D-071).
func (s *Service) finishStopped(j *store.Job) {
	if s.shuttingDown.Load() {
		s.markRetry(j, "ROUTER_RESTARTING", "router shutting down", s.clk.Now().UTC())
		return
	}
	j.Status = store.StatusCancelled
	j.UpdatedAt = s.clk.Now().UTC()
	ctx := context.Background()
	_ = s.store.UpdateJob(ctx, j)
	s.emit(ctx, j.ID, "status", map[string]any{"state": "cancelled"})
}

// BeginShutdown cancels running jobs so they can be marked retrying.
func (s *Service) BeginShutdown() {
	s.shuttingDown.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.cancels {
		c()
	}
}

func (s *Service) Start(ctx context.Context) {
	go s.retentionLoop(ctx)
	jobs, _ := s.store.ActiveJobs(ctx)
	for _, j := range jobs {
		if j.Status == store.StatusRunning {
			now := s.clk.Now().UTC()
			j.Status = store.StatusRetrying
			j.Attempt++
			if j.FirstFailureAt == nil {
				j.FirstFailureAt = &now
			}
			j.ErrorCode = "ROUTER_RESTARTING"
			j.ErrorMessage = "interrupted by restart"
			j.NextAttemptAt = &now
			j.UpdatedAt = now
			_ = s.store.UpdateJob(ctx, j)
		}
		go s.runLoop(j.ID)
	}
}

func (s *Service) retentionLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.Storage.RetentionSweepInterval.Duration)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			before := s.clk.Now().Add(-time.Duration(s.cfg.Storage.RetentionDays) * 24 * time.Hour)
			_, _ = s.store.PurgeOlderThan(ctx, before)
		}
	}
}
