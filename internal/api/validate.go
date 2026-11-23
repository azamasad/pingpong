package api

import (
	"strings"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/ollama"
)

var allowedOptions = map[string]struct{}{
	"temperature": {}, "top_p": {}, "top_k": {}, "min_p": {}, "seed": {},
	"num_predict": {}, "repeat_penalty": {}, "stop": {}, "num_thread": {},
}

type ChatBody struct {
	Messages []ollama.ChatMessage `json:"messages"`
	Prompt   string               `json:"prompt"`
	Fast     bool                 `json:"fast"`
	Model    string               `json:"model"`
	Machine  string               `json:"machine"`
	Think    *bool                `json:"think"`
	Stream   *bool                `json:"stream"`
	Options  map[string]any       `json:"options"`
	Profile  string               `json:"profile"`
}

func filterOptions(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any)
	for k, v := range in {
		if _, ok := allowedOptions[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (b *ChatBody) messages() ([]ollama.ChatMessage, error) {
	if b.Prompt != "" && len(b.Messages) > 0 {
		return nil, apperrors.New("BAD_REQUEST", "provide messages or prompt, not both")
	}
	if b.Prompt != "" {
		return []ollama.ChatMessage{{Role: "user", Content: b.Prompt}}, nil
	}
	if len(b.Messages) == 0 {
		return nil, apperrors.New("BAD_REQUEST", "messages required")
	}
	for i, m := range b.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			return nil, apperrors.New("BAD_REQUEST", "invalid message role")
		}
		if strings.TrimSpace(m.Content) == "" && m.Role != "assistant" {
			_ = i
		}
		if m.Role == "user" || m.Role == "assistant" || m.Role == "system" {
			if m.Content == "" && m.Role == "user" {
				return nil, apperrors.New("BAD_REQUEST", "empty message content")
			}
		}
	}
	last := b.Messages[len(b.Messages)-1]
	if last.Role != "user" {
		return nil, apperrors.New("BAD_REQUEST", "last message must be from user")
	}
	return b.Messages, nil
}

func (b *ChatBody) streaming() bool {
	if b.Stream == nil {
		return true
	}
	return *b.Stream
}
