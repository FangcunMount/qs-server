package subsystem

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	collectionresilience "github.com/FangcunMount/qs-server/internal/collection-server/resilience/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
	controlredis "github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSubsystemOwnsStableSharedBudgetsAndSnapshot(t *testing.T) {
	s := New(Options{RateLimit: options.NewRateLimitOptions()})
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
}

func TestQueueCommandWaitsForTargetInstanceResult(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := controlredis.NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	collectionOpts := collectionoptions.NewOptions()
	collection := collectionresilience.New(collectionresilience.Options{
		InstanceID: "collection-0", RateLimit: collectionOpts.RateLimit,
		Concurrency: collectionOpts.Concurrency, WaitReport: collectionOpts.WaitReport,
		OpsAvailable: true, StateStore: store,
	})
	queue := &fakeQueueController{state: resiliencecontrol.QueueStateActive}
	collection.RegisterQueue("answersheet_submit", queue, queue.snapshot)
	cancel := collection.Start(context.Background())
	t.Cleanup(cancel)
	waitForInstance(t, store, "collection-server")

	governor := New(Options{InstanceID: "api-0", RateLimit: options.NewRateLimitOptions(), StateStore: store})
	drained, err := governor.SetQueueState(context.Background(), resiliencecontrol.ActionActor{OrgID: 9}, resiliencecontrol.QueueChange{
		RequestID: "drain-1", Component: "collection-server", Queue: "answersheet_submit", Target: "all",
		DesiredState: resiliencecontrol.QueueStatePaused, TimeoutSeconds: 2,
	})
	if err != nil || drained.Status != resiliencecontrol.CommandStatusOK || len(drained.Instances) != 1 || queue.current() != resiliencecontrol.QueueStatePaused {
		t.Fatalf("drain result=%+v state=%s err=%v", drained, queue.current(), err)
	}
	resumed, err := governor.SetQueueState(context.Background(), resiliencecontrol.ActionActor{OrgID: 9}, resiliencecontrol.QueueChange{
		RequestID: "resume-1", Component: "collection-server", Queue: "answersheet_submit", Target: "all",
		DesiredState: resiliencecontrol.QueueStateActive, TimeoutSeconds: 2,
	})
	if err != nil || resumed.Status != resiliencecontrol.CommandStatusOK || queue.current() != resiliencecontrol.QueueStateActive {
		t.Fatalf("resume result=%+v state=%s err=%v", resumed, queue.current(), err)
	}
}

type fakeQueueController struct {
	mu    sync.Mutex
	state resiliencecontrol.QueueState
}

func (q *fakeQueueController) Drain(context.Context, resiliencecontrol.DrainOptions) (resiliencecontrol.DrainResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.state = resiliencecontrol.QueueStatePaused
	return resiliencecontrol.DrainResult{State: q.state, Version: 2}, nil
}

func (q *fakeQueueController) Resume(context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.state != resiliencecontrol.QueueStatePaused {
		return resiliencecontrol.ErrInvalidState
	}
	q.state = resiliencecontrol.QueueStateActive
	return nil
}

func (q *fakeQueueController) current() resiliencecontrol.QueueState {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

func (q *fakeQueueController) snapshot(time.Time) resilienceplane.QueueSnapshot {
	return resilienceplane.QueueSnapshot{Name: "answersheet_submit", State: string(q.current())}
}

func waitForInstance(t *testing.T, store resiliencecontrol.CommandStore, component string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		instances, err := store.ListInstances(context.Background(), component)
		if err == nil && len(instances) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("instance heartbeat did not appear")
}

func TestRateOverrideReconcilesAcrossInstancesAndResetRestoresConfig(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := controlredis.NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	cfg := options.NewRateLimitOptions()
	left := New(Options{InstanceID: "api-0", RateLimit: cfg, StateStore: store})
	right := New(Options{InstanceID: "api-1", RateLimit: cfg, StateStore: store})
	cancel := right.Start(context.Background())
	t.Cleanup(cancel)

	change := resiliencecontrol.RateLimitChange{
		Mode: "override", Component: "apiserver", Budget: "query", ExpectedVersion: 1,
		Global: resiliencecontrol.RatePolicy{RatePerSecond: 12, Burst: 18},
		User:   resiliencecontrol.RatePolicy{RatePerSecond: 3, Burst: 5}, TTLSeconds: 60,
	}
	result, err := left.TuneRateLimit(context.Background(), resiliencecontrol.ActionActor{OrgID: 9, UserID: 42}, change)
	if err != nil || result.Version != 2 {
		t.Fatalf("TuneRateLimit() = %+v, %v", result, err)
	}
	waitForBudget(t, right, 2, "governance")

	change.Mode = "reset"
	change.ExpectedVersion = 2
	if _, err := left.TuneRateLimit(context.Background(), resiliencecontrol.ActionActor{OrgID: 9, UserID: 42}, change); err != nil {
		t.Fatalf("reset TuneRateLimit() error = %v", err)
	}
	waitForBudget(t, right, 3, "config")

	change.Mode = "override"
	change.ExpectedVersion = 3
	change.Global = resiliencecontrol.RatePolicy{RatePerSecond: 20, Burst: 30}
	change.User = resiliencecontrol.RatePolicy{RatePerSecond: 4, Burst: 6}
	result, err = left.TuneRateLimit(context.Background(), resiliencecontrol.ActionActor{OrgID: 9, UserID: 42}, change)
	if err != nil || result.Version != 4 {
		t.Fatalf("TuneRateLimit() after reset = %+v, %v", result, err)
	}
	waitForBudget(t, right, 4, "governance")
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
