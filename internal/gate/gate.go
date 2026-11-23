package gate

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrMachineDown = errors.New("machine down")

// Gate limits concurrent in-flight work per machine and queues excess.
type Gate struct {
	mu       sync.Mutex
	limit    map[string]int
	inFlight map[string]int
	queues   map[string][]waiter
}

type waiter struct {
	ch   chan struct{}
	done bool
}

func New(limits map[string]int) *Gate {
	g := &Gate{
		limit:    make(map[string]int),
		inFlight: make(map[string]int),
		queues:   make(map[string][]waiter),
	}
	for k, v := range limits {
		g.limit[k] = v
		g.inFlight[k] = 0
		g.queues[k] = nil
	}
	return g
}

func (g *Gate) Busy(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	lim := g.limit[name]
	if lim <= 0 {
		lim = 1
	}
	return g.inFlight[name] >= lim
}

func (g *Gate) Waiting(name string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.queues[name])
}

func (g *Gate) InFlight(name string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inFlight[name]
}

// Acquire blocks until a slot is available, ctx is cancelled, or isDown() is true while waiting.
func (g *Gate) Acquire(ctx context.Context, machine string, positionFn func(int), isDown func() bool) error {
	g.mu.Lock()
	lim := g.limit[machine]
	if lim <= 0 {
		lim = 1
	}
	if g.inFlight[machine] < lim {
		g.inFlight[machine]++
		g.mu.Unlock()
		return nil
	}
	w := waiter{ch: make(chan struct{})}
	g.queues[machine] = append(g.queues[machine], w)
	pos := len(g.queues[machine])
	g.mu.Unlock()
	if positionFn != nil {
		positionFn(pos)
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			g.removeWaiter(machine, w.ch)
			return ctx.Err()
		case <-w.ch:
			return nil
		case <-tick.C:
			if isDown != nil && isDown() {
				g.removeWaiter(machine, w.ch)
				return ErrMachineDown
			}
			if positionFn != nil {
				g.mu.Lock()
				p := indexWaiter(g.queues[machine], w.ch)
				if p > 0 {
					positionFn(p)
				}
				g.mu.Unlock()
			}
		}
	}
}

func indexWaiter(q []waiter, ch chan struct{}) int {
	for i, w := range q {
		if w.ch == ch {
			return i + 1
		}
	}
	return 0
}

func (g *Gate) removeWaiter(machine string, ch chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	q := g.queues[machine]
	for i, w := range q {
		if w.ch == ch {
			g.queues[machine] = append(q[:i], q[i+1:]...)
			return
		}
	}
}

func (g *Gate) Release(machine string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight[machine] > 0 {
		g.inFlight[machine]--
	}
	if len(g.queues[machine]) > 0 {
		w := g.queues[machine][0]
		g.queues[machine] = g.queues[machine][1:]
		g.inFlight[machine]++
		close(w.ch)
	}
}
