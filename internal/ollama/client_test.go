package ollama_test

import (
	"context"
	"testing"
	"time"

	"github.com/azamasad/pingpong/internal/fakeollama"
	"github.com/azamasad/pingpong/internal/ollama"
)

func TestTagsAndChat(t *testing.T) {
	fs := &fakeollama.Server{
		Models: []string{"m:8b"},
		ChatSteps: []fakeollama.Step{
			{Content: "Hi"},
			{Done: &fakeollama.DoneStats{EvalCount: 1, PromptEvalCount: 3}},
		},
	}
	ts := fakeollama.NewHTTPTestServer(fs)
	defer ts.Close()
	cli := ollama.NewClient(ts.URL, time.Second)
	tags, err := cli.Tags(context.Background())
	if err != nil || len(tags) != 1 {
		t.Fatalf("tags: %v %v", tags, err)
	}
	stream, err := cli.ChatStream(context.Background(), ollama.ChatRequest{
		Model: "m:8b", Messages: []ollama.ChatMessage{{Role: "user", Content: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var tokens []string
	for {
		ch, ok, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if ch.Message.Content != "" {
			tokens = append(tokens, ch.Message.Content)
		}
		if ch.Done {
			break
		}
	}
	if len(tokens) != 1 || tokens[0] != "Hi" {
		t.Fatalf("tokens %v", tokens)
	}
}
