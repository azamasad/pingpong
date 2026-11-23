package jobs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azamasad/pingpong/internal/ollama"
)

type chatBody struct {
	Messages []ollama.ChatMessage `json:"messages"`
	Prompt   string               `json:"prompt"`
	Fast     bool                 `json:"fast"`
	Model    string               `json:"model"`
	Machine  string               `json:"machine"`
	Think    *bool                `json:"think"`
	Stream   *bool                `json:"stream"`
	Options  map[string]any       `json:"options"`
}

func parseChatBody(bodyJSON string) (chatBody, []ollama.ChatMessage, error) {
	var cb chatBody
	if err := json.Unmarshal([]byte(bodyJSON), &cb); err != nil {
		return cb, nil, fmt.Errorf("BAD_REQUEST")
	}
	msgs, err := cb.messages()
	if err != nil {
		return cb, nil, err
	}
	return cb, msgs, nil
}

func (b *chatBody) messages() ([]ollama.ChatMessage, error) {
	if b.Prompt != "" && len(b.Messages) > 0 {
		return nil, fmt.Errorf("BAD_REQUEST")
	}
	if b.Prompt != "" {
		return []ollama.ChatMessage{{Role: "user", Content: b.Prompt}}, nil
	}
	if len(b.Messages) == 0 {
		return nil, fmt.Errorf("BAD_REQUEST")
	}
	for _, m := range b.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			return nil, fmt.Errorf("BAD_REQUEST")
		}
		if m.Role == "user" && strings.TrimSpace(m.Content) == "" {
			return nil, fmt.Errorf("BAD_REQUEST")
		}
	}
	last := b.Messages[len(b.Messages)-1]
	if last.Role != "user" {
		return nil, fmt.Errorf("BAD_REQUEST")
	}
	return b.Messages, nil
}

var allowedOptions = map[string]struct{}{
	"temperature": {}, "top_p": {}, "top_k": {}, "min_p": {}, "seed": {},
	"num_predict": {}, "repeat_penalty": {}, "stop": {}, "num_thread": {},
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
