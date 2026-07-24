// Package subsystem owns the collection-server process resilience runtime.
package subsystem

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
)

const (
	BudgetQuery        ratelimit.BudgetID = "query"
	BudgetSubmit       ratelimit.BudgetID = "submit"
	BudgetWaitReport   ratelimit.BudgetID = "wait_report"
	BudgetReportEvents ratelimit.BudgetID = "report_events"
)

const (
	GateQuery          = "query"
	GateCatalog        = "catalog"
	GateSubmit         = "submit"
	GateWaitReport     = "wait_report"
	GateGRPCDownstream = "grpc_downstream"
)

type Options struct {
	InstanceID     string
	RateLimit      *options.RateLimitOptions
	Concurrency    *options.ConcurrencyOptions
	WaitReport     *options.WaitReportOptions
	GRPCClient     *options.GRPCClientOptions
	Backend        ratelimit.Backend
	Locks          *locksubsystem.Subsystem
	OpsAvailable   bool
	StateStore     control.StateStore
	ControlEnabled *bool
}

type queueRegistration struct {
	controller control.QueueController
	snapshot   func(time.Time) resilience.QueueSnapshot
}

type Subsystem struct {
	identity         control.InstanceIdentity
	rateEnabled      bool
	budgets          map[ratelimit.BudgetID]*ratelimit.Budget
	gates            map[string]*concurrency.Gate
	locks            *locksubsystem.Subsystem
	opsAvailable     bool
	stateStore       control.StateStore
	appliedRate      map[ratelimit.BudgetID]uint64
	queueMu          sync.RWMutex
	queues           map[string]queueRegistration
	commandMu        sync.Mutex
	activeCommands   map[string]struct{}
	controlReady     atomic.Bool
	grpcInflightWait time.Duration
	controlEnabled   bool
	submitFallback   options.SubmitDegradedLocalRateLimitOptions
}

func New(opts Options) (*Subsystem, error) {
	rateCfg := opts.RateLimit
	if rateCfg == nil {
		rateCfg = options.NewRateLimitOptions()
	}
	controlEnabled := true
	if opts.ControlEnabled != nil {
		controlEnabled = *opts.ControlEnabled
	}
	identity, err := control.ResolveInstanceIdentity("collection-server", opts.InstanceID)
	if err != nil {
		return nil, err
	}
	s := &Subsystem{
		identity:    identity,
		rateEnabled: rateCfg.Enabled, budgets: make(map[ratelimit.BudgetID]*ratelimit.Budget),
		gates: make(map[string]*concurrency.Gate), locks: opts.Locks, opsAvailable: opts.OpsAvailable, stateStore: opts.StateStore,
		appliedRate:    make(map[ratelimit.BudgetID]uint64),
		queues:         make(map[string]queueRegistration),
		activeCommands: make(map[string]struct{}),
		controlEnabled: controlEnabled,
		submitFallback: rateCfg.ResolvedSubmitDegradedLocal(),
	}
	s.budgets[BudgetQuery] = newBudget(BudgetQuery, opts.Backend, rateCfg.QueryGlobalQPS, rateCfg.QueryGlobalBurst, rateCfg.QueryUserQPS, rateCfg.QueryUserBurst)
	s.budgets[BudgetSubmit] = newSubmitBudget(opts.Backend, rateCfg)
	s.budgets[BudgetWaitReport] = newBudget(BudgetWaitReport, opts.Backend, rateCfg.WaitReportGlobalQPS, rateCfg.WaitReportGlobalBurst, rateCfg.WaitReportUserQPS, rateCfg.WaitReportUserBurst)
	s.budgets[BudgetReportEvents] = newBudget(BudgetReportEvents, opts.Backend, rateCfg.ReportEventsGlobalQPS, rateCfg.ReportEventsGlobalBurst, rateCfg.ReportEventsUserQPS, rateCfg.ReportEventsUserBurst)
	s.buildGates(opts.Concurrency, opts.WaitReport)
	grpcOptions := opts.GRPCClient
	s.gates[GateGRPCDownstream] = concurrency.NewGate(grpcOptions.ResolvedMaxInflight())
	s.grpcInflightWait = grpcOptions.ResolvedInflightWait()
	if !controlEnabled {
		s.controlReady.Store(true)
	}
	return s, nil
}

// Sync performs the cold-start control-state read before the process may
// report ready. Reconciliation continues asynchronously after this succeeds.
func (s *Subsystem) Sync(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if !s.controlEnabled {
		s.controlReady.Store(true)
		return nil
	}
	if s.stateStore == nil || !s.opsAvailable {
		return control.ErrUnavailable
	}
	if err := s.syncRegisteredQueues(ctx, true); err != nil {
		return err
	}
	s.controlReady.Store(true)
	return nil
}

