package subsystem

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	controlredis "github.com/FangcunMount/qs-server/internal/pkg/resilience/control/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSubsystemOwnsStableSharedBudgetsAndSnapshot(t *testing.T) {
	s := mustNewSubsystem(t, Options{RateLimit: options.NewRateLimitOptions(), Backpressure: options.NewBackpressureOptions()})
	left, ok := s.Budget(BudgetQuery)
	if !ok {
		t.Fatal("query budget unavailable")
	}
	right, ok := s.Budget(BudgetQuery)
	if !ok || left.Global != right.Global || left.User != right.User {
		t.Fatal("query callers must share stable limiter proxies")
	}
	snapshot := s.Snapshot(time.Now())
	if snapshot.Component != "apiserver" || snapshot.InstanceID == "" || len(snapshot.RateLimits) != 8 || len(snapshot.Backpressure) != 3 {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
	for _, name := range []string{"mysql", "mongo", "iam"} {
		left := s.Backpressure(name)
		right := s.Backpressure(name)
		if left == nil || left != right {
			t.Fatalf("%s backpressure is not a stable shared instance", name)
		}
	}
}

func mustNewSubsystem(t *testing.T, opts Options) *Subsystem {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRateOverrideReconcilesAcrossInstancesAndResetRestoresConfig(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	// Production clients are connected by NamedRedisRegistry before they are
	// injected into the resilience subsystem. Mirror that readiness contract so
	// go-redis does not perform its first-connection option negotiation while
	// SUBSCRIBE and WATCH start concurrently.
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Fatalf("initialize redis client: %v", err)
	}
	store := controlredis.NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	cfg := options.NewRateLimitOptions()
	left := mustNewSubsystem(t, Options{InstanceID: "api-0", RateLimit: cfg, StateStore: store})
	right := mustNewSubsystem(t, Options{InstanceID: "api-1", RateLimit: cfg, StateStore: store})
	cancel := right.Start(context.Background())
	t.Cleanup(cancel)

	change := control.RateLimitChange{
		Mode: "override", Component: "apiserver", Budget: "query", ExpectedVersion: 1,
		Global: control.RatePolicy{RatePerSecond: 12, Burst: 18},
		User:   control.RatePolicy{RatePerSecond: 3, Burst: 5}, TTLSeconds: 60,
	}
	result, err := left.TuneRateLimit(context.Background(), control.ActionActor{OrgID: 9, UserID: 42}, change)
	if err != nil || result.Version != 2 {
		t.Fatalf("TuneRateLimit() = %+v, %v", result, err)
	}
	waitForBudget(t, right, 2, "governance")

	change.Mode = "reset"
	change.ExpectedVersion = 2
	if _, err := left.TuneRateLimit(context.Background(), control.ActionActor{OrgID: 9, UserID: 42}, change); err != nil {
		t.Fatalf("reset TuneRateLimit() error = %v", err)
	}
	waitForBudget(t, right, 3, "config")

	change.Mode = "override"
	change.ExpectedVersion = 3
	change.Global = control.RatePolicy{RatePerSecond: 20, Burst: 30}
	change.User = control.RatePolicy{RatePerSecond: 4, Burst: 6}
	result, err = left.TuneRateLimit(context.Background(), control.ActionActor{OrgID: 9, UserID: 42}, change)
	if err != nil || result.Version != 4 {
		t.Fatalf("TuneRateLimit() after reset = %+v, %v", result, err)
	}
	waitForBudget(t, right, 4, "governance")
}

func TestCommandTargetInstancesDeduplicatesGenerations(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := controlredis.NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	identity, err := control.ResolveInstanceIdentity("collection-server", "collection-0")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Heartbeat(context.Background(), identity, time.Minute); err != nil {
		t.Fatal(err)
	}
	identity.Generation = "new-generation"
	if err := store.Heartbeat(context.Background(), identity, time.Minute); err != nil {
		t.Fatal(err)
	}

	instances, err := commandTargetInstances(context.Background(), store, identity.Component, "all")
	if err != nil || len(instances) != 1 || instances[0] != identity.InstanceID {
		t.Fatalf("commandTargetInstances() = %v, %v", instances, err)
	}
}

func TestSubsystemPollsCommandsOnStartupSignalAndRecoveryOnly(t *testing.T) {
	store := &commandPollStore{signals: make(chan string, 4)}
	s := mustNewSubsystem(t, Options{RateLimit: options.NewRateLimitOptions(), StateStore: store})
	s.stateEvery = time.Hour
	s.commandEvery = 200 * time.Millisecond
	cancel := s.Start(context.Background())
	t.Cleanup(cancel)

	waitForCommandPolls(t, store, 1)
	store.signals <- "rate:apiserver:query"
	time.Sleep(30 * time.Millisecond)
	if got := store.polls.Load(); got != 1 {
		t.Fatalf("non-command signal command polls=%d, want 1", got)
	}
	store.signals <- "command:apiserver"
	waitForCommandPolls(t, store, 2)
	waitForCommandPolls(t, store, 3)
}

type commandPollStore struct {
	signals chan string
	polls   atomic.Int64
}

func (s *commandPollStore) Load(context.Context, string) (control.VersionedState, bool, error) {
	return control.VersionedState{}, false, nil
}

func (s *commandPollStore) CompareAndSwap(_ context.Context, _ string, _ uint64, candidate control.VersionedState, _ time.Duration) (control.VersionedState, error) {
	return candidate, nil
}

func (s *commandPollStore) Delete(context.Context, string, uint64) error { return nil }

func (s *commandPollStore) WatchStateSignals(context.Context) (<-chan string, error) {
	return s.signals, nil
}

func (s *commandPollStore) PublishCommand(context.Context, control.Command, time.Duration) error {
	return nil
}

func (s *commandPollStore) ListCommands(context.Context, string, string) ([]control.Command, error) {
	s.polls.Add(1)
	return []control.Command{}, nil
}

func (s *commandPollStore) Claim(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}

func (s *commandPollStore) PutCommandResult(context.Context, control.CommandResult, time.Duration) error {
	return nil
}

func (s *commandPollStore) ListCommandResults(context.Context, int64, string) ([]control.CommandResult, error) {
	return []control.CommandResult{}, nil
}

func (s *commandPollStore) ListInstances(context.Context, string) ([]control.InstanceIdentity, error) {
	return []control.InstanceIdentity{}, nil
}

func waitForCommandPolls(t *testing.T, store *commandPollStore, want int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.polls.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("command polls=%d, want at least %d", store.polls.Load(), want)
}

func waitForBudget(t *testing.T, subsystem *Subsystem, version uint64, source string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		budget, _ := subsystem.RateBudget(ratelimit.BudgetID("query"))
		snapshot := budget.Snapshot()
		if snapshot.Version == version && snapshot.Source == source {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	budget, _ := subsystem.RateBudget(ratelimit.BudgetID("query"))
	t.Fatalf("budget did not converge: %+v", budget.Snapshot())
}
