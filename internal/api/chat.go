package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/route"
)

func routeMessages(code string) string {
	switch code {
	case route.CodeMachineDown:
		return "machine is down or unreachable"
	case route.CodeModelNotInstalled:
		return "model is not installed on the target machine"
	case route.CodeModelAmbiguous:
		return "model is installed on several machines; pin a machine"
	case route.CodeModelsUnknown:
		return "model status is unknown; try again"
	case route.CodeMachineUnknown:
		return "unknown machine"
	case route.CodeNoTarget:
		return "no machine for this request"
	default:
		return code
	}
}

func (s *Server) resolveRoute(ctx context.Context, req route.Request) route.Result {
	snap := s.Reg.RouteSnapshot()
	res := route.Decide(req, snap)
	if res.NeedRefresh && s.Cfg.Health.RefreshOnModelMiss {
		for _, m := range s.Cfg.Machines {
			s.Reg.RefreshMachine(ctx, m.Name)
		}
		snap = s.Reg.RouteSnapshotRefreshed()
		res = route.Decide(req, snap)
	}
	return res
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	max := s.Cfg.Router.MaxBodyBytes
	body, err := readBodyLimited(r, max)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	if int64(len(body)) > max {
		writeErr(w, apperrors.New("BODY_TOO_LARGE", "body too large"))
		return
	}
	var cb ChatBody
	if err := json.Unmarshal(body, &cb); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	msgs, err := cb.messages()
	if err != nil {
		writeErr(w, err.(apperrors.APIError))
		return
	}
	routeReq := route.Request{Fast: cb.Fast, Model: cb.Model, Machine: cb.Machine}
	res := s.resolveRoute(r.Context(), routeReq)
	if !res.OK {
		ae := apperrors.New(res.Code, routeMessages(res.Code))
		logx.Event("warn", "route_error", map[string]any{
			"error_code": res.Code, "message": ae.Message, "machine": res.Machine, "model": res.Model,
		})
		writeErr(w, ae)
		return
	}
	if !cb.streaming() {
		s.chatNonStream(w, r, cb, msgs, res)
		return
	}
	s.chatStream(w, r, cb, msgs, res)
}

type chunkRes struct {
	ch  ollama.StreamChunk
	ok  bool
	err error
}

func (s *Server) chatStream(w http.ResponseWriter, r *http.Request, cb ChatBody, msgs []ollama.ChatMessage, res route.Result) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_ = writeLine(w, map[string]any{
		"type": "route", "machine": res.Machine, "model": res.Model, "mode": res.Mode,
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	waitStart := time.Now()
	lastPos := 0
	err := s.Gate.Acquire(ctx, res.Machine, func(pos int) {
		if pos != lastPos {
			lastPos = pos
			_ = writeLine(w, map[string]any{"type": "status", "state": "waiting", "position": pos})
		}
	}, func() bool { return s.Reg.IsDown(res.Machine) })
	if err != nil {
		if errors.Is(err, gate.ErrMachineDown) {
			_ = writeLine(w, map[string]any{
				"type": "error", "code": "MACHINE_DOWN",
				"message": "machine went down while waiting", "retryable": true,
			})
		}
		return
	}
	defer s.Gate.Release(res.Machine)
	queueWait := time.Since(waitStart).Milliseconds()

	_ = writeLine(w, map[string]any{"type": "status", "state": "running"})

	mc, _ := s.Cfg.MachineByName(res.Machine)
	cli := ollama.NewClient(mc.Address, s.Cfg.Timeouts.ConnectTimeout.Duration)
	opt := filterOptions(cb.Options)
	if opt == nil {
		opt = map[string]any{}
	}
	opt["num_ctx"] = mc.NumCtx

	stream, err := cli.ChatStream(ctx, ollama.ChatRequest{
		Model: res.Model, Messages: msgs, Think: cb.Think, Options: opt,
	})
	if err != nil {
		s.writeStreamErr(w, err, res.Machine, true)
		return
	}
	defer stream.Close()

	var writeMu sync.Mutex
	writeSafe := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return writeLine(w, v)
	}

	start := time.Now()
	gotFirst := false
	var lastChunk time.Time
	var doneChunk ollama.StreamChunk
	firstTO := s.Cfg.Timeouts.FirstTokenTimeout.Duration
	idleTO := s.Cfg.Timeouts.IdleTokenTimeout.Duration
	maxRun := s.Cfg.Timeouts.MaxRunTime.Duration
	heartbeat := s.Cfg.Timeouts.HeartbeatInterval.Duration

	hbDone := make(chan struct{})
	go func() {
		t := time.NewTicker(heartbeat)
		defer t.Stop()
		for {
			select {
			case <-hbDone:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				_ = writeSafe(map[string]any{
					"type": "ping", "elapsed_ms": time.Since(start).Milliseconds(),
				})
			}
		}
	}()
	defer close(hbDone)

	chunks := make(chan chunkRes, 1)
	go func() {
		defer close(chunks)
		for {
			ch, ok, e := stream.Next()
			select {
			case <-ctx.Done():
				return
			case chunks <- chunkRes{ch, ok, e}:
			}
			if !ok || e != nil {
				return
			}
			if ch.Done {
				return
			}
		}
	}()

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

streamLoop:
	for {
		select {
		case <-ctx.Done():
			return
		case cr, open := <-chunks:
			if !open && !gotFirst {
				_ = writeSafe(map[string]any{"type": "error", "code": "MACHINE_FAILED", "message": "connection closed", "retryable": true})
				return
			}
			if !open {
				break streamLoop
			}
			if cr.err != nil {
				code := "UPSTREAM_ERROR"
				if gotFirst {
					code = "MACHINE_FAILED"
				}
				_ = writeSafe(map[string]any{"type": "error", "code": code, "message": safeUpstreamMsg(cr.err), "retryable": true})
				return
			}
			if !cr.ok {
				break streamLoop
			}
			if !gotFirst {
				gotFirst = true
				lastChunk = time.Now()
			}
			if cr.ch.Message.Thinking != "" {
				_ = writeSafe(map[string]any{"type": "thinking", "text": cr.ch.Message.Thinking})
			}
			if cr.ch.Message.Content != "" {
				_ = writeSafe(map[string]any{"type": "token", "text": cr.ch.Message.Content})
			}
			if cr.ch.Done {
				doneChunk = cr.ch
				break streamLoop
			}
			lastChunk = time.Now()
		case <-tick.C:
			if time.Since(start) > maxRun {
				cancel()
				stream.Close()
				_ = writeSafe(map[string]any{"type": "error", "code": "TIMEOUT", "message": "max run time", "retryable": true})
				return
			}
			if !gotFirst && time.Since(start) > firstTO {
				cancel()
				stream.Close()
				_ = writeSafe(map[string]any{"type": "error", "code": "TIMEOUT", "message": "first chunk timeout", "retryable": true})
				return
			}
			if gotFirst && time.Since(lastChunk) > idleTO {
				cancel()
				stream.Close()
				_ = writeSafe(map[string]any{"type": "error", "code": "TIMEOUT", "message": "idle timeout", "retryable": true})
				return
			}
		}
	}

	stats := statsFromChunk(doneChunk, queueWait)
	cw := contextWarning(mc.NumCtx, doneChunk, stats)
	_ = writeSafe(map[string]any{
		"type": "done", "machine": res.Machine, "model": res.Model, "mode": res.Mode,
		"stats": stats, "context_warning": cw,
	})
}

