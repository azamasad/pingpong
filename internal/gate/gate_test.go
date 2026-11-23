package gate

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestFirstComesFirstServed(t *testing.T) {
	g := New(map[string]int{"m": 1})
	ctx := context.Background()
	if err := g.Acquire(ctx, "m", nil); err != nil {
		t.Fatal(err)
	}
	if !g.Busy("m") {
		t.Fatal("busy")
	}
	done := make(chan struct{})
	go func() {
		_ = g.Acquire(ctx, "m", nil)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("second should wait")
	default:
	}
	g.Release("m")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestPosition(t *testing.T) {
	g := New(map[string]int{"m": 1})
	ctx := context.Background()
	_ = g.Acquire(ctx, "m", nil)
	var wg sync.WaitGroup
	var positions []int
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Acquire(ctx, "m", func(p int) {
				mu.Lock()
				positions = append(positions, p)
				mu.Unlock()
			})
			g.Release("m")
		}()
	}
	time.Sleep(30 * time.Millisecond)
	g.Release("m")
	wg.Wait()
	if len(positions) != 2 {
		t.Fatalf("positions %v", positions)
	}
}
