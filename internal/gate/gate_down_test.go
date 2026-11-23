package gate

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireFailsWhenDown(t *testing.T) {
	g := New(map[string]int{"m": 1})
	ctx := context.Background()
	_ = g.Acquire(ctx, "m", nil, nil)
	var down atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- g.Acquire(ctx, "m", nil, down.Load)
	}()
	time.Sleep(50 * time.Millisecond)
	down.Store(true)
	select {
	case err := <-done:
		if err != ErrMachineDown {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	g.Release("m")
}
