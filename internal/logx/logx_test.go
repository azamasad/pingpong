package logx

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestEventDropsFieldsOutsideTheWhitelist(t *testing.T) {
	var buf bytes.Buffer
	old := std
	std = log.New(&buf, "", 0)
	defer func() { std = old }()

	Event("info", "request_done", map[string]any{
		"machine": "mane", "output_tokens_per_s": 21.3,
		"prompt": "secret question", "text": "secret answer",
	})
	out := buf.String()
	if !strings.Contains(out, `"machine":"mane"`) || !strings.Contains(out, `"output_tokens_per_s":21.3`) {
		t.Fatalf("missing allowed fields: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("leaked content: %s", out)
	}
}
