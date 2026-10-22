package subsystem

import (
	"context"
	"errors"
)

// ShutdownIncomplete reports the single owned close coordinator still running.
// It does not mean subscriber/RPC capacity has been released. Wait can observe
// its real completion without spawning another closer or closing host pools.
type ShutdownIncomplete struct {
	PendingClosures int
	Cause           error
}

func (*ShutdownIncomplete) Error() string   { return "event subsystem shutdown incomplete" }
func (e *ShutdownIncomplete) Unwrap() error { return e.Cause }

// CloseContext seals startup/registration immediately and owns at most one
// background closer. The existing blocking Close remains available to legacy
// callers; both APIs share exactly the same completion and error result.
func (s *Subsystem) CloseContext(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closeDone == nil {
		s.closeDone = make(chan struct{})
	}
	s.closed = true
	s.mu.Unlock()
	s.closeAsyncOnce.Do(func() { go func() { _ = s.Close() }() })
	return s.Wait(ctx)
}

func (s *Subsystem) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	requested, done := s.closed, s.closeDone
	s.mu.Unlock()
	if !requested || done == nil {
		return errors.New("event subsystem shutdown not requested")
	}
	result := func() error { s.mu.Lock(); defer s.mu.Unlock(); return s.closeErr }
	select {
	case <-done:
		return result()
	default:
	}
	select {
	case <-done:
		return result()
	case <-ctx.Done():
		select {
		case <-done:
			return result()
		default:
		}
		return &ShutdownIncomplete{PendingClosures: 1, Cause: ctx.Err()}
	}
}
