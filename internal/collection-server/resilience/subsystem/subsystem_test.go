package subsystem

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
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

func TestSyncAppliesPausedDesiredStateBeforeReady(t *testing.T) {
	store := newQueueControlStore(t, control.QueueChange{
		Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused,
	})
	queue := newFakeQueue(control.QueueStateActive)
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !s.ControlSynchronized() || queue.stateValue() != control.QueueStatePaused {
		t.Fatalf("ready=%v state=%s, want ready paused", s.ControlSynchronized(), queue.stateValue())
	}
}

func TestSyncRejectsInvalidDesiredStateAndRemainsNotReady(t *testing.T) {
	store := newQueueControlStore(t, control.QueueChange{
		Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueState("invalid"),
	})
	queue := newFakeQueue(control.QueueStateActive)
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

	if err := s.Sync(context.Background()); !errors.Is(err, control.ErrInvalidState) {
		t.Fatalf("Sync() error = %v, want invalid state", err)
	}
	if s.ControlSynchronized() {
		t.Fatal("control readiness must remain false")
	}
}

func TestSyncFailureKeepsControlNotReady(t *testing.T) {
	t.Run("load failure", func(t *testing.T) {
		store := newQueueControlStore(t, control.QueueChange{})
		store.loadErr = errors.New("ops redis unavailable")
		s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
		queue := newFakeQueue(control.QueueStateActive)
		s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

		if err := s.Sync(context.Background()); err == nil || s.ControlSynchronized() {
			t.Fatalf("Sync() error=%v ready=%v, want failure and not ready", err, s.ControlSynchronized())
		}
	})

	t.Run("drain timeout", func(t *testing.T) {
		store := newQueueControlStore(t, control.QueueChange{
			Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused,
		})
		queue := newFakeQueue(control.QueueStateActive)
		queue.drainErr = context.DeadlineExceeded
		s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
		s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

		if err := s.Sync(context.Background()); !errors.Is(err, context.DeadlineExceeded) || s.ControlSynchronized() {
			t.Fatalf("Sync() error=%v ready=%v, want timeout and not ready", err, s.ControlSynchronized())
		}
		if queue.stateValue() != control.QueueStateActive {
			t.Fatalf("queue state=%s, want active after failed drain", queue.stateValue())
		}
	})
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

func TestReadyRemainsTrueAfterSuccessfulSyncWhenControlStoreLaterFails(t *testing.T) {
	store := newQueueControlStore(t, control.QueueChange{
		Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStateActive,
	})
	queue := newFakeQueue(control.QueueStateActive)
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.loadErr = errors.New("temporary redis failure")
	store.mu.Unlock()
	s.reconcile(context.Background())
	if !s.ControlSynchronized() {
		t.Fatal("transient post-sync failure revoked readiness")
	}
}

func TestReconcileAppliesDesiredStateWhenOldClaimCannotBeAcquired(t *testing.T) {
	change := control.QueueChange{Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused}
	store := newQueueControlStore(t, change)
	store.commands = []control.Command{{
		RequestID: "request-1", ActionID: "resilience.drain_queue", Target: control.Target{Component: "collection-server", InstanceID: "all"},
		Actor: control.ActionActor{OrgID: 9}, Payload: mustJSON(t, change), ExpiresAt: time.Now().Add(time.Minute),
	}}
	store.claimed = false
	queue := newFakeQueue(control.QueueStateActive)
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

	s.reconcile(context.Background())
	if queue.stateValue() != control.QueueStatePaused || queue.drainCalls != 1 {
		t.Fatalf("state=%s drainCalls=%d, want paused after stale claim", queue.stateValue(), queue.drainCalls)
	}
}

func TestReconcileDoesNotRaceLocallyExecutingResumeCommand(t *testing.T) {
	change := control.QueueChange{Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStateActive}
	store := newQueueControlStore(t, change)
	store.claimed = true
	store.commands = []control.Command{{
		RequestID: "request-2", ActionID: "resilience.resume_queue", Target: control.Target{Component: "collection-server", InstanceID: "all"},
		Actor: control.ActionActor{OrgID: 9}, Payload: mustJSON(t, change), ExpiresAt: time.Now().Add(time.Minute),
	}}
	queue := newFakeQueue(control.QueueStatePaused)
	queue.resumeStarted = make(chan struct{})
	queue.resumeRelease = make(chan struct{})
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

	s.reconcile(context.Background())
	select {
	case <-queue.resumeStarted:
	case <-time.After(time.Second):
		t.Fatal("resume command did not start")
	}
	s.reconcile(context.Background())
	if queue.resumeCalls != 1 {
		t.Fatalf("resumeCalls=%d, desired-state reconcile raced the active command", queue.resumeCalls)
	}
	close(queue.resumeRelease)
}

func TestCommandWaitsUntilDesiredStateCommitIsVisible(t *testing.T) {
	committed := control.QueueChange{RequestID: "older", StateVersion: 1, Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused}
	commandChange := control.QueueChange{RequestID: "request-commit", StateVersion: 2, Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStateActive}
	store := newQueueControlStore(t, committed)
	store.claimed = true
	store.commands = []control.Command{{
		RequestID: commandChange.RequestID, ActionID: "resilience.resume_queue", Target: control.Target{Component: "collection-server", InstanceID: "all"},
		Actor: control.ActionActor{OrgID: 9}, Payload: mustJSON(t, commandChange), ExpiresAt: time.Now().Add(time.Second),
	}}
	queue := newFakeQueue(control.QueueStatePaused)
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

	s.reconcile(context.Background())
	time.Sleep(75 * time.Millisecond)
	if queue.resumeCalls != 0 {
		t.Fatalf("resumeCalls=%d before desired-state commit", queue.resumeCalls)
	}
	store.setState(control.VersionedState{Version: 2, Payload: mustJSON(t, commandChange)})
	waitFor(t, time.Second, func() bool { return queue.stateValue() == control.QueueStateActive })
	if queue.resumeCalls != 1 {
		t.Fatalf("resumeCalls=%d after desired-state commit, want 1", queue.resumeCalls)
	}
}

func TestCommandResultWriteRetries(t *testing.T) {
	change := control.QueueChange{Component: "collection-server", Queue: "answersheet_submit", Target: "all", DesiredState: control.QueueStatePaused}
	store := newQueueControlStore(t, change)
	store.claimed = true
	store.putFailures = 2
	store.commands = []control.Command{{
		RequestID: "request-result", ActionID: "resilience.drain_queue", Target: control.Target{Component: "collection-server", InstanceID: "all"},
		Actor: control.ActionActor{OrgID: 9}, Payload: mustJSON(t, change), ExpiresAt: time.Now().Add(time.Second),
	}}
	queue := newFakeQueue(control.QueueStateActive)
	s := mustNewSubsystem(t, Options{InstanceID: "collection-0", OpsAvailable: true, StateStore: store})
	s.RegisterQueue("answersheet_submit", queue, queue.snapshot)

	s.reconcile(context.Background())
	waitFor(t, time.Second, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.results) == 1
	})
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.putCalls != 3 {
		t.Fatalf("PutCommandResult calls=%d, want 3", store.putCalls)
	}
}

