package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func New(cfg *config.Config, st *store.Store, reg *registry.Registry, g *gate.Gate, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.Real{}
	}
	return &Service{cfg: cfg, store: st, reg: reg, gate: g, clk: clk, cancels: map[string]context.CancelFunc{}}
}

func (s *Service) Store() *store.Store { return s.store }

func (s *Service) List(ctx context.Context, status string, limit int) ([]*store.Job, error) {
	return s.store.ListJobs(ctx, status, limit)
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
	if res.NeedRefresh && s.cfg.Health.RefreshOnModelMiss {
		ctx := context.Background()
		for _, m := range s.cfg.Machines {
			s.reg.RefreshMachine(ctx, m.Name)
		}
		snap = s.reg.RouteSnapshotRefreshed()
		res = route.Decide(req, snap)
	}
	if res.NeedRefresh {
		return route.Result{Code: route.CodeModelsUnknown}
	}
	return res
}

func retryableCode(code string) bool {
	switch code {
	case route.CodeModelsUnknown, route.CodeMachineDown, "MACHINE_FAILED", "TIMEOUT", "UPSTREAM_ERROR":
		return true
	default:
		return false
	}
}

func (s *Service) Submit(ctx context.Context, bodyJSON string) (string, error) {
	cb, _, err := parseChatBody(bodyJSON)
	if err != nil {
		if err.Error() == "BAD_REQUEST" {
			return "", &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		return "", err
	}
	routeReq := route.Request{Fast: cb.Fast, Model: cb.Model, Machine: cb.Machine}
	res := s.resolve(routeReq)
	if !res.OK && !retryableCode(res.Code) {
		return "", &RouteRejectedError{Code: res.Code}
	}
	now := s.clk.Now().UTC()
	id := uuid.NewString()
	j := &store.Job{
		ID: id, CreatedAt: now, UpdatedAt: now,
		RequestJSON: bodyJSON,
		Status:      store.StatusWaiting,
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
				wait := time.Until(*j.NextAttemptAt)
				if wait > 0 {
					timer := time.NewTimer(wait)
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
	j.Status = store.StatusRunning
	j.Machine, j.Model, j.Mode = res.Machine, res.Model, res.Mode
	j.Attempt++
	j.UpdatedAt = now
	j.OutputText, j.OutputThinking = "", ""
	_ = s.store.UpdateJob(ctx, j)
	s.emit(ctx, id, "status", map[string]any{"state": "running"})

	if err := s.gate.Acquire(ctx, res.Machine, nil, func() bool { return s.reg.IsDown(res.Machine) }); err != nil {
		if errors.Is(err, context.Canceled) {
			j.Status = store.StatusCancelled
			j.UpdatedAt = s.clk.Now().UTC()
			_ = s.store.UpdateJob(ctx, j)
			s.emit(ctx, id, "status", map[string]any{"state": "cancelled"})
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

	for {
		select {
		case <-ctx.Done():
			j.Status = store.StatusCancelled
			j.UpdatedAt = s.clk.Now().UTC()
			_ = s.store.UpdateJob(ctx, j)
			return
		default:
		}
		if s.clk.Now().Sub(start) > maxRun {
			s.markRetry(j, "TIMEOUT", "max run time", s.clk.Now().UTC())
			return
		}
		if !gotFirst && s.clk.Now().Sub(start) > firstTO {
			s.markRetry(j, "TIMEOUT", "first chunk timeout", s.clk.Now().UTC())
			return
		}
		if gotFirst && s.clk.Now().Sub(lastChunk) > idleTO {
			s.markRetry(j, "TIMEOUT", "idle timeout", s.clk.Now().UTC())
			return
		}
		ch, ok, err := stream.Next()
		if err != nil {
			code := "UPSTREAM_ERROR"
			if gotFirst {
				code = "MACHINE_FAILED"
			}
			s.markRetry(j, code, err.Error(), s.clk.Now().UTC())
			return
		}
		if !ok {
			if !gotFirst {
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
		if ch.Done {
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
	na := now.Add(s.cfg.Retry.RetryInterval.Duration)
	j.Status = store.StatusRetrying
	j.ErrorCode = code
	j.ErrorMessage = msg
	j.NextAttemptAt = &na
	j.UpdatedAt = now
	_ = s.store.UpdateJob(ctx, j)
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

func (s *Service) Start(ctx context.Context) {
	go s.retentionLoop(ctx)
	jobs, _ := s.store.ActiveJobs(ctx)
	for _, j := range jobs {
		if j.Status == store.StatusRunning {
			j.Status = store.StatusRetrying
			now := s.clk.Now().UTC()
			j.NextAttemptAt = &now
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
