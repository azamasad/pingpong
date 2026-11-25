package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/route"
)

const (
	oaiModelAuto = "pingpong-auto"
	oaiModelFast = "pingpong-fast"
)

func (s *Server) openaiEnabled() bool {
	return s.Cfg.OpenAI.Enabled
}

func (s *Server) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if !s.openaiEnabled() {
		writeErr(w, apperrors.New("NOT_FOUND", "not found"))
		return
	}
	seen := map[string]struct{}{}
	var data []map[string]any
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		data = append(data, map[string]any{
			"id": id, "object": "model", "created": 0, "owned_by": "pingpong",
		})
	}
	for _, m := range s.Reg.ListModels() {
		add(m.Name)
	}
	if s.Cfg.Aliases != nil {
		names := make([]string, 0, len(s.Cfg.Aliases))
		for n := range s.Cfg.Aliases {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			add(n)
		}
	}
	add(oaiModelAuto)
	add(oaiModelFast)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !s.openaiEnabled() {
		writeErr(w, apperrors.New("NOT_FOUND", "not found"))
		return
	}
	if s.shuttingDown.Load() {
		writeOpenAIErr(w, apperrors.New("ROUTER_RESTARTING", "router is shutting down"))
		return
	}
	_ = r.Header.Get("Authorization")
	max := s.Cfg.Router.MaxBodyBytes
	body, err := readBodyLimited(r, max)
	if err != nil {
		writeOpenAIErr(w, apperrors.New("BAD_REQUEST", "invalid body"))
		return
	}
	if int64(len(body)) > max {
		writeOpenAIErr(w, apperrors.New("BODY_TOO_LARGE", "body too large"))
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		writeOpenAIErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	if err := rejectOpenAIFields(raw); err != nil {
		writeOpenAIErr(w, err.(apperrors.APIError))
		return
	}
	var req openAIChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeOpenAIErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	msgs, err := parseOpenAIMessages(req.Messages)
	if err != nil {
		writeOpenAIErr(w, err.(apperrors.APIError))
		return
	}
	fast, model, machine := parseOpenAIModel(s.Cfg, req.Model)
	model, machine, _ = s.Cfg.ResolveAlias(model, machine)
	opt := openAIOptions(req)
	routeReq := route.Request{Fast: fast, Model: model, Machine: machine}
	res := s.resolveRoute(r.Context(), routeReq)
	if !res.OK {
		ae := apperrors.New(res.Code, routeMessages(res.Code))
		writeOpenAIErr(w, ae)
		return
	}
	stream := req.Stream != nil && *req.Stream
	created := time.Now().Unix()
	chatID := "chatcmpl-" + randomHex(12)
	if stream {
		s.openAIStream(w, r, msgs, res, opt, chatID, created)
		return
	}
	s.openAINonStream(w, r, msgs, res, opt, chatID, created)
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      *bool           `json:"stream"`
	Temperature *float64        `json:"temperature"`
	TopP        *float64        `json:"top_p"`
	MaxTokens   *int            `json:"max_tokens"`
	Stop        json.RawMessage `json:"stop"`
	Seed        *int            `json:"seed"`
}

type openAIMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func rejectOpenAIFields(raw map[string]json.RawMessage) error {
	bad := func(msg string) apperrors.APIError { return apperrors.New("BAD_REQUEST", msg) }
	for _, k := range []string{"tools", "functions", "response_format", "logprobs"} {
		if _, ok := raw[k]; ok {
			return bad(k + " is not supported")
		}
	}
	if v, ok := raw["n"]; ok {
		var n int
		if err := json.Unmarshal(v, &n); err != nil || n != 1 {
			return bad("n must be 1")
		}
	}
	return nil
}

func parseOpenAIModel(cfg *config.Config, name string) (fast bool, model, machine string) {
	switch name {
	case oaiModelAuto:
		return false, "", ""
	case oaiModelFast:
		return true, "", ""
	case "":
		return false, "", ""
	}
	if i := strings.Index(name, "/"); i > 0 {
		prefix := name[:i]
		rest := name[i+1:]
		if _, ok := cfg.MachineByName(prefix); ok {
			return false, rest, prefix
		}
	}
	return false, name, ""
}

