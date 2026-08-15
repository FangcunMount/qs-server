package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mongoconsistency "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

type mongoConsistencyAuditServiceStub struct {
	mu      sync.Mutex
	calls   int
	opts    mongoconsistency.RunOptions
	block   bool
	started chan struct{}
}

func (s *mongoConsistencyAuditServiceStub) RunAuditBatch(ctx context.Context, opts mongoconsistency.RunOptions) (mongoconsistency.BatchOutcome, error) {
	s.mu.Lock()
	s.calls++
	s.opts = opts
	block := s.block
	started := s.started
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block {
		<-ctx.Done()
		return mongoconsistency.BatchOutcome{}, ctx.Err()
	}
	return mongoconsistency.BatchOutcome{CycleID: "cycle-1", Phase: mongoconsistency.PhaseGenerationRun, Scanned: 200}, nil
}

func (s *mongoConsistencyAuditServiceStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type mongoConsistencyLeaderStub struct {
	run func(context.Context, leaderLockRunOptions, func(context.Context) error) error
}

func (mongoConsistencyLeaderStub) DisplayKey() string { return "lock-key" }
func (s mongoConsistencyLeaderStub) Run(ctx context.Context, opts leaderLockRunOptions, body func(context.Context) error) error {
	return s.run(ctx, opts, body)
}

func TestMongoConsistencyAuditRunOnceExecutesOneBoundedBatch(t *testing.T) {
	t.Parallel()
	service := &mongoConsistencyAuditServiceStub{}
	runner := &MongoConsistencyAuditRunner{
		opts: mongoConsistencyAuditTestOptions(), service: service,
		leader: mongoConsistencyLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			return body(ctx)
		}},
	}
	if _, err := runner.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.callCount() != 1 || service.opts.BatchSize != 200 || service.opts.BatchTimeout != 20*time.Millisecond || service.opts.MaxSamples != 10 {
		t.Fatalf("calls/options = %d / %#v", service.callCount(), service.opts)
	}
}

func TestMongoConsistencyAuditSkipsWhenLeaderNotAcquired(t *testing.T) {
	t.Parallel()
	service := &mongoConsistencyAuditServiceStub{}
	runner := &MongoConsistencyAuditRunner{
		opts: mongoConsistencyAuditTestOptions(), service: service,
		leader: mongoConsistencyLeaderStub{run: func(_ context.Context, opts leaderLockRunOptions, _ func(context.Context) error) error {
			opts.OnNotAcquired("lock-key")
			return nil
		}},
	}
	if _, err := runner.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.callCount() != 0 {
		t.Fatalf("calls = %d, want 0", service.callCount())
	}
}

func TestMongoConsistencyAuditBatchTimeoutCancelsScan(t *testing.T) {
	t.Parallel()
	service := &mongoConsistencyAuditServiceStub{block: true}
	runner := &MongoConsistencyAuditRunner{
		opts: mongoConsistencyAuditTestOptions(), service: service,
		leader: mongoConsistencyLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			return body(ctx)
		}},
	}
	if _, err := runner.runOnce(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runOnce error = %v, want deadline exceeded", err)
	}
	if service.callCount() != 1 {
		t.Fatalf("calls = %d, want 1", service.callCount())
	}
}

func TestMongoConsistencyAuditLeaseLossCancelsScan(t *testing.T) {
	t.Parallel()
	service := &mongoConsistencyAuditServiceStub{block: true}
	runner := &MongoConsistencyAuditRunner{
		opts: mongoConsistencyAuditTestOptions(), service: service,
		leader: mongoConsistencyLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			leaseCtx, cancel := context.WithCancel(ctx)
			cancel()
			return body(leaseCtx)
		}},
	}
	if _, err := runner.runOnce(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("runOnce error = %v, want canceled", err)
	}
	if service.callCount() != 1 {
		t.Fatalf("calls = %d, want 1", service.callCount())
	}
}

func TestMongoConsistencyAuditStartHonorsInitialDelay(t *testing.T) {
	service := &mongoConsistencyAuditServiceStub{started: make(chan struct{}, 1)}
	opts := mongoConsistencyAuditTestOptions()
	opts.InitialDelay = 50 * time.Millisecond
	opts.TickInterval = time.Hour
	runner := &MongoConsistencyAuditRunner{
		opts: opts, service: service,
		leader: mongoConsistencyLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			return body(ctx)
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	select {
	case <-service.started:
		t.Fatal("audit ran before initial delay")
	case <-time.After(15 * time.Millisecond):
	}
	select {
	case <-service.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("audit did not run after initial delay")
	}
}

func mongoConsistencyAuditTestOptions() *apiserveroptions.MongoConsistencyAuditOptions {
	return &apiserveroptions.MongoConsistencyAuditOptions{
		Enable: true, InitialDelay: 0, TickInterval: time.Second, CycleInterval: 24 * time.Hour,
		BatchSize: 200, BatchTimeout: 20 * time.Millisecond, MaxSamples: 10,
		LockKey: "lock", LockTTL: 30 * time.Second,
	}
}
