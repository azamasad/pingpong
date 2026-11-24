package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/store"
)

const (
	stressPrompt     = "Write one short paragraph about how a packet crosses a home network."
	stressNumPredict = 128
	stressAhead      = 2
)

// stressRun is one soak test. It keeps each chosen machine busy by submitting
// durable batch jobs, so the MacBook Air can close and the run continues.
type stressRun struct {
	mu       sync.Mutex
	running  bool
	stop     chan struct{}
	once     sync.Once
	started  time.Time
	until    time.Time
	target   string
	machines []string
	owned    map[string]struct{}
	samples  []stressSample
	lastErr  string
}

type stressSample struct {
	Machine      string
	Status       string
	TotalMs      float64
	LoadMs       float64
	OutputTPS    float64
	PromptTPS    float64
	OutputTokens float64
	Err          string
}

func (s *Server) handleStressGet(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.stressSnapshot())
}

func (s *Server) handleStressStart(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	var body struct {
		Target  string `json:"target"`
		Seconds int    `json:"seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid json"))
		return
	}
	if body.Seconds < 1 || body.Seconds > 2*60*60 {
		writeErr(w, apperrors.New("BAD_REQUEST", "seconds must be from 1 to 7200"))
		return
	}
	machines, err := s.stressMachines(body.Target)
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "target must be mane, firmino, or both"))
		return
	}
	if !s.startStress(body.Target, machines, time.Duration(body.Seconds)*time.Second) {
		writeErr(w, apperrors.New("CONFLICT", "a stress run is already going"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(s.stressSnapshot())
}

func (s *Server) handleStressStop(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	s.stopStress()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.stressSnapshot())
}

func (s *Server) stressMachines(target string) ([]string, error) {
	if target == "both" {
		names := make([]string, 0, len(s.Cfg.Machines))
		for _, m := range s.Cfg.Machines {
			names = append(names, m.Name)
		}
		if len(names) == 0 {
			return nil, errBadTarget
		}
		return names, nil
	}
	if _, ok := s.Cfg.MachineByName(target); !ok {
		return nil, errBadTarget
	}
	return []string{target}, nil
}

var errBadTarget = apperrors.New("BAD_REQUEST", "bad target")

func (s *Server) startStress(target string, machines []string, dur time.Duration) bool {
	s.stress.mu.Lock()
	defer s.stress.mu.Unlock()
	if s.stress.running {
		return false
	}
	now := time.Now()
	s.stress.running = true
	s.stress.stop = make(chan struct{})
	s.stress.once = sync.Once{}
	s.stress.started = now
	s.stress.until = now.Add(dur)
	s.stress.target = target
	s.stress.machines = machines
	s.stress.owned = map[string]struct{}{}
	s.stress.samples = nil
	s.stress.lastErr = ""
	stop := s.stress.stop
	until := s.stress.until
	go s.stressLoop(machines, until, stop)
	return true
}

func (s *Server) stopStress() {
	s.stress.mu.Lock()
	stop := s.stress.stop
	once := &s.stress.once
	running := s.stress.running
	s.stress.mu.Unlock()
	if !running || stop == nil {
		return
	}
	once.Do(func() { close(stop) })
}

func (s *Server) stressLoop(machines []string, until time.Time, stop <-chan struct{}) {
	defer s.markStressStopped()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	s.stressTick(machines)
	for {
		select {
		case <-stop:
			s.stressCancelOwned()
			return
		case <-tick.C:
			if time.Now().After(until) {
				s.stressCancelOwned()
				return
			}
			s.stressTick(machines)
		}
	}
}

func (s *Server) markStressStopped() {
	s.stress.mu.Lock()
	s.stress.running = false
	s.stress.mu.Unlock()
}

func (s *Server) stressTick(machines []string) {
	if s.stressStopped() {
		return
	}
	ctx := context.Background()
	s.stress.mu.Lock()
	owned := make([]string, 0, len(s.stress.owned))
	for id := range s.stress.owned {
		owned = append(owned, id)
	}
	s.stress.mu.Unlock()

	openByMachine := map[string]int{}
	for _, id := range owned {
		j, err := s.Jobs.Get(ctx, id)
		if err != nil || j == nil {
			continue
		}
		if terminalStatus(j.Status) {
			s.harvest(ctx, j)
			continue
		}
		openByMachine[j.Machine]++
	}

	for _, name := range machines {
		for openByMachine[name] < stressAhead {
			if s.stressStopped() {
				return
			}
			id, err := s.Jobs.Submit(ctx, stressBody(name))
			if err != nil {
				s.noteStressErr(name + ": " + err.Error())
				break
			}
			s.stress.mu.Lock()
			s.stress.owned[id] = struct{}{}
			s.stress.mu.Unlock()
			openByMachine[name]++
		}
	}
}

func stressBody(machine string) string {
	b, _ := json.Marshal(map[string]any{
		"prompt":  stressPrompt,
		"machine": machine,
		"lane":    "batch",
		"options": map[string]any{"num_predict": stressNumPredict},
	})
	return string(b)
}

func (s *Server) harvest(ctx context.Context, j *store.Job) {
	s.stress.mu.Lock()
	if _, ok := s.stress.owned[j.ID]; !ok {
		s.stress.mu.Unlock()
		return
	}
	delete(s.stress.owned, j.ID)
	s.stress.mu.Unlock()

	sample := stressSample{Machine: j.Machine, Status: string(j.Status), Err: j.ErrorCode}
	if j.Machine == "" {
		sample.Machine = "unknown"
	}
	if events, err := s.Jobs.EventsSince(ctx, j.ID, 0); err == nil {
		for _, e := range events {
			if e.Type != "done" {
				continue
			}
			var data map[string]any
			_ = json.Unmarshal([]byte(e.DataJSON), &data)
			stats, _ := data["stats"].(map[string]any)
			sample.TotalMs = num(stats["total_ms"])
			sample.LoadMs = num(stats["load_ms"])
			sample.OutputTPS = num(stats["output_tokens_per_s"])
			sample.PromptTPS = num(stats["prompt_tokens_per_s"])
			sample.OutputTokens = num(stats["output_tokens"])
		}
	}
	s.stress.mu.Lock()
	s.stress.samples = append(s.stress.samples, sample)
	if sample.Err != "" {
		s.stress.lastErr = sample.Machine + ": " + sample.Err
	}
	s.stress.mu.Unlock()
}

func (s *Server) stressStopped() bool {
	s.stress.mu.Lock()
	stop := s.stress.stop
	s.stress.mu.Unlock()
	if stop == nil {
		return true
	}
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

func (s *Server) noteStressErr(msg string) {
	s.stress.mu.Lock()
	s.stress.lastErr = msg
	s.stress.mu.Unlock()
}

func (s *Server) stressCancelOwned() {
	s.stress.mu.Lock()
	ids := make([]string, 0, len(s.stress.owned))
	for id := range s.stress.owned {
		ids = append(ids, id)
	}
	s.stress.owned = map[string]struct{}{}
	s.stress.mu.Unlock()
	ctx := context.Background()
	for _, id := range ids {
		_ = s.Jobs.Cancel(ctx, id)
	}
}

func (s *Server) stressSnapshot() map[string]any {
	s.stress.mu.Lock()
	defer s.stress.mu.Unlock()
	out := map[string]any{
		"running":  s.stress.running,
		"target":   s.stress.target,
		"last_err": s.stress.lastErr,
		"queued":   len(s.stress.owned),
	}
	if !s.stress.started.IsZero() {
		out["started_at"] = s.stress.started.UTC().Format(time.RFC3339)
		out["until"] = s.stress.until.UTC().Format(time.RFC3339)
		out["remaining_seconds"] = int(time.Until(s.stress.until).Seconds())
		if !s.stress.running {
			out["remaining_seconds"] = 0
		}
	}
	by := map[string][]stressSample{}
	for _, sm := range s.stress.samples {
		by[sm.Machine] = append(by[sm.Machine], sm)
	}
	machines := map[string]any{}
	for name, list := range by {
		machines[name] = summarize(list)
	}
	out["machines"] = machines
	return out
}

func summarize(list []stressSample) map[string]any {
	done, failed := 0, 0
	var outTPS, promptTPS, load, total []float64
	for _, sm := range list {
		if sm.Status == string(store.StatusDone) {
			done++
		} else {
			failed++
		}
		if sm.OutputTPS > 0 {
			outTPS = append(outTPS, sm.OutputTPS)
		}
		if sm.PromptTPS > 0 {
			promptTPS = append(promptTPS, sm.PromptTPS)
		}
		if sm.LoadMs > 0 {
			load = append(load, sm.LoadMs)
		}
		if sm.TotalMs > 0 {
			total = append(total, sm.TotalMs)
		}
	}
	return map[string]any{
		"done":              done,
		"failed":            failed,
		"output_tps_median": median(outTPS),
		"output_tps_min":    minF(outTPS),
		"output_tps_max":    maxF(outTPS),
		"prompt_tps_median": median(promptTPS),
		"load_ms_median":    median(load),
		"total_ms_median":   median(total),
	}
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func minF(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, v := range xs[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxF(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, v := range xs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
