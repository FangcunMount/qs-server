package statisticsv2

import (
	"context"
	"errors"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type collectorStub struct {
	name   string
	called int
}

func (s *collectorStub) Name() string { return s.name }
func (s *collectorStub) Collect(context.Context, domainv2.CollectRequest) (domainv2.CollectResult, error) {
	s.called++
	return domainv2.CollectResult{Collector: s.name, SourceCount: 1, InsertedCount: 1, FactTypeCounts: map[string]int64{"created": 1}}, nil
}

type projectionStub struct{ called int }

func (*projectionStub) Name() string { return "daily" }
func (s *projectionStub) Project(context.Context, domainv2.ProjectionRequest) (domainv2.ProjectionResult, error) {
	s.called++
	return domainv2.ProjectionResult{Name: s.Name(), Rows: 1}, nil
}

type runStoreStub struct {
	run        Run
	failed     int
	committed  int
	succeeded  int
	nextStatus domainv2.RunStatus
}

func (s *runStoreStub) Create(_ context.Context, run Run) (*Run, error) {
	run.ID = 1
	run.Attempt = 1
	s.run = run
	return &s.run, nil
}
func (s *runStoreStub) UpdateProgress(context.Context, uint64, string, map[string]int64, map[string]int64, map[string]int64) error {
	return nil
}
func (s *runStoreStub) MarkDataCommitted(_ context.Context, _ uint64, at time.Time) error {
	s.committed++
	s.run.Status = domainv2.RunStatusDataCommitted
	s.run.DataCommittedAt = &at
	return nil
}
func (s *runStoreStub) MarkSucceeded(_ context.Context, _ uint64, at time.Time) error {
	s.succeeded++
	s.run.Status = domainv2.RunStatusSucceeded
	s.run.FinishedAt = &at
	return nil
}
func (s *runStoreStub) MarkFailed(_ context.Context, _ uint64, _, _, _ string, at time.Time) error {
	s.failed++
	s.run.Status = domainv2.RunStatusFailed
	s.run.FinishedAt = &at
	return nil
}
func (s *runStoreStub) Get(context.Context, uint64) (*Run, error) { return &s.run, nil }
func (s *runStoreStub) List(context.Context, int64, int) ([]Run, error) {
	return []Run{s.run}, nil
}

type lockRunnerStub struct{}

func (lockRunnerStub) Run(ctx context.Context, _ locklease.WorkloadID, _ string, _ time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	return locklease.RunResult{Acquired: true}, body(ctx)
}

type cachePublisherStub struct{ err error }

func (s cachePublisherStub) Publish(context.Context, int64, time.Time) error { return s.err }

func newCoordinatorForTest(t *testing.T, cache CachePublisher) (*Coordinator, *collectorStub, *projectionStub, *runStoreStub, *int) {
	t.Helper()
	collector := &collectorStub{name: "assessment"}
	collectors, err := domainv2.NewCollectorSet(collector)
	if err != nil {
		t.Fatal(err)
	}
	projection := &projectionStub{}
	engine, err := domainv2.NewProjectionEngine(projection)
	if err != nil {
		t.Fatal(err)
	}
	store := &runStoreStub{}
	txCalls := 0
	tx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		txCalls++
		return fn(ctx)
	})
	coordinator := NewCoordinator(collectors, engine, store, tx, lockRunnerStub{}, cache)
	coordinator.now = func() time.Time { return time.Date(2026, 7, 22, 1, 0, 0, 0, domainv2.Shanghai) }
	return coordinator, collector, projection, store, &txCalls
}

func TestCoordinatorCacheFailurePreservesDataCommitted(t *testing.T) {
	coordinator, _, _, store, _ := newCoordinatorForTest(t, cachePublisherStub{err: errors.New("redis unavailable")})
	run, err := coordinator.Run(context.Background(), RunRequest{OrgID: 7, FromDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), ToDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual"})
	if err == nil {
		t.Fatal("expected cache publication error")
	}
	if run == nil || run.Status != domainv2.RunStatusDataCommitted {
		t.Fatalf("run=%+v", run)
	}
	if store.failed != 0 || store.committed != 1 || store.succeeded != 0 {
		t.Fatalf("failed=%d committed=%d succeeded=%d", store.failed, store.committed, store.succeeded)
	}
}

func TestCoordinatorValidateOnlyDoesNotProjectOrOpenResultTransaction(t *testing.T) {
	coordinator, collector, projection, store, txCalls := newCoordinatorForTest(t, nil)
	run, err := coordinator.Run(context.Background(), RunRequest{OrgID: 7, FromDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), ToDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual", ValidateOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainv2.RunStatusSucceeded || collector.called != 1 || projection.called != 0 || *txCalls != 0 || store.committed != 0 {
		t.Fatalf("run=%+v collector=%d projection=%d tx=%d committed=%d", run, collector.called, projection.called, *txCalls, store.committed)
	}
}

func TestCoordinatorResumeCacheDoesNotRecollectOrReproject(t *testing.T) {
	coordinator, collector, projection, store, _ := newCoordinatorForTest(t, cachePublisherStub{})
	store.run = Run{ID: 9, OrgID: 7, AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), Status: domainv2.RunStatusDataCommitted}
	run, err := coordinator.ResumeCache(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainv2.RunStatusSucceeded || collector.called != 0 || projection.called != 0 || store.succeeded != 1 {
		t.Fatalf("run=%+v collector=%d projection=%d succeeded=%d", run, collector.called, projection.called, store.succeeded)
	}
}
