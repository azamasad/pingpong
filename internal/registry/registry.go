package registry

import (
	"context"
	"sync"
	"time"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/gate"
	"github.com/azamasad/pingpong/internal/ollama"
	"github.com/azamasad/pingpong/internal/route"
)

type ModelInfo struct {
	Name          string `json:"name"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	ParameterSize string `json:"parameter_size,omitempty"`
}

type MachineView struct {
	Name         string      `json:"name"`
	Roles        []string    `json:"roles"`
	Reachable    bool        `json:"reachable"`
	Busy         bool        `json:"busy"`
	Waiting      int         `json:"waiting"`
	DefaultModel string      `json:"default_model"`
	Models       []ModelInfo `json:"models"`
	LoadedModels []string    `json:"loaded_models"`
	LastChecked  *time.Time  `json:"last_checked"`
}

// Registry tracks worker health and installed models.
type Registry struct {
	cfg   *config.Config
	gate  *gate.Gate
	mu    sync.RWMutex
	state map[string]*machineState
	stop  chan struct{}
}

type machineState struct {
	cfg          config.Machine
	names        []string
	models       []ModelInfo
	loaded       []string
	reachable    bool
	down         bool
	neverReached bool
	failures     int
	lastChecked  time.Time
	inFlight     int // from registry perspective for health rule
}

func New(cfg *config.Config, g *gate.Gate) *Registry {
	st := make(map[string]*machineState)
	for _, m := range cfg.Machines {
		st[m.Name] = &machineState{
			cfg:          m,
			neverReached: true,
		}
	}
	return &Registry{cfg: cfg, gate: g, state: st, stop: make(chan struct{})}
}

func (r *Registry) Start(ctx context.Context) {
	go r.loop(ctx)
}

func (r *Registry) Stop() { close(r.stop) }

func (r *Registry) loop(ctx context.Context) {
	tick := time.NewTicker(r.cfg.Health.Interval.Duration)
	refresh := time.NewTicker(r.cfg.Health.RegistryRefreshInterval.Duration)
	defer tick.Stop()
	defer refresh.Stop()
	r.refreshAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stop:
			return
		case <-tick.C:
			r.healthAll(ctx)
		case <-refresh.C:
			r.refreshAll(ctx)
		}
	}
}

func (r *Registry) client(m config.Machine) *ollama.Client {
	return ollama.NewClient(m.Address, r.cfg.Timeouts.ConnectTimeout.Duration)
}

func (r *Registry) healthAll(ctx context.Context) {
	for _, m := range r.cfg.Machines {
		r.checkOne(ctx, m.Name)
	}
}

func (r *Registry) refreshAll(ctx context.Context) {
	for _, m := range r.cfg.Machines {
		r.refreshOne(ctx, m.Name)
	}
}

func (r *Registry) refreshOne(ctx context.Context, name string) {
	cli := r.client(r.mustMachine(name))
	tags, err := cli.Tags(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	ms := r.state[name]
	now := time.Now()
	ms.lastChecked = now
	if err != nil {
		return
	}
	ms.reachable = true
	ms.down = false
	ms.failures = 0
	ms.neverReached = false
	ms.names = make([]string, 0, len(tags))
	ms.models = make([]ModelInfo, 0, len(tags))
	for _, t := range tags {
		ms.names = append(ms.names, t.Name)
		ms.models = append(ms.models, ModelInfo{Name: t.Name, SizeBytes: t.Size})
	}
	if loaded, err := cli.PS(ctx); err == nil {
		ms.loaded = loaded
	}
}

func (r *Registry) checkOne(ctx context.Context, name string) {
	if r.InFlight(name) > 0 {
		return
	}
	cli := r.client(r.mustMachine(name))
	ctx2, cancel := context.WithTimeout(ctx, r.cfg.Health.Timeout.Duration)
	_, err := cli.Tags(ctx2)
	cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	ms := r.state[name]
	now := time.Now()
	ms.lastChecked = now
	if err != nil {
		ms.failures++
		if ms.failures >= r.cfg.Health.FailuresToMarkDown {
			ms.down = true
			ms.reachable = false
		}
		return
	}
	ms.failures = 0
	ms.down = false
	ms.reachable = true
	ms.neverReached = false
}

func (r *Registry) mustMachine(name string) config.Machine {
	for _, m := range r.cfg.Machines {
		if m.Name == name {
			return m
		}
	}
	panic("unknown machine")
}

// MarkDownImmediate marks a machine down after connection failure.
// IsDown reports whether the machine is marked down.
func (r *Registry) IsDown(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state[name].down
}

func (r *Registry) MarkDownImmediate(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ms := r.state[name]
	ms.down = true
	ms.reachable = false
	ms.failures = r.cfg.Health.FailuresToMarkDown
}

// RefreshMachine refreshes tags for one machine.
func (r *Registry) RefreshMachine(ctx context.Context, name string) {
	r.refreshOne(ctx, name)
}

func (r *Registry) InFlight(name string) int {
	if r.gate != nil {
		return r.gate.InFlight(name)
	}
	return 0
}

func (r *Registry) RouteSnapshot() route.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var machines []route.MachineState
	for _, m := range r.cfg.Machines {
		ms := r.state[m.Name]
		inFlight, waiting := 0, 0
		if r.gate != nil {
			inFlight = r.gate.InFlight(m.Name)
			waiting = r.gate.Waiting(m.Name)
		}
		machines = append(machines, route.MachineState{
			Name:         m.Name,
			Roles:        append([]string{}, m.Roles...),
			DefaultModel: m.DefaultModel,
			Models:       append([]string{}, ms.names...),
			Down:         ms.down,
			NeverReached: ms.neverReached,
			InFlight:     inFlight,
			Waiting:      waiting,
		})
	}
	return route.Snapshot{
		Machines:        machines,
		AutoRole:        r.cfg.Routing.AutoRole,
		FastRole:        r.cfg.Routing.FastRole,
		RefreshedModels: false,
	}
}

func (r *Registry) RouteSnapshotRefreshed() route.Snapshot {
	s := r.RouteSnapshot()
	s.RefreshedModels = true
	return s
}

func (r *Registry) ListMachines() []MachineView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MachineView, 0, len(r.cfg.Machines))
	for _, m := range r.cfg.Machines {
		ms := r.state[m.Name]
		var last *time.Time
		if !ms.lastChecked.IsZero() {
			t := ms.lastChecked.UTC()
			last = &t
		}
		busy := r.gate != nil && r.gate.Busy(m.Name)
		waiting := 0
		if r.gate != nil {
			waiting = r.gate.Waiting(m.Name)
		}
		out = append(out, MachineView{
			Name:         m.Name,
			Roles:        append([]string{}, m.Roles...),
			Reachable:    ms.reachable && !ms.down,
			Busy:         busy,
			Waiting:      waiting,
			DefaultModel: m.DefaultModel,
			Models:       append([]ModelInfo{}, ms.models...),
			LoadedModels: append([]string{}, ms.loaded...),
			LastChecked:  last,
		})
	}
	return out
}

type ModelAggregate struct {
	Name     string   `json:"name"`
	Machines []string `json:"machines"`
}

func (r *Registry) ListModels() []ModelAggregate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byName := map[string][]string{}
	order := []string{}
	for _, m := range r.cfg.Machines {
		ms := r.state[m.Name]
		for _, n := range ms.names {
			if _, ok := byName[n]; !ok {
				order = append(order, n)
			}
			byName[n] = append(byName[n], m.Name)
		}
	}
	out := make([]ModelAggregate, 0, len(order))
	for _, n := range order {
		out = append(out, ModelAggregate{Name: n, Machines: byName[n]})
	}
	return out
}

func (r *Registry) NumCtx(machine string) int {
	for _, m := range r.cfg.Machines {
		if m.Name == machine {
			return m.NumCtx
		}
	}
	return 8192
}
