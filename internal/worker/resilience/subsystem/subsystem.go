// Package subsystem owns the worker process resilience runtime.
package subsystem

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
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
	heartbeater, ok := s.store.(control.InstanceHeartbeater)
	if !ok {
		return cancel
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
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
	configured := len(snapshot.Locks) == 1 && snapshot.Locks[0].Configured && !snapshot.Locks[0].Degraded
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
