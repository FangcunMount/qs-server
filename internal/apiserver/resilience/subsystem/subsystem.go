// Package subsystem owns the apiserver process resilience runtime.
package subsystem

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

const (
	BudgetQuery       ratelimit.BudgetID = "query"
	BudgetSubmit      ratelimit.BudgetID = "submit"
	BudgetAdminSubmit ratelimit.BudgetID = "admin_submit"
	BudgetWaitReport  ratelimit.BudgetID = "wait_report"
)

type Options struct {
	InstanceID   string
	RateLimit    *options.RateLimitOptions
	Backpressure map[string]backpressure.Acquirer
	Locks        *locksubsystem.Subsystem
	StateStore   resiliencecontrol.StateStore
}

type Subsystem struct {
	identity     resiliencecontrol.InstanceIdentity
	rateEnabled  bool
	budgets      map[ratelimit.BudgetID]*ratelimit.Budget
	backpressure map[string]backpressure.Acquirer
	locks        *locksubsystem.Subsystem
	stateStore   resiliencecontrol.StateStore
}

func New(opts Options) *Subsystem {
	cfg := opts.RateLimit
	if cfg == nil {
		cfg = options.NewRateLimitOptions()
	}
	s := &Subsystem{
		identity:     resiliencecontrol.ResolveInstanceIdentity("apiserver", opts.InstanceID),
		rateEnabled:  cfg.Enabled,
		budgets:      make(map[ratelimit.BudgetID]*ratelimit.Budget),
		backpressure: cloneBackpressure(opts.Backpressure),
		locks:        opts.Locks,
		stateStore:   opts.StateStore,
	}
	s.budgets[BudgetQuery] = newLocalBudget(BudgetQuery, cfg.QueryGlobalQPS, cfg.QueryGlobalBurst, cfg.QueryUserQPS, cfg.QueryUserBurst)
	s.budgets[BudgetSubmit] = newLocalBudget(BudgetSubmit, cfg.SubmitGlobalQPS, cfg.SubmitGlobalBurst, cfg.SubmitUserQPS, cfg.SubmitUserBurst)
	s.budgets[BudgetAdminSubmit] = newLocalBudget(BudgetAdminSubmit, cfg.AdminSubmitGlobalQPS, cfg.AdminSubmitGlobalBurst, cfg.AdminSubmitUserQPS, cfg.AdminSubmitUserBurst)
	s.budgets[BudgetWaitReport] = newLocalBudget(BudgetWaitReport, cfg.WaitReportGlobalQPS, cfg.WaitReportGlobalBurst, cfg.WaitReportUserQPS, cfg.WaitReportUserBurst)
	return s
}

// Start runs control-state reconciliation and instance heartbeats. Data-plane
// policies remain usable when the control plane is unavailable.
func (s *Subsystem) Start(parent context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	if s == nil || s.stateStore == nil {
		return cancel
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var signals <-chan string
		if watcher, ok := s.stateStore.(resiliencecontrol.StateSignalWatcher); ok {
			signals, _ = watcher.WatchStateSignals(ctx)
		}
		for {
			s.reconcile(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case _, ok := <-signals:
				if !ok {
					signals = nil
				}
			}
		}
	}()
	return cancel
}

func (s *Subsystem) reconcile(ctx context.Context) {
	if heartbeater, ok := s.stateStore.(resiliencecontrol.InstanceHeartbeater); ok {
		_ = heartbeater.Heartbeat(ctx, s.identity, 5*time.Second)
	}
	for _, id := range []ratelimit.BudgetID{BudgetQuery, BudgetSubmit, BudgetAdminSubmit, BudgetWaitReport} {
		budget := s.budgets[id]
		state, exists, err := s.stateStore.Load(ctx, rateStateName("apiserver", string(id)))
		if err != nil {
			continue
		}
		current := budget.Snapshot()
		if !exists {
			if current.Source == "governance" {
				_, _ = budget.Reset(current.Version)
			}
			continue
		}
		if state.Version <= current.Version {
			continue
		}
		var change resiliencecontrol.RateLimitChange
		if json.Unmarshal(state.Payload, &change) != nil {
			continue
		}
		if change.Mode == "reset" || change.Mode == "config" {
			_, _ = budget.ReconcileBaseline(state.Version)
			continue
		}
		if change.Mode != "override" || !change.Global.Valid() || !change.User.Valid() {
			continue
		}
		policy := current.Policy
		policy.Global = overridePolicy(policy.Global, change.Global)
		policy.User = overridePolicy(policy.User, change.User)
		_, _ = budget.Reconcile(state.Version, policy, "governance", state.ExpiresAt)
	}
	s.reconcileLeaderCooldowns(ctx)
	s.processCommands(ctx)
}

