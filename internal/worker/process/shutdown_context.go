package process

import (
	"context"
	"sync"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

// A caller's budget bounds waiting. One actual drain retains the subscriber,
// handler clients and pools until its users have really returned.
type workerShutdown struct {
	deps      lifecycleDeps
	once      sync.Once
	requested chan struct{}
	done      chan struct{}
	result    error
}

func newWorkerShutdown(deps lifecycleDeps) *workerShutdown {
	return &workerShutdown{deps: deps, requested: make(chan struct{}), done: make(chan struct{})}
}
func (s *workerShutdown) Close(ctx context.Context) error {
	s.once.Do(func() {
		close(s.requested)
		go func() { defer close(s.done); s.result = runWorkerLifecycle(s.deps) }()
	})
	return s.Wait(ctx)
}
func (s *workerShutdown) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.done:
		return s.result
	default:
	}
	select {
	case <-s.done:
		return s.result
	case <-ctx.Done():
		select {
		case <-s.done:
			return s.result
		default:
		}
		return &transaction.ShutdownIncomplete{RuntimePending: true, Cause: ctx.Err()}
	}
}