func parseOpenAIMessages(in []openAIMessage) ([]ollama.ChatMessage, error) {
	if len(in) == 0 {
		return nil, apperrors.New("BAD_REQUEST", "messages required")
	}
	out := make([]ollama.ChatMessage, 0, len(in))
	for _, m := range in {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			return nil, apperrors.New("BAD_REQUEST", "invalid message role")
		}
		text, err := parseOpenAIContent(m.Content)
		if err != nil {
			return nil, err
		}
		out = append(out, ollama.ChatMessage{Role: m.Role, Content: text})
	}
	last := out[len(out)-1]
	if last.Role != "user" {
		return nil, apperrors.New("BAD_REQUEST", "last message must be from user")
	}
	for _, m := range out {
		if m.Role == "user" && m.Content == "" {
			return nil, apperrors.New("BAD_REQUEST", "empty message content")
		}
	}
	return out, nil
}

func parseOpenAIContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", apperrors.New("BAD_REQUEST", "empty message content")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", apperrors.New("BAD_REQUEST", "invalid message content")
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type != "text" {
			return "", apperrors.New("BAD_REQUEST", "only text content parts are supported")
		}
		b.WriteString(p.Text)
	}
	return b.String(), nil
}

func openAIOptions(req openAIChatRequest) map[string]any {
	opt := map[string]any{}
	if req.Temperature != nil {
		opt["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		opt["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		opt["num_predict"] = *req.MaxTokens
	}
	if req.Seed != nil {
		opt["seed"] = *req.Seed
	}
	if len(req.Stop) > 0 {
		var one string
		if json.Unmarshal(req.Stop, &one) == nil {
			opt["stop"] = one
		} else {
			var many []string
			if json.Unmarshal(req.Stop, &many) == nil {
				opt["stop"] = many
			}
		}
	}
	return filterOptions(opt)
}

func writeOpenAIErr(w http.ResponseWriter, e apperrors.APIError) {
	errType := "invalid_request_error"
	if e.HTTPStatus >= 500 {
		errType = "server_error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": e.Message,
			"type":    errType,
			"param":   nil,
			"code":    e.Code,
		},
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func oaiFinishReason(ch ollama.StreamChunk) string {
	if ch.DoneReason == "length" {
		return "length"
	}
	return "stop"
}

func (s *Server) openAINonStream(w http.ResponseWriter, r *http.Request, msgs []ollama.ChatMessage, res route.Result, opt map[string]any, chatID string, created int64) {
	ctx := r.Context()
	if err := s.Gate.Acquire(ctx, res.Machine, nil, func() bool { return s.Reg.IsDown(res.Machine) }); err != nil {
		if errors.Is(err, gate.ErrMachineDown) {
			writeOpenAIErr(w, apperrors.New("MACHINE_DOWN", "machine went down while waiting"))
		}
		return
	}
	defer s.Gate.Release(res.Machine)

	mc, _ := s.Cfg.MachineByName(res.Machine)
	cli := ollama.NewClient(mc.Address, s.Cfg.Timeouts.FirstTokenTimeout.Duration)
	if opt == nil {
		opt = map[string]any{}
	}
	opt["num_ctx"] = mc.NumCtx
	ch, err := cli.ChatOnce(ctx, ollama.ChatRequest{
		Model: res.Model, Messages: msgs, Options: opt,
	})
	if err != nil {
		if isConnectErr(err) {
			s.Reg.MarkDownImmediate(res.Machine)
			writeOpenAIErr(w, apperrors.New("MACHINE_DOWN", "could not connect"))
			return
		}
		writeOpenAIErr(w, apperrors.New("UPSTREAM_ERROR", safeUpstreamMsg(err)))
		return
	}
	stats := statsFromChunk(ch, 0)
	logRequestDone("openai", res, stats, 0)
	usage := map[string]int{
		"prompt_tokens":     stats.PromptTokens,
		"completion_tokens": stats.OutputTokens,
		"total_tokens":      stats.PromptTokens + stats.OutputTokens,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": chatID, "object": "chat.completion", "created": created, "model": res.Model,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]string{
				"role": "assistant", "content": ch.Message.Content,
			},
			"finish_reason": oaiFinishReason(ch),
		}},
		"usage":    usage,
		"pingpong": map[string]string{"machine": res.Machine, "mode": res.Mode},
	})
}

