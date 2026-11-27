package hoststats

import (
	"strings"
	"testing"

	"github.com/azamasad/pingpong/internal/logx"
)

func TestLogReportsOneLinePerCollectedSample(t *testing.T) {
	var buf strings.Builder
	restore := logx.SetOutput(&buf)
	defer restore()

	cpu, ram, temp, load, rx, tx := 11.0, 42.5, 54.0, 1.25, 1500.0, 800.0
	thermal := 37
	LogReports([]Report{
		{Name: "mane", Source: "local", Sample: Sample{
			OK: true, CPUPercent: &cpu, RAMPercent: &ram, TempC: &temp,
			Load1: &load, NetRxBps: &rx, NetTxBps: &tx,
		}},
		{Name: "firmino", Source: "remote", Sample: Sample{
			OK: true, CPUPercent: &cpu, RAMPercent: &ram, ThermalLevel: &thermal,
			Load1: &load, NetRxBps: &rx, NetTxBps: &tx,
		}},
		{Name: "other", Source: "none", Sample: Sample{OK: false, Error: "not_configured"}},
		{Name: "bad", Source: "remote", Sample: Sample{
			OK: false, Error: "http://secret.example/hook prompt output",
		}},
	})

	text := buf.String()
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines %d: %s", len(lines), text)
	}
	for _, want := range []string{
		`"event":"host_sample"`, `"machine":"mane"`, `"machine":"firmino"`, `"machine":"bad"`,
		`"cpu_percent":11`, `"ram_percent":42.5`, `"temp_c":54`, `"load_1":1.25`,
		`"net_rx_bps":1500`, `"net_tx_bps":800`, `"thermal_level":37`, `"time":`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
	if strings.Contains(text, "other") || strings.Contains(text, "not_configured") {
		t.Fatalf("logged a machine that is not set up: %s", text)
	}
	for _, leaked := range []string{"secret", "prompt", "output", "http", "hook"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("leaked %q: %s", leaked, text)
		}
	}
}
