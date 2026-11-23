package api

import (
	"testing"

	"github.com/azamasad/pingpong/internal/ollama"
)

func TestContextWarning(t *testing.T) {
	ch := ollama.StreamChunk{DoneReason: "length"}
	s := Stats{PromptTokens: 100, OutputTokens: 10}
	if !contextWarning(8192, ch, s) {
		t.Fatal("length reason")
	}
	ch = ollama.StreamChunk{}
	s = Stats{PromptTokens: 7800, OutputTokens: 400}
	if !contextWarning(8192, ch, s) {
		t.Fatal("sum near ctx")
	}
	s = Stats{PromptTokens: 100}
	if contextWarning(8192, ch, s) {
		t.Fatal("small prompt")
	}
}