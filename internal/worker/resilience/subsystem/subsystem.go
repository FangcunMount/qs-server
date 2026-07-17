// Package subsystem owns the worker process resilience runtime.
package subsystem

import (
	"context"
	"time"

	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

type Options struct {
	InstanceID string
	Locks      *locksubsystem.Subsystem
	StateStore resiliencecontrol.StateStore
}

type Subsystem struct {
	identity resiliencecontrol.InstanceIdentity
	locks    *locksubsystem.Subsystem
	store    resiliencecontrol.StateStore
}

func New(opts Options) *Subsystem {
	return &Subsystem{identity: resiliencecontrol.ResolveInstanceIdentity("worker", opts.InstanceID), locks: opts.Locks, store: opts.StateStore}
}

func (s *Subsystem) Start(parent context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	if s == nil {
		return cancel
	}
	heartbeater, ok := s.store.(resiliencecontrol.InstanceHeartbeater)
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

func (s *Subsystem) Snapshot(now time.Time) resilienceplane.RuntimeSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	snapshot := resilienceplane.NewRuntimeSnapshot("worker", now)
	if s == nil {
		return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
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
	snapshot.DuplicateSuppression = []resilienceplane.CapabilitySnapshot{{
		Name: "answersheet_submitted", Kind: resilienceplane.ProtectionDuplicateSuppression.String(), Strategy: "redis_lock",
		Configured: configured, Degraded: !configured, Reason: reason,
	}}
	return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
}
