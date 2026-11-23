package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	Base    string
	HTTP    *http.Client
	Timeout time.Duration
}

type ModelTag struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type TagsResponse struct {
	Models []ModelTag `json:"models"`
}

type PSModel struct {
	Name string `json:"name"`
}

type PSResponse struct {
	Models []PSModel `json:"models"`
}

type ChatRequest struct {
	Model    string         `json:"model"`
	Messages []ChatMessage  `json:"messages"`
	Stream   *bool          `json:"stream,omitempty"`
	Think    *bool          `json:"think,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamChunk is one NDJSON line from Ollama.
type StreamChunk struct {
	Model   string `json:"model"`
	Message struct {
		Role     string `json:"role"`
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done               bool   `json:"done"`
	Error              string `json:"error"`
	EvalCount          int    `json:"eval_count"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	DoneReason         string `json:"done_reason"`
}

func NewClient(base string, connectTimeout time.Duration) *Client {
	return &Client{
		Base:    strings.TrimRight(base, "/"),
		HTTP:    &http.Client{Timeout: 0}, // streaming: no global timeout
		Timeout: connectTimeout,
	}
}

// Version returns Ollama's reported version from GET /api/version.
func (c *Client) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/version", nil)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version: http %d", resp.StatusCode)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	if v.Version == "" {
		return "unknown", nil
	}
	return v.Version, nil
}

func (c *Client) Tags(ctx context.Context) ([]ModelTag, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tags: http %d", resp.StatusCode)
	}
	var tr TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return tr.Models, nil
}

func (c *Client) PS(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ps: http %d", resp.StatusCode)
	}
	var pr PSResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(pr.Models))
	for _, m := range pr.Models {
		out = append(out, m.Name)
	}
	return out, nil
}

type ChatStream struct {
	Scanner *bufio.Scanner
	Resp    *http.Response
	cancel  context.CancelFunc
}

func (s *ChatStream) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.Resp != nil && s.Resp.Body != nil {
		s.Resp.Body.Close()
	}
}

func (c *Client) ChatStream(ctx context.Context, body ChatRequest) (*ChatStream, error) {
	if body.Stream == nil {
		t := true
		body.Stream = &t
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/api/chat", bytes.NewReader(b))
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("chat: http %d: %s", resp.StatusCode, string(slurp))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &ChatStream{Scanner: sc, Resp: resp, cancel: cancel}, nil
}

func (s *ChatStream) Next() (StreamChunk, bool, error) {
	if !s.Scanner.Scan() {
		if err := s.Scanner.Err(); err != nil {
			return StreamChunk{}, false, err
		}
		return StreamChunk{}, false, nil
	}
	var ch StreamChunk
	if err := json.Unmarshal(s.Scanner.Bytes(), &ch); err != nil {
		return StreamChunk{}, false, err
	}
	if ch.Error != "" {
		return ch, true, fmt.Errorf("ollama: %s", ch.Error)
	}
	return ch, true, nil
}

// ChatOnce non-streaming response.
func (c *Client) ChatOnce(ctx context.Context, body ChatRequest) (StreamChunk, error) {
	f := false
	body.Stream = &f
	b, err := json.Marshal(body)
	if err != nil {
		return StreamChunk{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return StreamChunk{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return StreamChunk{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return StreamChunk{}, fmt.Errorf("chat: http %d: %s", resp.StatusCode, string(slurp))
	}
	var ch StreamChunk
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		return StreamChunk{}, err
	}
	if ch.Error != "" {
		return ch, fmt.Errorf("ollama: %s", ch.Error)
	}
	return ch, nil
}