type queueControlStore struct {
	mu          sync.Mutex
	state       control.VersionedState
	loadErr     error
	commands    []control.Command
	claimed     bool
	results     []control.CommandResult
	putCalls    int
	putFailures int
}

func newQueueControlStore(t *testing.T, change control.QueueChange) *queueControlStore {
	t.Helper()
	return &queueControlStore{state: control.VersionedState{Version: 1, Payload: mustJSON(t, change)}}
}

func (s *queueControlStore) Load(_ context.Context, name string) (control.VersionedState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return control.VersionedState{}, false, s.loadErr
	}
	if name == "queue:collection-server:answersheet_submit" {
		return s.state, true, nil
	}
	return control.VersionedState{}, false, nil
}

func (s *queueControlStore) setState(state control.VersionedState) {
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
}
func (*queueControlStore) CompareAndSwap(context.Context, string, uint64, control.VersionedState, time.Duration) (control.VersionedState, error) {
	return control.VersionedState{}, nil
}
func (*queueControlStore) Delete(context.Context, string, uint64) error { return nil }
func (s *queueControlStore) ListCommands(context.Context, string, string) ([]control.Command, error) {
	return append([]control.Command(nil), s.commands...), nil
}
func (s *queueControlStore) Claim(context.Context, string, string, time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.claimed {
		return false, nil
	}
	s.claimed = false
	return true, nil
}
func (s *queueControlStore) PutCommandResult(_ context.Context, result control.CommandResult, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCalls++
	if s.putFailures > 0 {
		s.putFailures--
		return errors.New("temporary result write failure")
	}
	s.results = append(s.results, result)
	return nil
}
func (s *queueControlStore) ListCommandResults(context.Context, int64, string) ([]control.CommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]control.CommandResult(nil), s.results...), nil
}
func (*queueControlStore) PublishCommand(context.Context, control.Command, time.Duration) error {
	return nil
}
func (*queueControlStore) ListInstances(context.Context, string) ([]control.InstanceIdentity, error) {
	return nil, nil
}

type fakeQueue struct {
	mu            sync.Mutex
	state         control.QueueState
	version       uint64
	drainCalls    int
	drainErr      error
	resumeCalls   int
	resumeStarted chan struct{}
	resumeRelease chan struct{}
}

func newFakeQueue(state control.QueueState) *fakeQueue { return &fakeQueue{state: state, version: 1} }

func (q *fakeQueue) Drain(context.Context, control.DrainOptions) (control.DrainResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.drainCalls++
	if q.drainErr != nil {
		return control.DrainResult{State: q.state, Version: q.version}, q.drainErr
	}
	q.state = control.QueueStatePaused
	q.version++
	result := control.DrainResult{State: q.state, Version: q.version}
	return result, nil
}
func (q *fakeQueue) Resume(context.Context) error {
	q.mu.Lock()
	q.resumeCalls++
	started, release := q.resumeStarted, q.resumeRelease
	q.mu.Unlock()
	if started != nil {
		select {
		case <-started:
		default:
			close(started)
		}
	}
	if release != nil {
		<-release
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.state != control.QueueStatePaused {
		return control.ErrInvalidState
	}
	q.state = control.QueueStateActive
	q.version++
	return nil
}
func (q *fakeQueue) snapshot(now time.Time) resilience.QueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	return resilience.QueueSnapshot{Name: "answersheet_submit", State: string(q.state), StateVersion: q.version, GeneratedAt: now}
}
func (q *fakeQueue) stateValue() control.QueueState {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func mustNewSubsystem(t *testing.T, opts Options) *Subsystem {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