func (s *Subsystem) reconcileLeaderCooldowns(ctx context.Context) {
	if s.locks == nil {
		return
	}
	for _, capability := range s.locks.Capabilities() {
		if capability.Kind != locklease.KindLeader {
			continue
		}
		state, exists, err := s.stateStore.Load(ctx, leaderStateName("apiserver", s.identity.InstanceID, string(capability.ID)))
		if err != nil || !exists {
			continue
		}
		_ = s.locks.ApplyLeaderCooldown(capability.ID, state.ExpiresAt)
	}
}

func newLocalBudget(id ratelimit.BudgetID, globalQPS float64, globalBurst int, userQPS float64, userBurst int) *ratelimit.Budget {
	return ratelimit.NewBudget(id, ratelimit.BudgetPolicy{
		Global: ratePolicy(string(id), "global", "local", globalQPS, globalBurst),
		User:   ratePolicy(string(id), "user", "local_key", userQPS, userBurst),
	}, ratelimit.BudgetOptions{ConservativeTransition: time.Second})
}

func ratePolicy(scope, resource, strategy string, qps float64, burst int) ratelimit.RateLimitPolicy {
	return ratelimit.RateLimitPolicy{
		Component: "apiserver", Scope: scope, Resource: resource, Strategy: strategy,
		RatePerSecond: qps, Burst: burst,
	}
}

func cloneBackpressure(source map[string]backpressure.Acquirer) map[string]backpressure.Acquirer {
	result := make(map[string]backpressure.Acquirer, len(source))
	for name, limiter := range source {
		result[name] = limiter
	}
	return result
}

func (s *Subsystem) Budget(id ratelimit.BudgetID) (ratelimit.RateBudget, bool) {
	if s == nil || !s.rateEnabled {
		return ratelimit.RateBudget{}, false
	}
	budget, ok := s.budgets[id]
	if !ok || budget == nil {
		return ratelimit.RateBudget{}, false
	}
	return budget.Limiters(), true
}

func (s *Subsystem) RateBudget(id ratelimit.BudgetID) (*ratelimit.Budget, bool) {
	if s == nil {
		return nil, false
	}
	budget, ok := s.budgets[id]
	return budget, ok
}

func (s *Subsystem) Backpressure(name string) backpressure.Acquirer {
	if s == nil {
		return nil
	}
	return s.backpressure[name]
}

func (s *Subsystem) Snapshot(now time.Time) resilienceplane.RuntimeSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	snapshot := resilienceplane.NewRuntimeSnapshot("apiserver", now)
	if s == nil {
		return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
	}
	snapshot.InstanceID = s.identity.InstanceID
	snapshot.Generation = s.identity.Generation
	for _, id := range []ratelimit.BudgetID{BudgetQuery, BudgetSubmit, BudgetAdminSubmit, BudgetWaitReport} {
		budget := s.budgets[id]
		if budget == nil {
			continue
		}
		policy := budget.Snapshot()
		snapshot.RateLimits = append(snapshot.RateLimits,
			rateSnapshot(id, "global", s.rateEnabled, policy.Version, policy.Source, policy.ExpiresAt, policy.Policy.Global),
			rateSnapshot(id, "user", s.rateEnabled, policy.Version, policy.Source, policy.ExpiresAt, policy.Policy.User),
		)
	}
	for _, name := range []string{"mysql", "mongo", "iam"} {
		snapshot.Backpressure = append(snapshot.Backpressure, backpressureSnapshot(name, s.backpressure[name]))
	}
	if s.locks != nil {
		snapshot.Locks = s.locks.Snapshots()
	}
	return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
}

func rateSnapshot(id ratelimit.BudgetID, dimension string, configured bool, version uint64, source string, expiresAt time.Time, policy ratelimit.RateLimitPolicy) resilienceplane.CapabilitySnapshot {
	return resilienceplane.CapabilitySnapshot{
		Name: string(id) + "_" + dimension, Kind: resilienceplane.ProtectionRateLimit.String(),
		Strategy: policy.Strategy, Configured: configured, RatePerSecond: policy.RatePerSecond,
		Burst: policy.Burst, PolicyVersion: version, PolicySource: source, OverrideExpiresAt: expiresAt,
	}
}

type backpressureSnapshotter interface {
	Snapshot(name string) resilienceplane.BackpressureSnapshot
}

func backpressureSnapshot(name string, limiter backpressure.Acquirer) resilienceplane.BackpressureSnapshot {
	if snapshotter, ok := limiter.(backpressureSnapshotter); ok {
		return snapshotter.Snapshot(name)
	}
	return resilienceplane.BackpressureSnapshot{Component: "apiserver", Name: name, Dependency: name, Strategy: "semaphore", Enabled: limiter != nil}
}

var _ ratelimit.RateBudgetProvider = (*Subsystem)(nil)
