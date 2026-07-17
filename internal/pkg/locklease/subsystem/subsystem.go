// Package subsystem owns process-local lock lease composition and execution.
package subsystem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redisobserve "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

type ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type realTicker struct{ ticker *time.Ticker }

func (t realTicker) Chan() <-chan time.Time { return t.ticker.C }
func (t realTicker) Stop()                  { t.ticker.Stop() }

type tickerFactory func(time.Duration) ticker

type renewalStopReason uint8

const (
	renewalStoppedByBody renewalStopReason = iota
	renewalStoppedByParent
	renewalStoppedByLeaseLoss
	renewalStoppedByRenewFailure
)

type renewalOutcome struct {
	reason renewalStopReason
	err    error
}

// Options configures one process-local lock lease subsystem.
type Options struct {
	Component        string
	Handle           *redisruntime.Handle
	StatusRegistry   *redisobserve.FamilyStatusRegistry
	Manager          locklease.RenewableManager
	RenewalEnabled   bool
	EnabledWorkloads map[locklease.WorkloadID]bool
	Warn             func(message string)
	tickerFactory    tickerFactory
}

// Subsystem owns the catalog binding, Redis adapter, key builder and lease runner.
type Subsystem struct {
	component      string
	handle         *redisruntime.Handle
	manager        locklease.RenewableManager
	renewalEnabled bool
	enabled        map[locklease.WorkloadID]bool
	statusRegistry *redisobserve.FamilyStatusRegistry
	warn           func(string)
	tickerFactory  tickerFactory
}

func New(opts Options) *Subsystem {
	factory := opts.tickerFactory
	if factory == nil {
		factory = func(interval time.Duration) ticker {
			return realTicker{ticker: time.NewTicker(interval)}
		}
	}
	manager := opts.Manager
	if manager == nil {
		manager = redisadapter.NewManagerWithObservers(
			opts.Component,
			"lock_lease",
			opts.Handle,
			nil,
			redisobserve.NewComponentObserver(opts.Component, opts.StatusRegistry),
		)
	}
	return &Subsystem{
		component:      opts.Component,
		handle:         opts.Handle,
		manager:        manager,
		renewalEnabled: opts.RenewalEnabled,
		enabled:        copyBindings(opts.EnabledWorkloads),
		statusRegistry: opts.StatusRegistry,
		warn:           opts.Warn,
		tickerFactory:  factory,
	}
}

func copyBindings(source map[locklease.WorkloadID]bool) map[locklease.WorkloadID]bool {
	if source == nil {
		return nil
	}
	result := make(map[locklease.WorkloadID]bool, len(source))
	for id, enabled := range source {
		result[id] = enabled
	}
	return result
}

func (s *Subsystem) Manager() locklease.RenewableManager {
	if s == nil {
		return nil
	}
	return s.manager
}

func (s *Subsystem) AcquireSpec(ctx context.Context, spec locklease.Spec, key string, ttlOverride ...time.Duration) (*locklease.Lease, bool, error) {
	if s == nil || s.manager == nil {
		return nil, false, fmt.Errorf("lock lease subsystem is unavailable")
	}
	return s.manager.AcquireSpec(ctx, spec, key, ttlOverride...)
}

func (s *Subsystem) RenewSpec(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease, ttlOverride ...time.Duration) (bool, error) {
	if s == nil || s.manager == nil {
		return false, fmt.Errorf("lock lease subsystem is unavailable")
	}
	return s.manager.RenewSpec(ctx, spec, key, lease, ttlOverride...)
}

func (s *Subsystem) ReleaseSpec(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
	if s == nil || s.manager == nil {
		return nil
	}
	return s.manager.ReleaseSpec(ctx, spec, key, lease)
}

func (s *Subsystem) Builder() *keyspace.Builder {
	if s == nil || s.handle == nil {
		return nil
	}
	return s.handle.Builder
}

func (s *Subsystem) RenewalEnabled() bool {
	return s != nil && s.renewalEnabled
}

