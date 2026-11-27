package logx

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"
)

var std = log.New(os.Stderr, "", 0)

// SetOutput redirects structured logs (for tests). The restore func puts stderr back.
func SetOutput(w io.Writer) (restore func()) {
	prev := std
	std = log.New(w, "", 0)
	return func() { std = prev }
}

var allowed = map[string]struct{}{
	"machine": {}, "model": {}, "mode": {}, "state": {}, "error_code": {},
	"message": {}, "duration_ms": {}, "request_id": {}, "source": {},
	"outcome": {}, "lane": {}, "load_ms": {}, "queue_wait_ms": {},
	"prompt_tokens": {}, "output_tokens": {},
	"prompt_tokens_per_s": {}, "output_tokens_per_s": {},
	"batch_id": {}, "stage": {}, "items": {}, "schedule": {},
	"cpu_percent": {}, "ram_percent": {}, "temp_c": {}, "thermal_level": {},
	"load_1": {}, "net_rx_bps": {}, "net_tx_bps": {},
}

// Event writes one whitelist JSON log line (no prompts or model output).
func Event(level, event string, fields map[string]any) {
	m := map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"level": level,
		"event": event,
	}
	for k, v := range fields {
		if _, ok := allowed[k]; ok {
			m[k] = v
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	std.Println(string(b))
}
