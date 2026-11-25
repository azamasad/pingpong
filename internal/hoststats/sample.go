package hoststats

import (
	"net"
	"net/url"
	"sync"
	"time"
)

// Sample is one hardware reading. Rates are omitted until a second reading exists.
type Sample struct {
	OK            bool     `json:"ok"`
	Error         string   `json:"error,omitempty"`
	CPUPercent    *float64 `json:"cpu_percent,omitempty"`
	RAMPercent    *float64 `json:"ram_percent,omitempty"`
	RAMUsedBytes  *uint64  `json:"ram_used_bytes,omitempty"`
	RAMTotalBytes *uint64  `json:"ram_total_bytes,omitempty"`
	TempC         *float64 `json:"temp_c,omitempty"`
	ThermalLevel  *int     `json:"thermal_level,omitempty"`
	NetRxBps      *float64 `json:"net_rx_bps,omitempty"`
	NetTxBps      *float64 `json:"net_tx_bps,omitempty"`
	Load1         *float64 `json:"load_1,omitempty"`
}

type raw struct {
	CPUTotal uint64
	CPUIdle  uint64
	RAMTotal uint64
	RAMUsed  uint64
	NetRx    uint64
	NetTx    uint64
	TempC    *float64
	Thermal  *int
	Load1    *float64
}

// Sampler keeps the previous counters so a later reading can show rates.
type Sampler struct {
	mu      sync.Mutex
	prev    raw
	prevAt  time.Time
	hasPrev bool
	now     func() time.Time
	read    func() (raw, error)
}

// NewSampler reads this machine.
func NewSampler() *Sampler {
	return &Sampler{now: time.Now, read: readHost}
}

// Sample reads the host once. The first call has no rates.
func (s *Sampler) Sample() Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.read()
	if err != nil {
		return Sample{OK: false, Error: "unreadable"}
	}
	now := s.now()
	out := Sample{OK: true, TempC: cur.TempC, ThermalLevel: cur.Thermal, Load1: cur.Load1}
	if cur.RAMTotal > 0 {
		total := cur.RAMTotal
		used := cur.RAMUsed
		out.RAMTotalBytes = &total
		out.RAMUsedBytes = &used
		pct := float64(used) / float64(total) * 100
		out.RAMPercent = &pct
	}
	if s.hasPrev {
		dt := now.Sub(s.prevAt).Seconds()
		cpuDelta := float64(cur.CPUTotal - s.prev.CPUTotal)
		if cpuDelta > 0 {
			idleDelta := float64(cur.CPUIdle - s.prev.CPUIdle)
			pct := (1 - idleDelta/cpuDelta) * 100
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			out.CPUPercent = &pct
		}
		if dt > 0 {
			rx := float64(cur.NetRx-s.prev.NetRx) / dt
			tx := float64(cur.NetTx-s.prev.NetTx) / dt
			if rx < 0 {
				rx = 0
			}
			if tx < 0 {
				tx = 0
			}
			out.NetRxBps = &rx
			out.NetTxBps = &tx
		}
	}
	s.prev = cur
	s.prevAt = now
	s.hasPrev = true
	return out
}

// LocalMachine reports whether an Ollama address is on this host.
func LocalMachine(address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// CheckListen rejects an address that would listen on every interface.
func CheckListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errListen
	}
	if ip.IsUnspecified() {
		return errUnspecified
	}
	return nil
}
