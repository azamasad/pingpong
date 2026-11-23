package gate

import (
	"context"
	"sync"
)

// Gate limits concurrent in-flight work per machine and queues excess.
type Gate struct {
	mu       sync.Mutex
	limit    map[string]int
	inFlight map[string]int
	queues   map[string][]chan struct{}
}

func New(limits map[string]int) *Gate {
	g := &Gate{
		limit:    make(map[string]int),
		inFlight: make(map[string]int),
		queues:   make(map[string][]chan struct{}),
	}
	for k, v := range limits {
		g.limit[k] = v
		g.inFlight[k] = 0
		g.queues[k] = nil
	}
	return g
}

// Busy reports whether the machine is at its concurrency limit.
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

// InFlight returns current running count.
func (g *Gate) InFlight(name string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inFlight[name]
}

// Acquire blocks until a slot is available or ctx is cancelled.
// positionFn is called with 1-based position when waiting; 0 when immediate.
func (g *Gate) Acquire(ctx context.Context, machine string, positionFn func(int)) error {
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
	ch := make(chan struct{})
	g.queues[machine] = append(g.queues[machine], ch)
	pos := len(g.queues[machine])
	g.mu.Unlock()
	if positionFn != nil {
		positionFn(pos)
	}
	select {
	case <-ctx.Done():
		g.removeWaiter(machine, ch)
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func (g *Gate) removeWaiter(machine string, ch chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	q := g.queues[machine]
	for i, c := range q {
		if c == ch {
			g.queues[machine] = append(q[:i], q[i+1:]...)
			close(ch)
			return
		}
	}
}

// Release frees a slot and wakes the next waiter.
func (g *Gate) Release(machine string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight[machine] > 0 {
		g.inFlight[machine]--
	}
	if len(g.queues[machine]) > 0 {
		ch := g.queues[machine][0]
		g.queues[machine] = g.queues[machine][1:]
		g.inFlight[machine]++
		close(ch)
	}
}

// CancelWaiting removes a waiter without acquiring (client disconnect while waiting).
func (g *Gate) CancelWaiting(machine string, acquired bool) {
	if acquired {
		g.Release(machine)
		return
	}
	// if still waiting, the acquire ctx cancel path handles removal
}
