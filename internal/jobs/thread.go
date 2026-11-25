package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/google/uuid"
)

// ThreadBusyError means a turn is still active.
type ThreadBusyError struct{}

func (e *ThreadBusyError) Error() string { return "THREAD_BUSY" }

// ThreadTooLongError means history does not fit or the turn cap is reached.
type ThreadTooLongError struct {
	Estimated int
	Limit     int
}

func (e *ThreadTooLongError) Error() string { return "THREAD_TOO_LONG" }

// PinChange records a machine/model switch after a given turn (V7.5).
type PinChange struct {
	AfterTurn int       `json:"after_turn"`
	Machine   string    `json:"machine"`
	Model     string    `json:"model"`
	At        time.Time `json:"at"`
}

type ThreadMessageReq struct {
	Content string         `json:"content"`
	Trim    string         `json:"trim"`
	Options map[string]any `json:"options"`
	Think   *bool          `json:"think"`
}

func (s *Service) CreateThread(ctx context.Context, title, profile string, fast bool, model, machine string) (*store.Thread, error) {
	fast, model, machine, err := s.cfg.ApplyProfile(profile, fast, model, machine)
	if err != nil {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	model, machine, _ = s.cfg.ResolveAlias(model, machine)
	res := s.resolve(route.Request{Fast: fast, Model: model, Machine: machine})
	if !res.OK {
		return nil, &RouteRejectedError{Code: res.Code}
	}
	now := s.clk.Now().UTC()
	t := &store.Thread{
		ID: uuid.NewString(), Title: title, Machine: res.Machine, Model: res.Model,
		PinChangesJSON: "[]", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.InsertThread(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) SubmitThreadMessage(ctx context.Context, threadID string, req ThreadMessageReq) (string, int, int, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return "", 0, 0, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	th, err := s.store.GetThread(ctx, threadID)
	if err != nil {
		return "", 0, 0, err
	}
	active, err := s.store.ThreadActiveJob(ctx, threadID)
	if err != nil {
		return "", 0, 0, err
	}
	if active != "" {
		return "", 0, 0, &ThreadBusyError{}
	}
	allTurns, err := s.store.ThreadJobs(ctx, threadID)
	if err != nil {
		return "", 0, 0, err
	}
	maxTurn, _ := s.store.MaxThreadTurn(ctx, threadID)
	nextTurn := maxTurn + 1
	if nextTurn > s.cfg.Threads.MaxTurns {
		return "", 0, 0, &ThreadTooLongError{Estimated: nextTurn, Limit: s.cfg.Threads.MaxTurns}
	}
	mc, ok := s.cfg.MachineByName(th.Machine)
	if !ok {
		return "", 0, 0, &RouteRejectedError{Code: "MACHINE_UNKNOWN"}
	}
	doneTurns := doneThreadTurns(allTurns)
	dropped := 0
	trimOldest := req.Trim == "oldest"
	if trimOldest {
		doneTurns, dropped, err = trimDoneTurns(doneTurns, content, mc.NumCtx, th, nextTurn)
		if err != nil {
			return "", 0, 0, err
		}
	} else if err := checkThreadLength(doneTurns, content, mc.NumCtx, th, nextTurn); err != nil {
		return "", 0, 0, err
	}
	machine, model := pairForTurn(th, nextTurn)
	msgs := buildThreadMessages(doneTurns, content)
	body := map[string]any{
		"messages": msgs,
		"machine":  machine,
		"model":    model,
	}
	if req.Options != nil {
		body["options"] = filterOptions(req.Options)
	}
	if req.Think != nil {
		body["think"] = *req.Think
	}
	bodyJSON, _ := json.Marshal(body)
	now := s.clk.Now().UTC()
	id := uuid.NewString()
	j := &store.Job{
		ID: id, CreatedAt: now, UpdatedAt: now,
		RequestJSON: string(bodyJSON),
		Status:      store.StatusWaiting,
		Lane:        store.LaneInteractive,
		Machine:     machine, Model: model,
		ThreadID: threadID, Turn: nextTurn,
	}
	mode := s.resolve(route.Request{Model: model, Machine: machine})
	if mode.OK {
		j.Mode = mode.Mode
	}
	if err := s.store.InsertJob(ctx, j); err != nil {
		return "", 0, 0, err
	}
	th.UpdatedAt = now
	if th.Title == "" {
		th.Title = truncateRunes(content, 40)
	}
	if err := s.store.UpdateThread(ctx, th); err != nil {
		return "", 0, 0, err
	}
	go s.runLoop(id)
	return id, nextTurn, dropped, nil
}

func (s *Service) PatchThread(ctx context.Context, threadID, title, machine, model string, pinChange bool) (*store.Thread, error) {
	th, err := s.store.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	active, err := s.store.ThreadActiveJob(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if active != "" {
		return nil, &ThreadBusyError{}
	}
	now := s.clk.Now().UTC()
	if title != "" {
		th.Title = title
	}
	if pinChange {
		model, machine, _ = s.cfg.ResolveAlias(model, machine)
		res := s.resolve(route.Request{Model: model, Machine: machine})
		if !res.OK {
			return nil, &RouteRejectedError{Code: res.Code}
		}
		maxTurn, _ := s.store.MaxThreadTurn(ctx, threadID)
		changes, _ := parsePinChanges(th.PinChangesJSON)
		changes = append(changes, PinChange{
			AfterTurn: maxTurn, Machine: res.Machine, Model: res.Model, At: now,
		})
		b, _ := json.Marshal(changes)
		th.PinChangesJSON = string(b)
	}
	th.UpdatedAt = now
	if err := s.store.UpdateThread(ctx, th); err != nil {
		return nil, err
	}
	return th, nil
}

func (s *Service) DeleteThread(ctx context.Context, threadID string) error {
	active, err := s.store.ThreadActiveJob(ctx, threadID)
	if err != nil {
		return err
	}
	if active != "" {
		return &ThreadBusyError{}
	}
	return s.store.DeleteThread(ctx, threadID)
}

func doneThreadTurns(turns []*store.Job) []*store.Job {
	var out []*store.Job
	for _, j := range turns {
		if j.Status == store.StatusDone {
			out = append(out, j)
		}
	}
	return out
}

func pairForTurn(th *store.Thread, turn int) (machine, model string) {
	machine, model = th.Machine, th.Model
	changes, _ := parsePinChanges(th.PinChangesJSON)
	for _, c := range changes {
		if c.AfterTurn < turn {
			machine, model = c.Machine, c.Model
		}
	}
	return machine, model
}

func parsePinChanges(raw string) ([]PinChange, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var out []PinChange
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func turnUserPrompt(j *store.Job) string {
	var cb map[string]any
	_ = json.Unmarshal([]byte(j.RequestJSON), &cb)
	if p, ok := cb["prompt"].(string); ok && p != "" {
		return p
	}
	raw, _ := cb["messages"].([]any)
	for i := len(raw) - 1; i >= 0; i-- {
		m, _ := raw[i].(map[string]any)
		if m["role"] == "user" {
			if s, ok := m["content"].(string); ok {
				return s
			}
		}
	}
	return ""
}

func buildThreadMessages(done []*store.Job, newContent string) []ollama.ChatMessage {
	var msgs []ollama.ChatMessage
	for _, j := range done {
		prompt := turnUserPrompt(j)
		msgs = append(msgs, ollama.ChatMessage{Role: "user", Content: prompt})
		msgs = append(msgs, ollama.ChatMessage{Role: "assistant", Content: j.OutputText})
	}
	msgs = append(msgs, ollama.ChatMessage{Role: "user", Content: newContent})
	return msgs
}

func historyTextLen(done []*store.Job, newContent string) int {
	n := utf8.RuneCountInString(newContent)
	for _, j := range done {
		n += utf8.RuneCountInString(turnUserPrompt(j))
		n += utf8.RuneCountInString(j.OutputText)
	}
	return n
}

func estimateTokens(done []*store.Job, newContent string) int {
	if len(done) > 0 {
		last := done[len(done)-1]
		if last.PromptTokens > 0 || last.OutputTokens > 0 {
			return last.PromptTokens + last.OutputTokens + utf8.RuneCountInString(newContent)/3
		}
	}
	return historyTextLen(done, newContent) / 3
}

func checkThreadLength(done []*store.Job, content string, numCtx int, _ *store.Thread, _ int) error {
	est := estimateTokens(done, content)
	limit := numCtx
	if est+512 > limit {
		return &ThreadTooLongError{Estimated: est, Limit: limit}
	}
	return nil
}

func trimDoneTurns(done []*store.Job, content string, numCtx int, th *store.Thread, nextTurn int) ([]*store.Job, int, error) {
	dropped := 0
	for len(done) > 0 {
		if err := checkThreadLength(done, content, numCtx, th, nextTurn); err == nil {
			return done, dropped, nil
		}
		done = done[1:]
		dropped++
	}
	if err := checkThreadLength(done, content, numCtx, th, nextTurn); err != nil {
		return done, dropped, err
	}
	return done, dropped, nil
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// ThreadTurnView is one turn in a thread detail response.
type ThreadTurnView struct {
	Turn      int       `json:"turn"`
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Prompt    string    `json:"prompt"`
	Output    string    `json:"output"`
	Machine   string    `json:"machine"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) ThreadDetail(ctx context.Context, threadID string, limit int) (map[string]any, error) {
	th, err := s.store.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	all, err := s.store.ThreadJobs(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	start := 0
	if len(all) > limit {
		start = len(all) - limit
	}
	slice := all[start:]
	turns := make([]ThreadTurnView, 0, len(slice))
	for _, j := range slice {
		turns = append(turns, ThreadTurnView{
			Turn: j.Turn, JobID: j.ID, Status: string(j.Status),
			Prompt: turnUserPrompt(j), Output: j.OutputText,
			Machine: j.Machine, Model: j.Model, CreatedAt: j.CreatedAt,
		})
	}
	changes, _ := parsePinChanges(th.PinChangesJSON)
	pinOut := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		pinOut = append(pinOut, map[string]any{
			"after_turn": c.AfterTurn, "machine": c.Machine, "model": c.Model,
			"at": c.At.UTC().Format(time.RFC3339),
		})
	}
	return map[string]any{
		"id": th.ID, "title": th.Title, "machine": th.Machine, "model": th.Model,
		"created_at": th.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": th.UpdatedAt.UTC().Format(time.RFC3339),
		"turns":      turns, "pin_changes": pinOut,
	}, nil
}

func (s *Service) ListThreads(ctx context.Context, before *time.Time, limit int) ([]map[string]any, error) {
	list, err := s.store.ListThreads(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list))
	for _, th := range list {
		n, _ := s.store.ThreadTurnCount(ctx, th.ID)
		active, _ := s.store.ThreadActiveJob(ctx, th.ID)
		row := map[string]any{
			"id": th.ID, "title": th.Title, "machine": th.Machine, "model": th.Model,
			"turns": n, "updated_at": th.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if active != "" {
			row["active_job_id"] = active
		}
		out = append(out, row)
	}
	return out, nil
}

func ThreadSummary(th *store.Thread, turns int) map[string]any {
	return map[string]any{
		"id": th.ID, "title": th.Title, "machine": th.Machine, "model": th.Model,
		"turns": turns, "created_at": th.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": th.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func FormatPinChanges(raw string) ([]map[string]any, error) {
	changes, err := parsePinChanges(raw)
	if err != nil {
		return nil, fmt.Errorf("BAD_REQUEST")
	}
	out := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		out = append(out, map[string]any{
			"after_turn": c.AfterTurn, "machine": c.Machine, "model": c.Model,
			"at": c.At.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}
