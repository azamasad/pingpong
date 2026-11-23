package api

import (
	"testing"

	"github.com/azamasad/pingpong/internal/ollama"
)

func TestMessagesRules(t *testing.T) {
	cb := ChatBody{Prompt: "hi", Messages: []ollama.ChatMessage{{Role: "user", Content: "x"}}}
	if _, err := cb.messages(); err == nil {
		t.Fatal("both prompt and messages")
	}
	cb = ChatBody{Messages: []ollama.ChatMessage{{Role: "user", Content: "hi"}}}
	if _, err := cb.messages(); err != nil {
		t.Fatal(err)
	}
	cb = ChatBody{Messages: []ollama.ChatMessage{{Role: "assistant", Content: "x"}}}
	if _, err := cb.messages(); err == nil {
		t.Fatal("last must be user")
	}
}
