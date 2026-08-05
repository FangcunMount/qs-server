package subsystem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
)

func TestSubsystemOwnsBudgetsAndGates(t *testing.T) {
	opts := options.NewOptions()
	opts.GRPCClient.MaxInflight = 7
	opts.GRPCClient.InflightWaitMs = 25
	s := mustNewSubsystem(t, Options{RateLimit: opts.RateLimit, Concurrency: opts.Concurrency, WaitReport: opts.WaitReport, GRPCClient: opts.GRPCClient})
	left, ok := s.Budget(BudgetReportEvents)
	if !ok {
		t.Fatal("report events budget unavailable")
	}
	right, _ := s.Budget(BudgetReportEvents)
	if left.Global != right.Global || left.User != right.User {
		t.Fatal("report events callers must share stable limiter proxies")
	}
	if s.Gate(GateQuery) == nil || s.Gate(GateSubmit) == nil || s.Gate(GateWaitReport) == nil {
		t.Fatal("expected process-owned concurrency gates")
	}
	grpcGate := s.Gate(GateGRPCDownstream)
	if grpcGate == nil || grpcGate.Capacity() != 7 || !grpcGate.TryAcquire() {
		t.Fatalf("grpc gate = %#v", grpcGate)
	}
	t.Cleanup(grpcGate.Release)
	snapshot := s.Snapshot(time.Now())
	if len(snapshot.RateLimits) != 8 || snapshot.InstanceID == "" {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
	if len(snapshot.Backpressure) != 1 || snapshot.Backpressure[0].Name != GateGRPCDownstream || snapshot.Backpressure[0].MaxInflight != 7 || snapshot.Backpressure[0].InFlight != 1 || snapshot.Backpressure[0].TimeoutMillis != 25 {
		t.Fatalf("grpc backpressure snapshot = %+v", snapshot.Backpressure)
	}
}

func TestSubmitBudgetUsesConservativeLocalFallbackOnlyWhenRedisDegrades(t *testing.T) {
	cfg := options.NewRateLimitOptions()
	cfg.SubmitDegradedLocal.GlobalQPS = 1
	cfg.SubmitDegradedLocal.GlobalBurst = 1
	cfg.SubmitDegradedLocal.UserQPS = 1
	cfg.SubmitDegradedLocal.UserBurst = 1
	backend := &rateBackend{allowed: true}
	s := mustNewSubsystem(t, Options{RateLimit: cfg, Backend: backend})
	budget, ok := s.Budget(BudgetSubmit)
	if !ok {
		t.Fatal("submit budget unavailable")
	}

	if first := budget.Global.Decide(t.Context(), "submit"); !first.Allowed || first.Outcome != resilience.OutcomeAllowed {
		t.Fatalf("healthy first decision = %#v", first)
	}
	if second := budget.Global.Decide(t.Context(), "submit"); !second.Allowed || second.Outcome != resilience.OutcomeAllowed {
		t.Fatalf("healthy second decision consumed fallback capacity: %#v", second)
	}

	backend.err = errors.New("redis down")
	if degraded := budget.Global.Decide(t.Context(), "submit"); !degraded.Allowed || degraded.Outcome != resilience.OutcomeDegradedOpen || degraded.Subject.Strategy != "local_fallback" {
		t.Fatalf("degraded first decision = %#v", degraded)
	}
	if limited := budget.Global.Decide(t.Context(), "submit"); limited.Allowed || limited.Outcome != resilience.OutcomeRateLimited || limited.Subject.Strategy != "local_fallback" {
		t.Fatalf("degraded second decision = %#v", limited)
	}

	query, _ := s.Budget(BudgetQuery)
	for i := 0; i < 2; i++ {
		if decision := query.Global.Decide(t.Context(), "query"); !decision.Allowed || decision.Outcome != resilience.OutcomeDegradedOpen {
			t.Fatalf("query decision %d = %#v, want unchanged fail-open", i, decision)
		}
	}
}

func TestSubmitBudgetWithoutBackendUsesFallbackAndCapsDynamicOverride(t *testing.T) {
	cfg := options.NewRateLimitOptions()
	cfg.SubmitDegradedLocal.GlobalQPS = 30
	cfg.SubmitDegradedLocal.GlobalBurst = 45
	cfg.SubmitDegradedLocal.UserQPS = 10
	cfg.SubmitDegradedLocal.UserBurst = 15
	s := mustNewSubsystem(t, Options{RateLimit: cfg})
	budget, _ := s.Budget(BudgetSubmit)
	if decision := budget.Global.Decide(t.Context(), "submit"); !decision.Allowed || decision.Outcome != resilience.OutcomeDegradedOpen {
		t.Fatalf("nil backend decision = %#v", decision)
	}

	runtimeBudget, _ := s.RateBudget(BudgetSubmit)
	_, err := runtimeBudget.Apply(1, ratelimit.BudgetPolicy{
		Global: ratePolicy(BudgetSubmit, "global", "redis", 5, 6),
		User:   ratePolicy(BudgetSubmit, "user", "redis", 2, 3),
	}, "governance", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot(time.Now())
	var global, user *resilience.CapabilitySnapshot
	for i := range snapshot.RateLimits {
		item := &snapshot.RateLimits[i]
		switch item.Name {
		case "submit_global":
			global = item
		case "submit_user":
			user = item
		}
	}
	if global == nil || global.FallbackRate != 5 || global.FallbackBurst != 6 {
		t.Fatalf("submit global fallback snapshot = %+v", global)
	}
	if user == nil || user.FallbackRate != 2 || user.FallbackBurst != 3 {
		t.Fatalf("submit user fallback snapshot = %+v", user)
	}
}

func TestControlSyncIsRequiredByDefaultWhenStoreIsMissing(t *testing.T) {
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0"})
	if err := s.Sync(context.Background()); !errors.Is(err, control.ErrUnavailable) {
		t.Fatalf("Sync() error=%v, want unavailable", err)
	}
	if s.ControlSynchronized() {
		t.Fatal("control readiness=true before required initial sync")
	}
}

func TestControlCanBeExplicitlyDisabled(t *testing.T) {
	disabled := false
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", ControlEnabled: &disabled})
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync() error=%v", err)
	}
	if !s.ControlSynchronized() {
		t.Fatal("control readiness=false when control is explicitly disabled")
	}
	cancel := s.Start(context.Background())
	cancel()
}

func TestControlSyncSucceedsWhenStoreIsAvailable(t *testing.T) {
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: availableStateStore{}})
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync() error=%v", err)
	}
	if !s.ControlSynchronized() {
		t.Fatal("control readiness=false after successful sync")
	}
}

type availableStateStore struct{}

func (availableStateStore) Load(context.Context, string) (control.VersionedState, bool, error) {
	return control.VersionedState{}, false, nil
}
func (availableStateStore) CompareAndSwap(context.Context, string, uint64, control.VersionedState, time.Duration) (control.VersionedState, error) {
	return control.VersionedState{}, nil
}
func (availableStateStore) Delete(context.Context, string, uint64) error { return nil }

type rateBackend struct {
	allowed bool
	err     error
}

func (b *rateBackend) Allow(context.Context, string, float64, int) (bool, time.Duration, error) {
	return b.allowed, 0, b.err
}

func mustNewSubsystem(t *testing.T, opts Options) *Subsystem {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
