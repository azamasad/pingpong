package logx

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

var std = log.New(os.Stderr, "", 0)

// Event writes one whitelist JSON log line (no prompts or model output).
func Event(level, event string, fields map[string]any) {
	m := map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"level": level,
		"event": event,
	}
	for k, v := range fields {
		switch k {
		case "machine", "model", "mode", "state", "error_code", "message", "duration_ms", "request_id":
			m[k] = v
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	std.Println(string(b))
}