func (s *Subsystem) ControlSynchronized() bool {
	return s == nil || s.controlReady.Load()
}

func (s *Subsystem) Start(ctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	if s == nil || !s.controlEnabled {
		return cancel
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var signals <-chan string
		if watcher, ok := s.stateStore.(control.StateSignalWatcher); ok {
			signals, _ = watcher.WatchStateSignals(ctx)
		}
		for {
			if !s.controlReady.Load() {
				_ = s.Sync(ctx)
			} else {
				s.reconcile(ctx)
			}
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
	if s == nil || s.stateStore == nil || !s.controlEnabled {
		return
	}
	if heartbeater, ok := s.stateStore.(control.InstanceHeartbeater); ok {
		_ = heartbeater.Heartbeat(ctx, s.identity, 5*time.Second)
	}
	for _, id := range []ratelimit.BudgetID{BudgetQuery, BudgetSubmit, BudgetWaitReport, BudgetReportEvents} {
		state, exists, err := s.stateStore.Load(ctx, "rate:collection-server:"+string(id))
		if err != nil {
			continue
		}
		budget := s.budgets[id]
		if !exists {
			current := budget.Snapshot()
			if current.Source == "governance" {
				_, _ = budget.Reset(current.Version)
			}
			continue
		}
		if state.Version <= s.appliedRate[id] {
			continue
		}
		var change control.RateLimitChange
		if json.Unmarshal(state.Payload, &change) != nil {
			continue
		}
		if change.Mode == "reset" || change.Mode == "config" {
			if _, err := budget.ReconcileBaseline(state.Version); err == nil {
				s.appliedRate[id] = state.Version
			}
			continue
		}
		if change.Mode != "override" || !change.Global.Valid() || !change.User.Valid() {
			continue
		}
		current := budget.Snapshot().Policy
		current.Global.RatePerSecond, current.Global.Burst = change.Global.RatePerSecond, change.Global.Burst
		current.User.RatePerSecond, current.User.Burst = change.User.RatePerSecond, change.User.Burst
		if _, err := budget.Reconcile(state.Version, current, "governance", state.ExpiresAt); err == nil {
			s.appliedRate[id] = state.Version
		}
	}
	if !s.processCommands(ctx) {
		_ = s.syncRegisteredQueues(ctx, false)
	}
}

func (s *Subsystem) syncRegisteredQueues(ctx context.Context, requireSettled bool) error {
	s.queueMu.RLock()
	names := make([]string, 0, len(s.queues))
	for name := range s.queues {
		names = append(names, name)
	}
	s.queueMu.RUnlock()
	for _, name := range names {
		state, exists, err := s.stateStore.Load(ctx, "queue:collection-server:"+name)
		if err != nil {
			return err
		}
		if err := s.applyQueueDesiredState(ctx, state, exists, requireSettled); err != nil {
			return err
		}
	}
	return nil
}

func (s *Subsystem) applyQueueDesiredState(ctx context.Context, state control.VersionedState, exists, requireSettled bool) error {
	if !exists {
		return nil
	}
	var change control.QueueChange
	if err := json.Unmarshal(state.Payload, &change); err != nil {
		return err
	}
	if change.Target != "" && change.Target != "all" && change.Target != s.identity.InstanceID {
		return nil
	}
	s.queueMu.RLock()
	queue, ok := s.queues[change.Queue]
	s.queueMu.RUnlock()
	if !ok {
		return control.ErrUnavailable
	}
	snapshot := queue.snapshot(time.Now())
	switch change.DesiredState {
	case control.QueueStatePaused:
		if snapshot.State == string(control.QueueStatePaused) {
			return nil
		}
		timeout := time.Duration(change.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = time.Minute
		}
		result, err := queue.controller.Drain(ctx, control.DrainOptions{Timeout: timeout})
		if err != nil {
			return err
		}
		if requireSettled && result.State != control.QueueStatePaused {
			return control.ErrInvalidState
		}
		return nil
	case control.QueueStateActive:
		if snapshot.State == string(control.QueueStateActive) {
			return nil
		}
		if snapshot.State == string(control.QueueStatePaused) {
			return queue.controller.Resume(ctx)
		}
		return control.ErrInvalidState
	default:
		return control.ErrInvalidState
	}
}

func newBudget(id ratelimit.BudgetID, backend ratelimit.Backend, globalQPS float64, globalBurst int, userQPS float64, userBurst int) *ratelimit.Budget {
	strategy, userStrategy := "local", "local_key"
	opts := ratelimit.BudgetOptions{ConservativeTransition: time.Second}
	if backend != nil {
		strategy, userStrategy = "redis", "redis"
		opts.ConservativeTransition = 0
		opts.GlobalFactory = func(policy ratelimit.RateLimitPolicy) ratelimit.RateLimiter {
			return ratelimit.NewDistributedLimiter(backend, policy)
		}
		opts.UserFactory = opts.GlobalFactory
	}
	return ratelimit.NewBudget(id, ratelimit.BudgetPolicy{
		Global: ratePolicy(id, "global", strategy, globalQPS, globalBurst),
		User:   ratePolicy(id, "user", userStrategy, userQPS, userBurst),
	}, opts)
}

func newSubmitBudget(backend ratelimit.Backend, cfg *options.RateLimitOptions) *ratelimit.Budget {
	if cfg == nil {
		cfg = options.NewRateLimitOptions()
	}
	fallback := cfg.ResolvedSubmitDegradedLocal()
	if !fallback.Enabled {
		return newBudget(BudgetSubmit, backend, cfg.SubmitGlobalQPS, cfg.SubmitGlobalBurst, cfg.SubmitUserQPS, cfg.SubmitUserBurst)
	}

	opts := ratelimit.BudgetOptions{
		GlobalFactory: func(policy ratelimit.RateLimitPolicy) ratelimit.RateLimiter {
			localPolicy := degradedLocalPolicy(policy, fallback.GlobalQPS, fallback.GlobalBurst)
			return ratelimit.NewDegradedFallbackLimiter(
				ratelimit.NewDistributedLimiter(backend, policy),
				ratelimit.NewLocalLimiter(localPolicy),
			)
		},
		UserFactory: func(policy ratelimit.RateLimitPolicy) ratelimit.RateLimiter {
			localPolicy := degradedLocalPolicy(policy, fallback.UserQPS, fallback.UserBurst)
			return ratelimit.NewDegradedFallbackLimiter(
				ratelimit.NewDistributedLimiter(backend, policy),
				ratelimit.NewKeyedLocalLimiter(localPolicy),
			)
		},
	}
	return ratelimit.NewBudget(BudgetSubmit, ratelimit.BudgetPolicy{
		Global: ratePolicy(BudgetSubmit, "global", "redis", cfg.SubmitGlobalQPS, cfg.SubmitGlobalBurst),
		User:   ratePolicy(BudgetSubmit, "user", "redis", cfg.SubmitUserQPS, cfg.SubmitUserBurst),
	}, opts)
}

func degradedLocalPolicy(primary ratelimit.RateLimitPolicy, configuredQPS float64, configuredBurst int) ratelimit.RateLimitPolicy {
	policy := primary
	policy.Strategy = "local_fallback"
	policy.RatePerSecond = min(primary.RatePerSecond, configuredQPS)
	policy.Burst = min(primary.Burst, configuredBurst)
	return policy
}

func ratePolicy(id ratelimit.BudgetID, resource, strategy string, qps float64, burst int) ratelimit.RateLimitPolicy {
	return ratelimit.RateLimitPolicy{Component: "collection-server", Scope: string(id), Resource: resource, Strategy: strategy, RatePerSecond: qps, Burst: burst}
}

func (s *Subsystem) buildGates(concurrencyOpts *options.ConcurrencyOptions, waitOpts *options.WaitReportOptions) {
	if concurrencyOpts == nil {
		concurrencyOpts = options.NewOptions().Concurrency
	}
	s.gates[GateQuery] = concurrency.NewGate(concurrencyOpts.ResolvedQueryConcurrency())
	s.gates[GateCatalog] = concurrency.NewGate(concurrencyOpts.ResolvedCatalogConcurrency())
	s.gates[GateSubmit] = concurrency.NewGate(concurrencyOpts.ResolvedSubmitConcurrency())
	maxWait, degrade := 400, true
	if waitOpts != nil {
		if waitOpts.MaxHTTPConcurrency > 0 {
			maxWait = waitOpts.MaxHTTPConcurrency
		}
		degrade = waitOpts.DegradeImmediateEnabled
	}
	if degrade {
		s.gates[GateWaitReport] = concurrency.NewGate(maxWait)
	} else {
		s.gates[GateWaitReport] = s.gates[GateQuery]
	}
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

func (s *Subsystem) Gate(name string) *concurrency.Gate {
	if s == nil {
		return nil
	}
	return s.gates[name]
}

func (s *Subsystem) RegisterQueue(name string, controller control.QueueController, snapshot func(time.Time) resilience.QueueSnapshot) {
	if s == nil || name == "" || controller == nil || snapshot == nil {
		return
	}
	s.queueMu.Lock()
	s.queues[name] = queueRegistration{controller: controller, snapshot: snapshot}
	s.queueMu.Unlock()
}

func (s *Subsystem) Queue(name string) (control.QueueController, bool) {
	if s == nil {
		return nil, false
	}
	s.queueMu.RLock()
	defer s.queueMu.RUnlock()
	registration, ok := s.queues[name]
	return registration.controller, ok
}

func (s *Subsystem) Snapshot(now time.Time) resilience.RuntimeSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	snapshot := resilience.NewRuntimeSnapshot("collection-server", now)
	if s == nil {
		return resilience.FinalizeRuntimeSnapshot(snapshot)
	}
	snapshot.InstanceID, snapshot.Generation = s.identity.InstanceID, s.identity.Generation
	for _, id := range []ratelimit.BudgetID{BudgetQuery, BudgetSubmit, BudgetWaitReport, BudgetReportEvents} {
		policy := s.budgets[id].Snapshot()
		global := rateSnapshot(id, "global", s.rateEnabled, policy, policy.Policy.Global)
		user := rateSnapshot(id, "user", s.rateEnabled, policy, policy.Policy.User)
		if id == BudgetSubmit && s.submitFallback.Enabled {
			addFallbackSnapshot(&global, degradedLocalPolicy(policy.Policy.Global, s.submitFallback.GlobalQPS, s.submitFallback.GlobalBurst))
			addFallbackSnapshot(&user, degradedLocalPolicy(policy.Policy.User, s.submitFallback.UserQPS, s.submitFallback.UserBurst))
		}
		snapshot.RateLimits = append(snapshot.RateLimits,
			global,
			user,
		)
	}
	s.queueMu.RLock()
	for _, queue := range s.queues {
		snapshot.Queues = append(snapshot.Queues, queue.snapshot(now))
	}
	s.queueMu.RUnlock()
	if s.locks != nil {
		snapshot.Locks = s.locks.Snapshots()
	}
	if gate := s.gates[GateGRPCDownstream]; gate != nil {
		snapshot.Backpressure = append(snapshot.Backpressure, resilience.BackpressureSnapshot{
			Component: "collection-server", Name: GateGRPCDownstream, Dependency: "apiserver_grpc",
			Strategy: "semaphore_wait", Enabled: true, MaxInflight: gate.Capacity(), InFlight: gate.InFlight(),
			TimeoutMillis: s.grpcInflightWait.Milliseconds(),
		})
	}
	configured := len(snapshot.Locks) == 1 && snapshot.Locks[0].Configured && !snapshot.Locks[0].Degraded && s.opsAvailable
	snapshot.Idempotency = []resilience.CapabilitySnapshot{{
		Name: "answersheet_submit", Kind: resilience.ProtectionIdempotency.String(), Strategy: "redis_lock",
		Configured: configured, Degraded: !configured, Reason: reasonIf(!configured, "submit guard redis runtime unavailable"),
	}}
	return resilience.FinalizeRuntimeSnapshot(snapshot)
}

func addFallbackSnapshot(snapshot *resilience.CapabilitySnapshot, policy ratelimit.RateLimitPolicy) {
	if snapshot == nil {
		return
	}
	snapshot.FallbackStrategy = policy.Strategy
	snapshot.FallbackRate = policy.RatePerSecond
	snapshot.FallbackBurst = policy.Burst
}

func rateSnapshot(id ratelimit.BudgetID, dimension string, configured bool, snapshot ratelimit.BudgetSnapshot, policy ratelimit.RateLimitPolicy) resilience.CapabilitySnapshot {
	return resilience.CapabilitySnapshot{Name: string(id) + "_" + dimension, Kind: resilience.ProtectionRateLimit.String(), Strategy: policy.Strategy,
		Configured: configured, RatePerSecond: policy.RatePerSecond, Burst: policy.Burst, PolicyVersion: snapshot.Version,
		PolicySource: snapshot.Source, OverrideExpiresAt: snapshot.ExpiresAt}
}

func reasonIf(condition bool, reason string) string {
	if condition {
		return reason
	}
	return ""
}

var _ ratelimit.RateBudgetProvider = (*Subsystem)(nil)
