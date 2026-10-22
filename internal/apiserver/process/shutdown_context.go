package process

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
)

// processShutdown owns one real transport/hook/container drain. A caller
// timeout bounds waiting, not the lifetime of work still using host resources.
type processShutdown struct {
	requested atomic.Bool
	deps      processLifecycleDeps
	once      sync.Once
	done      chan struct{}
	result    error
}

func newProcessShutdown(deps processLifecycleDeps) *processShutdown {
	return &processShutdown{deps: deps, done: make(chan struct{})}
}
func (s *processShutdown) Close(ctx context.Context) error {
	s.once.Do(func() {
		s.requested.Store(true)
		if s.deps.container.stopAdmission != nil {
			s.deps.container.stopAdmission()
		}
		go func() {
			defer close(s.done)
			if err := runPrepareRunShutdownHooks(s.deps.runtime.lifecycle); err != nil {
				s.result = err
				return
			}
			s.result = runProcessLifecycleDeps(s.deps)
		}()
	})
	return s.Wait(ctx)
}
func (s *processShutdown) Wait(ctx context.Context) error {
	if !s.requested.Load() {
		return errors.New("process shutdown not requested")
	}
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
		return &eventsubsystem.ShutdownIncomplete{PendingClosures: 1, Cause: ctx.Err()}
	}
}
