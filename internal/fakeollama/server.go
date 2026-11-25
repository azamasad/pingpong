package fakeollama

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Step is one scripted behaviour for POST /api/chat.
type Step struct {
	Wait     time.Duration `json:"wait"`
	Content  string        `json:"content"`
	Thinking string        `json:"thinking"`
	Error    string        `json:"error"`
	Close    bool          `json:"close"`
	HTTPCode int           `json:"http_code"`
	Done     *DoneStats    `json:"done"`
}

type DoneStats struct {
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	DoneReason      string `json:"done_reason"`
}

// Call records one chat request seen by the fake server.
type Call struct {
	Model    string
	Messages []message
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ModelDetail is optional metadata returned from GET /api/tags.
type ModelDetail struct {
	Name          string
	ParameterSize string
}

// Server is a test double for Ollama.
type Server struct {
	Models       []string
	ModelDetails []ModelDetail
	ChatSteps    []Step
	Reply        func(Call) []Step
	TagsStatus   int
	ChatStatus   int
	mu           sync.Mutex
	chatCalls    int
	calls        []Call
	OnChat       func(*http.Request)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", s.handleTags)
	mux.HandleFunc("GET /api/ps", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "fake"})
	})
	mux.HandleFunc("POST /api/chat", s.handleChat)
	return mux
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if s.TagsStatus != 0 && s.TagsStatus != 200 {
		w.WriteHeader(s.TagsStatus)
		return
	}
	type detail struct {
		Name          string `json:"name"`
		ParameterSize string `json:"parameter_size,omitempty"`
	}
	list := make([]detail, 0)
	if len(s.ModelDetails) > 0 {
		for _, m := range s.ModelDetails {
			list = append(list, detail{Name: m.Name, ParameterSize: m.ParameterSize})
		}
	} else {
		for _, n := range s.Models {
			list = append(list, detail{Name: n})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"models": list})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
		Stream   *bool     `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	stream := true
	if req.Stream != nil {
		stream = *req.Stream
	}

	call := Call{Model: req.Model, Messages: req.Messages}

	s.mu.Lock()
	s.chatCalls++
	if s.OnChat != nil {
		s.OnChat(r)
	}
	s.calls = append(s.calls, call)
	steps := s.ChatSteps
	if s.Reply != nil {
		if custom := s.Reply(call); custom != nil {
			steps = custom
		}
	}
	status := s.ChatStatus
	s.mu.Unlock()

	if status != 0 && status != 200 {
		w.WriteHeader(status)
		fmt.Fprint(w, `{"error":"overload"}`)
		return
	}

	if !stream {
		var content, thinking string
		var done DoneStats
		for _, step := range steps {
			if step.Content != "" {
				content = step.Content
			}
			if step.Thinking != "" {
				thinking = step.Thinking
			}
			if step.Done != nil {
				done = *step.Done
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake",
			"message": map[string]string{
				"role": "assistant", "content": content, "thinking": thinking,
			},
			"done": true, "eval_count": done.EvalCount, "prompt_eval_count": done.PromptEvalCount,
			"done_reason": done.DoneReason, "total_duration": int64(time.Second),
		})
		return
	}

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")

	for _, step := range steps {
		if step.Wait > 0 {
			time.Sleep(step.Wait)
		}
		if step.HTTPCode != 0 {
			w.WriteHeader(step.HTTPCode)
			return
		}
		if step.Close {
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
			return
		}
		if step.Error != "" {
			line := map[string]any{"error": step.Error}
			b, _ := json.Marshal(line)
			_, _ = w.Write(append(b, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		if step.Content != "" || step.Thinking != "" {
			line := map[string]any{
				"model": "fake",
				"message": map[string]string{
					"role":     "assistant",
					"content":  step.Content,
					"thinking": step.Thinking,
				},
				"done": false,
			}
			b, _ := json.Marshal(line)
			_, _ = w.Write(append(b, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if step.Done != nil {
			line := map[string]any{
				"model":             "fake",
				"message":           map[string]string{"role": "assistant", "content": ""},
				"done":              true,
				"prompt_eval_count": step.Done.PromptEvalCount,
				"eval_count":        step.Done.EvalCount,
				"done_reason":       step.Done.DoneReason,
				"total_duration":    int64(time.Second),
			}
			b, _ := json.Marshal(line)
			_, _ = w.Write(append(b, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	if len(steps) == 0 {
		line := map[string]any{
			"model":      "fake",
			"message":    map[string]string{"role": "assistant", "content": "ok"},
			"done":       true,
			"eval_count": 2,
		}
		b, _ := json.Marshal(line)
		_, _ = w.Write(append(b, '\n'))
	}
}

func (s *Server) ChatCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatCalls
}

// Calls returns recorded chat calls in order.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Call, len(s.calls))
	copy(out, s.calls)
	return out
}

func NewHTTPTestServer(s *Server) *httptest.Server {
	return httptest.NewServer(s.Handler())
}

func ModelNames(tags []string) []string {
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

func JoinBase(ts *httptest.Server) string {
	return strings.TrimRight(ts.URL, "/")
}
