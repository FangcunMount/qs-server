package scheduler

import (
	"context"
	"sync"
)

// Each runner owns one existing loop. Waiting never creates another goroutine
// and cancellation is not mistaken for completion of its current database work.
type runnerLifecycle struct {
	mu   sync.Mutex
	done chan struct{}
}

func (l *runnerLifecycle) launch(work func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done != nil {
		return
	}
	l.done = make(chan struct{})
	go func() { defer close(l.done); work() }()
}
func (l *runnerLifecycle) Wait(ctx context.Context) error {
	l.mu.Lock()
	done := l.done
	l.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return ctx.Err()
		}
	}
}