func (s *Subsystem) Capabilities() []locklease.Capability {
	if s == nil {
		return nil
	}
	all := locklease.All()
	result := make([]locklease.Capability, 0, len(all))
	for _, capability := range all {
		if capability.Component == s.component {
			result = append(result, capability)
		}
	}
	return result
}

// Snapshots projects the catalog and current lock Redis family health without exposing keys.
func (s *Subsystem) Snapshots() []resilienceplane.CapabilitySnapshot {
	if s == nil {
		return nil
	}
	configured, degraded, reason := s.familyHealth()
	capabilities := s.Capabilities()
	result := make([]resilienceplane.CapabilitySnapshot, 0, len(capabilities))
	for _, capability := range capabilities {
		enabled := true
		if s.enabled != nil {
			enabled = s.enabled[capability.ID]
		}
		itemConfigured := enabled && configured
		itemDegraded := enabled && degraded
		itemReason := reason
		if !enabled {
			itemReason = "workload disabled"
			itemDegraded = false
		}
		renewEvery := int64(0)
		if s.renewalEnabled {
			renewEvery = int64((capability.Spec.DefaultTTL / 3).Seconds())
		}
		result = append(result, resilienceplane.CapabilitySnapshot{
			Name:              capability.Spec.Name,
			Kind:              string(capability.Kind),
			Strategy:          "redis_lease",
			Configured:        itemConfigured,
			Degraded:          itemDegraded,
			Reason:            itemReason,
			TTLSeconds:        int64(capability.Spec.DefaultTTL.Seconds()),
			RenewalMode:       renewalMode(s.renewalEnabled),
			RenewEverySeconds: renewEvery,
		})
	}
	return result
}

func renewalMode(enabled bool) string {
	if enabled {
		return string(locklease.RenewalModeAuto)
	}
	return "disabled"
}

func (s *Subsystem) familyHealth() (configured bool, degraded bool, reason string) {
	if s == nil || s.handle == nil || s.handle.Client == nil {
		return false, true, "lock_lease Redis family unavailable"
	}
	configured = s.handle.Configured
	if !configured && s.handle.Client != nil {
		configured = true
	}
	degraded = s.handle.Degraded || !s.handle.Available
	if s.handle.LastError != nil {
		reason = s.handle.LastError.Error()
	}
	if s.statusRegistry != nil {
		for _, status := range s.statusRegistry.Snapshot() {
			if status.Component == s.component && status.Family == string(redisruntime.FamilyLock) {
				configured = status.Configured
				degraded = status.Degraded || !status.Available
				reason = status.LastError
				break
			}
		}
	}
	if degraded && reason == "" {
		reason = "lock_lease Redis family unavailable"
	}
	return configured, degraded, reason
}

func (s *Subsystem) Run(
	ctx context.Context,
	workload locklease.WorkloadID,
	key string,
	ttl time.Duration,
	body func(context.Context) error,
) (result locklease.RunResult, runErr error) {
	capability, ok := locklease.Lookup(workload)
	if !ok {
		return result, fmt.Errorf("unknown lock lease workload %q", workload)
	}
	if s == nil || s.manager == nil {
		return result, fmt.Errorf("%w: workload %s: manager unavailable", locklease.ErrLeaseAcquireFailed, workload)
	}
	if s.component != "" && capability.Component != s.component {
		return result, fmt.Errorf("lock lease workload %q belongs to %q, not %q", workload, capability.Component, s.component)
	}
	if ttl <= 0 {
		ttl = capability.Spec.DefaultTTL
	}
	if cause := context.Cause(ctx); cause != nil {
		return result, cause
	}

	lease, acquired, err := s.manager.AcquireSpec(ctx, capability.Spec, key, ttl)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return result, cause
		}
		return result, fmt.Errorf("%w: workload %s: %w", locklease.ErrLeaseAcquireFailed, workload, err)
	}
	if !acquired {
		return result, nil
	}
	result.Acquired = true
	defer func() {
		result.ReleaseErr = s.manager.ReleaseSpec(context.Background(), capability.Spec, key, lease)
	}()

	if body == nil {
		return result, nil
	}
	if !s.renewalEnabled {
		return result, body(ctx)
	}

	interval := ttl / 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
	bodyCtx, cancelBody := context.WithCancelCause(ctx)
	defer cancelBody(nil)
	renewCtx, cancelRenew := context.WithCancel(bodyCtx)
	defer cancelRenew()
	done := make(chan struct{})
	renewResult := make(chan renewalOutcome, 1)
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(done) }) }

	go s.renewLoop(ctx, renewCtx, cancelBody, done, renewResult, capability, key, lease, ttl, interval)
	bodyErr := body(bodyCtx)
	stop()
	cancelRenew()
	outcome := <-renewResult
	switch outcome.reason {
	case renewalStoppedByLeaseLoss, renewalStoppedByRenewFailure:
		if bodyErr == nil || cancelerr.Is(bodyErr) {
			return result, outcome.err
		}
	case renewalStoppedByParent:
		if bodyErr == nil || cancelerr.Is(bodyErr) {
			return result, outcome.err
		}
	}
	return result, bodyErr
}