func (s *Server) writeStreamErr(w http.ResponseWriter, err error, machine string, beforeTokens bool) {
	if isConnectErr(err) {
		s.Reg.MarkDownImmediate(machine)
		_ = writeLine(w, map[string]any{"type": "error", "code": "MACHINE_DOWN", "message": "could not connect", "retryable": true})
		return
	}
	code := "UPSTREAM_ERROR"
	if strings.Contains(err.Error(), "404") {
		s.Reg.RefreshMachine(context.Background(), machine)
		code = "MODEL_NOT_INSTALLED"
	}
	_ = writeLine(w, map[string]any{"type": "error", "code": code, "message": safeUpstreamMsg(err), "retryable": code == "UPSTREAM_ERROR"})
	_ = beforeTokens
}

func (s *Server) chatNonStream(w http.ResponseWriter, r *http.Request, cb ChatBody, msgs []ollama.ChatMessage, res route.Result) {
	ctx := r.Context()
	if err := s.Gate.Acquire(ctx, res.Machine, nil, func() bool { return s.Reg.IsDown(res.Machine) }); err != nil {
		if errors.Is(err, gate.ErrMachineDown) {
			writeErr(w, apperrors.New("MACHINE_DOWN", "machine went down while waiting"))
		}
		return
	}
	defer s.Gate.Release(res.Machine)

	mc, _ := s.Cfg.MachineByName(res.Machine)
	cli := ollama.NewClient(mc.Address, s.Cfg.Timeouts.FirstTokenTimeout.Duration)
	opt := filterOptions(cb.Options)
	if opt == nil {
		opt = map[string]any{}
	}
	opt["num_ctx"] = mc.NumCtx
	ch, err := cli.ChatOnce(ctx, ollama.ChatRequest{
		Model: res.Model, Messages: msgs, Think: cb.Think, Options: opt,
	})
	if err != nil {
		if isConnectErr(err) {
			s.Reg.MarkDownImmediate(res.Machine)
			writeErr(w, apperrors.New("MACHINE_DOWN", "could not connect"))
			return
		}
		writeErr(w, apperrors.New("UPSTREAM_ERROR", safeUpstreamMsg(err)))
		return
	}
	stats := statsFromChunk(ch, 0)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"route":           map[string]string{"machine": res.Machine, "model": res.Model, "mode": res.Mode},
		"text":            ch.Message.Content,
		"thinking":        ch.Message.Thinking,
		"stats":           stats,
		"context_warning": contextWarning(mc.NumCtx, ch, stats),
	})
}

func isConnectErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "dial")
}

func safeUpstreamMsg(err error) string {
	if err == nil {
		return "upstream error"
	}
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
