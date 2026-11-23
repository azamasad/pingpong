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

// Server is a test double for Ollama.
type Server struct {
	Models     []string
	ChatSteps  []Step
	TagsStatus int
	ChatStatus int
	mu         sync.Mutex
	chatCalls  int
	OnChat     func(*http.Request)
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
	type m struct {
		Name string `json:"name"`
	}
	list := make([]m, 0, len(s.Models))
	for _, n := range s.Models {
		list = append(list, m{Name: n})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"models": list})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.chatCalls++
	if s.OnChat != nil {
		s.OnChat(r)
	}
	steps := s.ChatSteps
	status := s.ChatStatus
	s.mu.Unlock()

	if status != 0 && status != 200 {
		w.WriteHeader(status)
		fmt.Fprint(w, `{"error":"overload"}`)
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
	// default done if no steps
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
