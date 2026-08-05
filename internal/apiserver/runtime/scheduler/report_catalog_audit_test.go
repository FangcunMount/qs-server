package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	interpretationcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

type reportCatalogAuditServiceStub struct {
	mu      sync.Mutex
	calls   int
	opts    interpretationcatalog.AuditRunOptions
	block   bool
	started chan struct{}
	outcome interpretationcatalog.AuditBatchOutcome
	err     error
}

func (s *reportCatalogAuditServiceStub) RunAuditBatch(ctx context.Context, opts interpretationcatalog.AuditRunOptions) (interpretationcatalog.AuditBatchOutcome, error) {
	s.mu.Lock()
	s.calls++
	s.opts = opts
	started := s.started
	block := s.block
	outcome := s.outcome
	err := s.err
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block {
		<-ctx.Done()
		return interpretationcatalog.AuditBatchOutcome{}, ctx.Err()
	}
	if outcome.CycleID == "" {
		outcome = interpretationcatalog.AuditBatchOutcome{CycleID: "cycle-1", Phase: interpretationcatalog.AuditPhaseMissing, Scanned: 200}
	}
	return outcome, err
}

func (s *reportCatalogAuditServiceStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type reportCatalogLeaderStub struct {
	run func(context.Context, leaderLockRunOptions, func(context.Context) error) error
}

func (reportCatalogLeaderStub) DisplayKey() string { return "lock-key" }
func (s reportCatalogLeaderStub) Run(ctx context.Context, opts leaderLockRunOptions, body func(context.Context) error) error {
	return s.run(ctx, opts, body)
}

func TestReportCatalogAuditRunOnceExecutesOneBoundedBatch(t *testing.T) {
	t.Parallel()
	service := &reportCatalogAuditServiceStub{}
	runner := &ReportCatalogAuditRunner{
		opts: reportCatalogAuditTestOptions(), service: service,
		leader: reportCatalogLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			return body(ctx)
		}},
	}
	if _, err := runner.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.callCount() != 1 || service.opts.BatchSize != 200 || service.opts.BatchTimeout != 20*time.Millisecond {
		t.Fatalf("calls/options = %d / %#v", service.callCount(), service.opts)
	}
}

func TestReportCatalogAuditBatchTimeoutCancelsWithoutSecondCall(t *testing.T) {
	t.Parallel()
	service := &reportCatalogAuditServiceStub{block: true}
	runner := &ReportCatalogAuditRunner{
		opts: reportCatalogAuditTestOptions(), service: service,
		leader: reportCatalogLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			return body(ctx)
		}},
	}
	if _, err := runner.runOnce(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runOnce() error = %v", err)
	}
	if service.callCount() != 1 {
		t.Fatalf("calls = %d, want 1", service.callCount())
	}
}

func TestReportCatalogAuditStartHonorsInitialDelay(t *testing.T) {
	service := &reportCatalogAuditServiceStub{started: make(chan struct{}, 1)}
	opts := reportCatalogAuditTestOptions()
	opts.InitialDelay = 50 * time.Millisecond
	opts.TickInterval = time.Hour
	runner := &ReportCatalogAuditRunner{
		opts: opts, service: service,
		leader: reportCatalogLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
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

func TestReportCatalogAuditWaitsUntilNextDailyCycle(t *testing.T) {
	t.Parallel()
	nextCycleAt := time.Now().Add(time.Hour)
	service := &reportCatalogAuditServiceStub{outcome: interpretationcatalog.AuditBatchOutcome{
		CycleID: "cycle-1", Phase: interpretationcatalog.AuditPhaseCatalog,
		Completed: true, NextCycleAt: nextCycleAt,
	}}
	runner := &ReportCatalogAuditRunner{
		opts: reportCatalogAuditTestOptions(), service: service,
		leader: reportCatalogLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			return body(ctx)
		}},
	}
	runner.executeTick(context.Background())
	runner.executeTick(context.Background())
	if service.callCount() != 1 || !runner.nextCycleAt.Equal(nextCycleAt) {
		t.Fatalf("calls/next cycle = %d / %s", service.callCount(), runner.nextCycleAt)
	}
}

func TestReportCatalogAuditLeaseCancellationReachesBatch(t *testing.T) {
	t.Parallel()
	service := &reportCatalogAuditServiceStub{block: true}
	runner := &ReportCatalogAuditRunner{
		opts: reportCatalogAuditTestOptions(), service: service,
		leader: reportCatalogLeaderStub{run: func(ctx context.Context, _ leaderLockRunOptions, body func(context.Context) error) error {
			leaseCtx, cancel := context.WithCancel(ctx)
			cancel()
			return body(leaseCtx)
		}},
	}
	if _, err := runner.runOnce(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("runOnce() error = %v", err)
	}
	if service.callCount() != 1 {
		t.Fatalf("calls = %d, want 1", service.callCount())
	}
}

func reportCatalogAuditTestOptions() *apiserveroptions.ReportCatalogAuditOptions {
	return &apiserveroptions.ReportCatalogAuditOptions{
		Enable: true, InitialDelay: 0, TickInterval: time.Second, CycleInterval: 24 * time.Hour,
		BatchSize: 200, BatchTimeout: 20 * time.Millisecond, LockKey: "lock", LockTTL: 30 * time.Second,
	}
}
