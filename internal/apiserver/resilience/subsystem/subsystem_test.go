package subsystem

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	collectionresilience "github.com/FangcunMount/qs-server/internal/collection-server/resilience/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
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

func TestQueueCommandWaitsForTargetInstanceResult(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := controlredis.NewStore(client, keyspace.NewBuilderWithNamespace("ops:runtime"))
	collectionOpts := collectionoptions.NewOptions()
	collection, err := collectionresilience.New(collectionresilience.Options{
		InstanceID: "collection-0", RateLimit: collectionOpts.RateLimit,
		Concurrency: collectionOpts.Concurrency, WaitReport: collectionOpts.WaitReport,
		OpsAvailable: true, StateStore: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	queue := &fakeQueueController{state: control.QueueStateActive}
	collection.RegisterQueue("answersheet_submit", queue, queue.snapshot)
	cancel := collection.Start(context.Background())
	t.Cleanup(cancel)
	waitForInstance(t, store, "collection-server")

	governor := mustNewSubsystem(t, Options{InstanceID: "api-0", RateLimit: options.NewRateLimitOptions(), StateStore: store})
	drained, err := governor.SetQueueState(context.Background(), control.ActionActor{OrgID: 9}, control.QueueChange{
		RequestID: "drain-1", Component: "collection-server", Queue: "answersheet_submit", Target: "all",
		DesiredState: control.QueueStatePaused, TimeoutSeconds: 2,
	})
	if err != nil || drained.Status != control.CommandStatusOK || len(drained.Instances) != 1 || queue.current() != control.QueueStatePaused {
		t.Fatalf("drain result=%+v state=%s err=%v", drained, queue.current(), err)
	}
	resumed, err := governor.SetQueueState(context.Background(), control.ActionActor{OrgID: 9}, control.QueueChange{
		RequestID: "resume-1", Component: "collection-server", Queue: "answersheet_submit", Target: "all",
		DesiredState: control.QueueStateActive, TimeoutSeconds: 2,
	})
	if err != nil || resumed.Status != control.CommandStatusOK || queue.current() != control.QueueStateActive {
		t.Fatalf("resume result=%+v state=%s err=%v", resumed, queue.current(), err)
	}
}

type fakeQueueController struct {
	mu    sync.Mutex
	state control.QueueState
}

func (q *fakeQueueController) Drain(context.Context, control.DrainOptions) (control.DrainResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.state = control.QueueStatePaused
	return control.DrainResult{State: q.state, Version: 2}, nil
}

func (q *fakeQueueController) Resume(context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.state != control.QueueStatePaused {
		return control.ErrInvalidState
	}
	q.state = control.QueueStateActive
	return nil
}

func (q *fakeQueueController) current() control.QueueState {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

func (q *fakeQueueController) snapshot(time.Time) resilience.QueueSnapshot {
	return resilience.QueueSnapshot{Name: "answersheet_submit", State: string(q.current())}
}

func waitForInstance(t *testing.T, store control.CommandStore, component string) {
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

func TestQueuePublisherWritesCommandBeforeCommittingDesiredState(t *testing.T) {
	current := control.QueueChange{RequestID: "old", StateVersion: 1, Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused}
	payload, _ := json.Marshal(current)
	store := &queuePublisherStore{
		state: control.VersionedState{Version: 1, Payload: payload}, exists: true,
		instances: []control.InstanceIdentity{{Component: "collection-server", InstanceID: "collection-0", Generation: "g1"}},
		casErr:    errors.New("state CAS failed"),
	}
	governor := mustNewSubsystem(t, Options{InstanceID: "api-0", StateStore: store})
	_, err := governor.SetQueueState(context.Background(), control.ActionActor{OrgID: 9}, control.QueueChange{
		RequestID: "resume-commit", Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStateActive,
	})
	if !errors.Is(err, store.casErr) {
		t.Fatalf("SetQueueState() error=%v, want CAS error", err)
	}
	if len(store.calls) != 2 || store.calls[0] != "publish" || store.calls[1] != "cas" {
		t.Fatalf("calls=%v, want publish before cas", store.calls)
	}
	var commandChange control.QueueChange
	if err := json.Unmarshal(store.command.Payload, &commandChange); err != nil {
		t.Fatal(err)
	}
	if commandChange.RequestID != "resume-commit" || commandChange.StateVersion != 2 {
		t.Fatalf("command change=%+v, want request/version handshake", commandChange)
	}
	if store.state.Version != 1 {
		t.Fatalf("state version=%d after failed CAS, want unchanged", store.state.Version)
	}
}

func TestQueuePublisherDoesNotCommitStateWhenCommandPublishFails(t *testing.T) {
	payload, _ := json.Marshal(control.QueueChange{Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused})
	store := &queuePublisherStore{
		state: control.VersionedState{Version: 1, Payload: payload}, exists: true,
		instances:  []control.InstanceIdentity{{Component: "collection-server", InstanceID: "collection-0", Generation: "g1"}},
		publishErr: errors.New("command publish failed"),
	}
	governor := mustNewSubsystem(t, Options{InstanceID: "api-0", StateStore: store})
	_, err := governor.SetQueueState(context.Background(), control.ActionActor{OrgID: 9}, control.QueueChange{
		RequestID: "resume-publish", Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStateActive,
	})
	if !errors.Is(err, store.publishErr) || len(store.calls) != 1 || store.calls[0] != "publish" || store.state.Version != 1 {
		t.Fatalf("error=%v calls=%v state=%+v", err, store.calls, store.state)
	}
}

func TestQueuePublisherPersistsDesiredStateWithoutLiveInstances(t *testing.T) {
	payload, _ := json.Marshal(control.QueueChange{Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused})
	store := &queuePublisherStore{state: control.VersionedState{Version: 1, Payload: payload}, exists: true}
	governor := mustNewSubsystem(t, Options{InstanceID: "api-0", StateStore: store})
	result, err := governor.SetQueueState(context.Background(), control.ActionActor{OrgID: 9}, control.QueueChange{
		RequestID: "resume-future", Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStateActive,
	})
	if err != nil || result.Status != control.CommandStatusNoop || result.Version != 2 || len(store.calls) != 1 || store.calls[0] != "cas" {
		t.Fatalf("result=%+v error=%v calls=%v", result, err, store.calls)
	}
	var committed control.QueueChange
	if err := json.Unmarshal(store.state.Payload, &committed); err != nil || committed.DesiredState != control.QueueStateActive || committed.StateVersion != 2 {
		t.Fatalf("committed=%+v error=%v", committed, err)
	}
}

type queuePublisherStore struct {
	state      control.VersionedState
	exists     bool
	instances  []control.InstanceIdentity
	command    control.Command
	calls      []string
	publishErr error
	casErr     error
}

func (s *queuePublisherStore) Load(context.Context, string) (control.VersionedState, bool, error) {
	return s.state, s.exists, nil
}
func (s *queuePublisherStore) CompareAndSwap(_ context.Context, _ string, expected uint64, candidate control.VersionedState, _ time.Duration) (control.VersionedState, error) {
	s.calls = append(s.calls, "cas")
	if s.casErr != nil {
		return control.VersionedState{}, s.casErr
	}
	candidate.Version = expected + 1
	s.state, s.exists = candidate, true
	return candidate, nil
}
func (*queuePublisherStore) Delete(context.Context, string, uint64) error { return nil }
func (s *queuePublisherStore) PublishCommand(_ context.Context, command control.Command, _ time.Duration) error {
	s.calls = append(s.calls, "publish")
	s.command = command
	return s.publishErr
}
func (*queuePublisherStore) ListCommands(context.Context, string, string) ([]control.Command, error) {
	return nil, nil
}
func (*queuePublisherStore) Claim(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}
func (*queuePublisherStore) PutCommandResult(context.Context, control.CommandResult, time.Duration) error {
	return nil
}
func (*queuePublisherStore) ListCommandResults(context.Context, int64, string) ([]control.CommandResult, error) {
	return nil, nil
}
func (s *queuePublisherStore) ListInstances(context.Context, string) ([]control.InstanceIdentity, error) {
	return s.instances, nil
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
