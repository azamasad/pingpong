package hoststats

import "github.com/azamasad/pingpong/internal/logx"

// LogReports writes one host_sample line for each local or remote reading.
// A machine that is not set up is skipped. The line is counters and names only.
func LogReports(reports []Report) {
	for _, r := range reports {
		if r.Source != "local" && r.Source != "remote" {
			continue
		}
		f := map[string]any{
			"machine": r.Name,
			"source":  r.Source,
		}
		if r.Sample.OK {
			f["outcome"] = "ok"
		} else {
			f["outcome"] = "error"
			if code, ok := hostErrorCode(r.Sample.Error); ok {
				f["error_code"] = code
			}
		}
		putFloat(f, "cpu_percent", r.Sample.CPUPercent)
		putFloat(f, "ram_percent", r.Sample.RAMPercent)
		putFloat(f, "temp_c", r.Sample.TempC)
		putFloat(f, "load_1", r.Sample.Load1)
		putFloat(f, "net_rx_bps", r.Sample.NetRxBps)
		putFloat(f, "net_tx_bps", r.Sample.NetTxBps)
		if r.Sample.ThermalLevel != nil {
			f["thermal_level"] = *r.Sample.ThermalLevel
		}
		logx.Event("info", "host_sample", f)
	}
}

func putFloat(f map[string]any, key string, v *float64) {
	if v != nil {
		f[key] = *v
	}
}

// hostErrorCode returns only the fixed sampler errors. Anything else, including
// a URL, is dropped.
func hostErrorCode(s string) (string, bool) {
	switch s {
	case "unreadable", "unreachable", "bad report", "not_configured":
		return s, true
	default:
		return "", false
	}
}