func (s *Subsystem) renewLoop(
	parentCtx context.Context,
	renewCtx context.Context,
	cancelBody context.CancelCauseFunc,
	done <-chan struct{},
	result chan<- renewalOutcome,
	capability locklease.Capability,
	key string,
	lease *locklease.Lease,
	ttl time.Duration,
	interval time.Duration,
) {
	ticker := s.tickerFactory(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			result <- renewalOutcome{reason: renewalStoppedByBody}
			return
		case <-parentCtx.Done():
			cause := context.Cause(parentCtx)
			result <- renewalOutcome{reason: renewalStoppedByParent, err: cause}
			s.warnUntilDone(done, ticker.Chan(), capability.ID, cause)
			return
		case <-renewCtx.Done():
			if outcome, stopped := renewalStoppedOutcome(parentCtx, done); stopped {
				result <- outcome
				if outcome.reason == renewalStoppedByParent {
					s.warnUntilDone(done, ticker.Chan(), capability.ID, outcome.err)
				}
				return
			}
			result <- renewalOutcome{reason: renewalStoppedByBody}
			return
		case <-ticker.Chan():
			owned, err := s.manager.RenewSpec(renewCtx, capability.Spec, key, lease, ttl)
			if outcome, stopped := renewalStoppedOutcome(parentCtx, done); stopped {
				result <- outcome
				if outcome.reason == renewalStoppedByParent {
					s.warnUntilDone(done, ticker.Chan(), capability.ID, outcome.err)
				}
				return
			}
			if err == nil && owned {
				continue
			}
			var renewErr error
			if err != nil {
				renewErr = fmt.Errorf("%w: workload %s: %w", locklease.ErrLeaseRenewFailed, capability.ID, err)
			} else {
				renewErr = fmt.Errorf("%w: workload %s", locklease.ErrLeaseLost, capability.ID)
			}
			reason := renewalStoppedByLeaseLoss
			if err != nil {
				reason = renewalStoppedByRenewFailure
			}
			cancelBody(renewErr)
			result <- renewalOutcome{reason: reason, err: renewErr}
			s.warnUntilDone(done, ticker.Chan(), capability.ID, renewErr)
			return
		}
	}
}

func renewalStoppedOutcome(parentCtx context.Context, done <-chan struct{}) (renewalOutcome, bool) {
	select {
	case <-done:
		return renewalOutcome{reason: renewalStoppedByBody}, true
	default:
	}
	if cause := context.Cause(parentCtx); cause != nil {
		return renewalOutcome{reason: renewalStoppedByParent, err: cause}, true
	}
	return renewalOutcome{}, false
}

func (s *Subsystem) warnUntilDone(done <-chan struct{}, ticks <-chan time.Time, workload locklease.WorkloadID, cause error) {
	for {
		select {
		case <-done:
			return
		case <-ticks:
			if s.warn != nil {
				s.warn(fmt.Sprintf("lock lease component %s workload %s has not stopped after cancellation: %v", s.component, workload, cause))
			}
		}
	}
}
