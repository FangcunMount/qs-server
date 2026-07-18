// Package subsystem owns process-local lock lease composition and execution.
package subsystem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redisobserve "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
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
	now              func() time.Time
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
	now            func() time.Time
	activeMu       sync.Mutex
	active         map[locklease.WorkloadID]map[uint64]*activeRun
	cooldownUntil  map[locklease.WorkloadID]time.Time
	nextRunID      uint64
}

type activeRun struct {
	id         uint64
	workload   locklease.WorkloadID
	cancel     context.CancelCauseFunc
	done       chan struct{}
	releaseErr error
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
	now := opts.now
	if now == nil {
		now = time.Now
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
		now:            now,
		active:         make(map[locklease.WorkloadID]map[uint64]*activeRun),
		cooldownUntil:  make(map[locklease.WorkloadID]time.Time),
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
func (s *Subsystem) Snapshots() []resilience.CapabilitySnapshot {
	if s == nil {
		return nil
	}
	configured, degraded, reason := s.familyHealth()
	capabilities := s.Capabilities()
	result := make([]resilience.CapabilitySnapshot, 0, len(capabilities))
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
		result = append(result, resilience.CapabilitySnapshot{
			Name:              capability.Spec.Name,
			Kind:              string(capability.Kind),
			Strategy:          "redis_lease",
			Configured:        itemConfigured,
			Degraded:          itemDegraded,
			Reason:            itemReason,
			TTLSeconds:        int64(capability.Spec.DefaultTTL.Seconds()),
			RenewalMode:       renewalMode(s.renewalEnabled),
			RenewEverySeconds: renewEvery,
			Active:            s.activeCount(capability.ID),
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
	if s.inCooldown(workload) {
		return result, nil
	}

	lease, acquired, err := s.manager.AcquireSpec(ctx, capability.Spec, key, ttl)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return result, cause
		}
		return result, fmt.Errorf("%w: workload %s: %w", locklease.ErrLeaseAcquireFailed, workload, err)
	}
	if !acquired {
		if cause := context.Cause(ctx); cause != nil {
			return result, cause
		}
		return result, nil
	}

	if body == nil {
		_, admitted, admissionErr := s.admitAcquiredLease(ctx, workload, nil)
		if !admitted {
			result.ReleaseErr = s.manager.ReleaseSpec(context.Background(), capability.Spec, key, lease)
			return result, admissionErr
		}
		result.Acquired = true
		result.ReleaseErr = s.manager.ReleaseSpec(context.Background(), capability.Spec, key, lease)
		return result, nil
	}
	bodyCtx, cancelBody := context.WithCancelCause(ctx)
	run, admitted, admissionErr := s.admitAcquiredLease(bodyCtx, workload, cancelBody)
	if !admitted {
		cancelBody(nil)
		result.ReleaseErr = s.manager.ReleaseSpec(context.Background(), capability.Spec, key, lease)
		return result, admissionErr
	}
	result.Acquired = true
	defer cancelBody(nil)
	defer func() {
		result.ReleaseErr = s.manager.ReleaseSpec(context.Background(), capability.Spec, key, lease)
		s.finishActive(run, result.ReleaseErr)
	}()
	if !s.renewalEnabled {
		return result, body(bodyCtx)
	}

	interval := ttl / 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
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

func (s *Subsystem) RelinquishLeader(ctx context.Context, workload locklease.WorkloadID, opts locklease.RelinquishOptions) (locklease.RelinquishResult, error) {
	result := locklease.RelinquishResult{Workload: workload}
	if s == nil {
		return result, fmt.Errorf("lock lease subsystem is unavailable")
	}
	capability, ok := locklease.Lookup(workload)
	if !ok {
		return result, fmt.Errorf("unknown lock lease workload %q", workload)
	}
	if capability.Component != s.component || capability.Kind != locklease.KindLeader {
		return result, fmt.Errorf("workload %q is not a releasable leader lease for component %q", workload, s.component)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = capability.Spec.DefaultTTL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}

	s.activeMu.Lock()
	result.CooldownUntil = s.now().Add(opts.Cooldown)
	s.cooldownUntil[workload] = result.CooldownUntil
	runs := make([]*activeRun, 0, len(s.active[workload]))
	for _, run := range s.active[workload] {
		runs = append(runs, run)
	}
	s.activeMu.Unlock()
	result.ActiveCount = len(runs)
	if len(runs) == 0 {
		return result, nil
	}
	for _, run := range runs {
		run.cancel(locklease.ErrLeaseRelinquished)
	}
	waitCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	for _, run := range runs {
		select {
		case <-run.done:
			result.Relinquished++
			if run.releaseErr != nil {
				result.ReleaseErrors++
			}
		case <-waitCtx.Done():
			return result, waitCtx.Err()
		}
	}
	return result, nil
}

// ApplyLeaderCooldown restores a control-plane cooldown after process restart.
// It never acquires or releases a lease.
func (s *Subsystem) ApplyLeaderCooldown(workload locklease.WorkloadID, until time.Time) error {
	if s == nil {
		return fmt.Errorf("lock lease subsystem is unavailable")
	}
	capability, ok := locklease.Lookup(workload)
	if !ok || capability.Component != s.component || capability.Kind != locklease.KindLeader {
		return fmt.Errorf("workload %q is not a leader lease for component %q", workload, s.component)
	}
	if until.IsZero() || !until.After(s.now()) {
		return nil
	}
	s.activeMu.Lock()
	if current := s.cooldownUntil[workload]; until.After(current) {
		s.cooldownUntil[workload] = until
	}
	s.activeMu.Unlock()
	return nil
}

// admitAcquiredLease is the authoritative admission point after Redis acquire.
// Cooldown publication and active-run registration are linearized by activeMu.
// A nil cancel checks admission without registering an active body.
func (s *Subsystem) admitAcquiredLease(
	ctx context.Context,
	workload locklease.WorkloadID,
	cancel context.CancelCauseFunc,
) (*activeRun, bool, error) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if cause := context.Cause(ctx); cause != nil {
		return nil, false, cause
	}
	if s.cooldownActiveLocked(workload) {
		return nil, false, nil
	}
	if cancel == nil {
		return nil, true, nil
	}
	return s.registerActiveLocked(workload, cancel), true, nil
}

func (s *Subsystem) registerActiveLocked(workload locklease.WorkloadID, cancel context.CancelCauseFunc) *activeRun {
	s.nextRunID++
	run := &activeRun{id: s.nextRunID, workload: workload, cancel: cancel, done: make(chan struct{})}
	if s.active[workload] == nil {
		s.active[workload] = make(map[uint64]*activeRun)
	}
	s.active[workload][run.id] = run
	return run
}

func (s *Subsystem) finishActive(run *activeRun, releaseErr error) {
	if s == nil || run == nil {
		return
	}
	s.activeMu.Lock()
	run.releaseErr = releaseErr
	delete(s.active[run.workload], run.id)
	close(run.done)
	s.activeMu.Unlock()
}

func (s *Subsystem) activeCount(workload locklease.WorkloadID) int {
	if s == nil {
		return 0
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active[workload])
}

func (s *Subsystem) inCooldown(workload locklease.WorkloadID) bool {
	if s == nil {
		return false
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return s.cooldownActiveLocked(workload)
}

func (s *Subsystem) cooldownActiveLocked(workload locklease.WorkloadID) bool {
	until := s.cooldownUntil[workload]
	if until.IsZero() || !s.now().Before(until) {
		delete(s.cooldownUntil, workload)
		return false
	}
	return true
}

var _ locklease.LeaderRelinquisher = (*Subsystem)(nil)

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