func (s *Server) openAIStream(w http.ResponseWriter, r *http.Request, msgs []ollama.ChatMessage, res route.Result, opt map[string]any, chatID string, created int64) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	waitStart := time.Now()
	err := s.Gate.Acquire(ctx, res.Machine, nil, func() bool { return s.Reg.IsDown(res.Machine) })
	if err != nil {
		if errors.Is(err, gate.ErrMachineDown) {
			writeOpenAIErr(w, apperrors.New("MACHINE_DOWN", "machine went down while waiting"))
		}
		return
	}
	defer s.Gate.Release(res.Machine)
	queueWait := time.Since(waitStart).Milliseconds()

	mc, _ := s.Cfg.MachineByName(res.Machine)
	cli := ollama.NewClient(mc.Address, s.Cfg.Timeouts.ConnectTimeout.Duration)
	if opt == nil {
		opt = map[string]any{}
	}
	opt["num_ctx"] = mc.NumCtx
	stream, err := cli.ChatStream(ctx, ollama.ChatRequest{
		Model: res.Model, Messages: msgs, Options: opt,
	})
	if err != nil {
		s.writeOpenAIStreamSetupErr(w, err, res.Machine)
		return
	}
	defer stream.Close()

	var writeMu sync.Mutex
	writeSSE := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return writeSSEData(w, v)
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
				writeMu.Lock()
				_, _ = fmt.Fprint(w, ": ping\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				writeMu.Unlock()
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
			if !open {
				break streamLoop
			}
			if cr.err != nil {
				return
			}
			if !cr.ok {
				break streamLoop
			}
			if !gotFirst {
				gotFirst = true
				lastChunk = time.Now()
			}
			if cr.ch.Message.Content != "" {
				_ = writeSSE(openAIChunk(chatID, created, res.Model, cr.ch.Message.Content, ""))
			}
			if cr.ch.Done {
				doneChunk = cr.ch
				break streamLoop
			}
			lastChunk = time.Now()
		case <-tick.C:
			if s.shuttingDown.Load() {
				return
			}
			if time.Since(start) > maxRun {
				cancel()
				stream.Close()
				return
			}
			if !gotFirst && time.Since(start) > firstTO {
				cancel()
				stream.Close()
				return
			}
			if gotFirst && time.Since(lastChunk) > idleTO {
				cancel()
				stream.Close()
				return
			}
		}
	}

	stats := statsFromChunk(doneChunk, queueWait)
	logRequestDone("openai", res, stats, time.Since(start))
	fr := oaiFinishReason(doneChunk)
	_ = writeSSE(openAIChunk(chatID, created, res.Model, "", fr))
	writeMu.Lock()
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	writeMu.Unlock()
}

func openAIChunk(id string, created int64, model, content, finishReason string) map[string]any {
	delta := map[string]string{}
	if content != "" {
		delta["content"] = content
	}
	ch := map[string]any{
		"index": 0, "delta": delta,
	}
	if finishReason != "" {
		ch["finish_reason"] = finishReason
	} else {
		ch["finish_reason"] = nil
	}
	return map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []map[string]any{ch},
	}
}

func writeSSEData(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func (s *Server) writeOpenAIStreamSetupErr(w http.ResponseWriter, err error, machine string) {
	if isConnectErr(err) {
		s.Reg.MarkDownImmediate(machine)
		writeOpenAIErr(w, apperrors.New("MACHINE_DOWN", "could not connect"))
		return
	}
	writeOpenAIErr(w, apperrors.New("UPSTREAM_ERROR", safeUpstreamMsg(err)))
}
