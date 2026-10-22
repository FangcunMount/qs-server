// Package subsystem owns the worker process resilience runtime.
package subsystem

import (
	"context"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
)

type Options struct {
	InstanceID string
	Locks      *locksubsystem.Subsystem
	StateStore control.StateStore
}

type Subsystem struct {
	identity control.InstanceIdentity
	locks    *locksubsystem.Subsystem
	store    control.StateStore
	mu       sync.Mutex
	started  bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func New(opts Options) (*Subsystem, error) {
	identity, err := control.ResolveInstanceIdentity("worker", opts.InstanceID)
	if err != nil {
		return nil, err
	}
	return &Subsystem{identity: identity, locks: opts.Locks, store: opts.StateStore}, nil
}

func (s *Subsystem) Start(parent context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	if s == nil {
		return cancel
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		cancel()
		return s.cancel
	}
	s.started = true
	s.cancel = cancel
	s.done = make(chan struct{})
	heartbeater, ok := s.store.(control.InstanceHeartbeater)
	if !ok {
		close(s.done)
		return cancel
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			_ = heartbeater.Heartbeat(ctx, s.identity, 5*time.Second)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

// Wait joins the actual borrowed Redis operation. Cancellation alone does not
// prove the heartbeater has stopped using the host connection.
func (s *Subsystem) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
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

func (s *Subsystem) Snapshot(now time.Time) resilience.RuntimeSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	snapshot := resilience.NewRuntimeSnapshot("worker", now)
	if s == nil {
		return resilience.FinalizeRuntimeSnapshot(snapshot)
	}
	snapshot.InstanceID, snapshot.Generation = s.identity.InstanceID, s.identity.Generation
	if s.locks != nil {
		snapshot.Locks = s.locks.Snapshots()
	}
	configured := healthyLockCapability(snapshot.Locks, string(locklease.WorkloadAnswersheetProcessing))
	reason := ""
	if !configured {
		reason = "worker duplicate suppression lock manager unavailable"
	}
	snapshot.DuplicateSuppression = []resilience.CapabilitySnapshot{{
		Name: "answersheet_submitted", Kind: resilience.ProtectionDuplicateSuppression.String(), Strategy: "redis_lock",
		Configured: configured, Degraded: !configured, Reason: reason,
	}}
	return resilience.FinalizeRuntimeSnapshot(snapshot)
}

func healthyLockCapability(locks []resilience.CapabilitySnapshot, name string) bool {
	for _, capability := range locks {
		if capability.Name == name {
			return capability.Configured && !capability.Degraded
		}
	}
	return false
}
