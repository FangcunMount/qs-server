package statisticsv2

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type collectorStub struct {
	name   string
	called int
	result domainv2.CollectResult
	err    error
}

func (s *collectorStub) Name() string { return s.name }
func (s *collectorStub) Collect(context.Context, domainv2.CollectRequest) (domainv2.CollectResult, error) {
	s.called++
	if s.result.Collector != "" || s.err != nil {
		return s.result, s.err
	}
	return domainv2.CollectResult{Collector: s.name, SourceCount: 1, InsertedCount: 1, FactTypeCounts: map[string]int64{"created": 1}}, nil
}

type projectionStub struct {
	name   string
	called int
}

func (s *projectionStub) Name() string { return s.name }
func (s *projectionStub) Project(context.Context, domainv2.ProjectionRequest) (domainv2.ProjectionResult, error) {
	s.called++
	return domainv2.ProjectionResult{Name: s.Name(), Rows: 1}, nil
}

type runStoreStub struct {
	run            Run
	failed         int
	committed      int
	succeeded      int
	publishableErr error
	failedStage    string
	failedCode     string
}

func (s *runStoreStub) Create(_ context.Context, run Run) (*Run, error) {
	run.ID = 1
	run.Attempt = 1
	s.run = run
	return &s.run, nil
}

func (s *runStoreStub) UpdateProgress(_ context.Context, _ uint64, stage string, sources, facts, results map[string]int64) error {
	s.run.Stage = stage
	if sources != nil {
		s.run.SourceCounts = cloneCounts(sources)
	}
	if facts != nil {
		s.run.FactCounts = cloneCounts(facts)
	}
	if results != nil {
		s.run.ResultCounts = cloneCounts(results)
	}
	return nil
}

func cloneCounts(input map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
func (s *runStoreStub) AssertPublishable(context.Context, int64, time.Time) error {
	return s.publishableErr
}
func (s *runStoreStub) MarkDataCommitted(_ context.Context, _ uint64, at time.Time) error {
	s.committed++
	s.run.Status = domainv2.RunStatusDataCommitted
	s.run.DataCommittedAt = &at
	return nil
}
func (s *runStoreStub) MarkCachePublished(_ context.Context, _ uint64, generation int64, at time.Time) error {
	s.run.CacheGeneration = generation
	s.run.CachePublishedAt = &at
	return nil
}
func (s *runStoreStub) MarkCachePublishFailed(_ context.Context, _ uint64, generation int64, _ string, at time.Time) error {
	if generation > 0 {
		s.run.CacheGeneration = generation
		s.run.CachePublishedAt = &at
	}
	return nil
}
func (s *runStoreStub) RecordCacheResume(context.Context, uint64, uint64, string, string, int64, time.Time) error {
	return nil
}
func (s *runStoreStub) MarkSucceeded(_ context.Context, _ uint64, at time.Time) error {
	s.succeeded++
	s.run.Status = domainv2.RunStatusSucceeded
	s.run.FinishedAt = &at
	return nil
}
func (s *runStoreStub) MarkFailed(_ context.Context, _ uint64, stage, code, _ string, at time.Time) error {
	s.failed++
	s.failedStage = stage
	s.failedCode = code
	s.run.Status = domainv2.RunStatusFailed
	s.run.Stage = stage
	s.run.ErrorCode = code
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

type cachePublisherStub struct {
	generation int64
	err        error
}

func (s cachePublisherStub) Publish(context.Context, int64, time.Time) (int64, error) {
	return s.generation, s.err
}

type countingCachePublisher struct{ called int }

func (s *countingCachePublisher) Publish(context.Context, int64, time.Time) (int64, error) {
	s.called++
	return int64(s.called), nil
}

func newCoordinatorForTest(t *testing.T, cache CachePublisher) (*Coordinator, *collectorStub, *projectionStub, *projectionStub, *runStoreStub, *int) {
	t.Helper()
	collector := &collectorStub{name: "assessment"}
	collectors, err := domainv2.NewCollectorSet(collector)
	if err != nil {
		t.Fatal(err)
	}
	daily := &projectionStub{name: "daily"}
	dailyEngine, err := domainv2.NewProjectionEngine(daily)
	if err != nil {
		t.Fatal(err)
	}
	global := &projectionStub{name: "global"}
	globalEngine, err := domainv2.NewProjectionEngine(global)
	if err != nil {
		t.Fatal(err)
	}
	store := &runStoreStub{}
	txCalls := 0
	tx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		txCalls++
		return fn(ctx)
	})
	coordinator := NewCoordinator(collectors, dailyEngine, globalEngine, store, tx, lockRunnerStub{}, cache)
	coordinator.now = func() time.Time { return time.Date(2026, 7, 22, 1, 0, 0, 0, domainv2.Shanghai) }
	return coordinator, collector, daily, global, store, &txCalls
}

func TestCoordinatorCacheFailurePreservesDataCommitted(t *testing.T) {
	coordinator, _, _, _, store, _ := newCoordinatorForTest(t, cachePublisherStub{generation: 8, err: errors.New("warmup unavailable")})
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
	if run.CacheGeneration != 8 || run.CachePublishedAt == nil {
		t.Fatalf("switched generation was not recorded: %+v", run)
	}
}

func TestCoordinatorPersistsPublishedCacheGeneration(t *testing.T) {
	coordinator, _, _, _, _, _ := newCoordinatorForTest(t, cachePublisherStub{generation: 9})
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, FromDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai),
		ToDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "scheduled", Mode: domainv2.RunModePublish,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainv2.RunStatusSucceeded || run.CacheGeneration != 9 || run.CachePublishedAt == nil {
		t.Fatalf("run=%+v", run)
	}
}

func TestCoordinatorValidateOnlyDoesNotProjectOrOpenResultTransaction(t *testing.T) {
	coordinator, collector, projection, global, store, txCalls := newCoordinatorForTest(t, nil)
	run, err := coordinator.Run(context.Background(), RunRequest{OrgID: 7, FromDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), ToDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual", ValidateOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainv2.RunStatusSucceeded || collector.called != 1 || projection.called != 0 || global.called != 0 || *txCalls != 0 || store.committed != 0 {
		t.Fatalf("run=%+v collector=%d projection=%d global=%d tx=%d committed=%d", run, collector.called, projection.called, global.called, *txCalls, store.committed)
	}
}

func TestCoordinatorRepairRebuildsDailyWithoutPublishingGlobalResults(t *testing.T) {
	cache := &countingCachePublisher{}
	coordinator, collector, daily, global, store, txCalls := newCoordinatorForTest(t, cache)
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, FromDate: time.Date(2025, 1, 1, 0, 0, 0, 0, domainv2.Shanghai),
		ToDate: time.Date(2025, 1, 7, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual", Mode: domainv2.RunModeRepair,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainv2.RunStatusSucceeded || run.AsOfDate.Format("2006-01-02") != "2026-07-21" {
		t.Fatalf("run=%+v", run)
	}
	if collector.called != 1 || daily.called != 1 || global.called != 0 || cache.called != 0 || *txCalls != 1 || store.committed != 0 {
		t.Fatalf("collector=%d daily=%d global=%d cache=%d tx=%d committed=%d", collector.called, daily.called, global.called, cache.called, *txCalls, store.committed)
	}
}

func TestCoordinatorRejectsCurrentOrFutureBusinessDay(t *testing.T) {
	coordinator, collector, daily, global, _, txCalls := newCoordinatorForTest(t, nil)
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, FromDate: time.Date(2026, 7, 22, 0, 0, 0, 0, domainv2.Shanghai),
		ToDate: time.Date(2026, 7, 22, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual", Mode: domainv2.RunModeRepair,
	})
	if err == nil || run != nil {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if collector.called != 0 || daily.called != 0 || global.called != 0 || *txCalls != 0 {
		t.Fatalf("unexpected work collector=%d daily=%d global=%d tx=%d", collector.called, daily.called, global.called, *txCalls)
	}
}

func TestCoordinatorRejectsMissingWindowBeforeCreatingRun(t *testing.T) {
	coordinator, collector, daily, global, store, txCalls := newCoordinatorForTest(t, nil)
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, TriggerType: "manual", Mode: domainv2.RunModeRepair,
	})
	if err == nil || !IsInvalidRunRequest(err) || run != nil {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if store.run.ID != 0 || collector.called != 0 || daily.called != 0 || global.called != 0 || *txCalls != 0 {
		t.Fatalf("run_id=%d collector=%d daily=%d global=%d tx=%d", store.run.ID, collector.called, daily.called, global.called, *txCalls)
	}
}

func TestCoordinatorRejectsOversizedAuditReasonBeforeCreatingRun(t *testing.T) {
	coordinator, collector, daily, global, store, txCalls := newCoordinatorForTest(t, nil)
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, FromDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai),
		ToDate:      time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai),
		TriggerType: "manual", Mode: domainv2.RunModeRepair, Reason: strings.Repeat("审", 501),
	})
	if err == nil || !IsInvalidRunRequest(err) || run != nil {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if store.run.ID != 0 || collector.called != 0 || daily.called != 0 || global.called != 0 || *txCalls != 0 {
		t.Fatalf("run_id=%d collector=%d daily=%d global=%d tx=%d", store.run.ID, collector.called, daily.called, global.called, *txCalls)
	}
}

func TestCoordinatorRejectsPublishedWatermarkRegression(t *testing.T) {
	coordinator, _, daily, global, store, _ := newCoordinatorForTest(t, nil)
	store.publishableErr = errors.New("current watermark is newer")
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, FromDate: time.Date(2026, 7, 15, 0, 0, 0, 0, domainv2.Shanghai),
		ToDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual", Mode: domainv2.RunModePublish,
	})
	if err == nil || run == nil || run.Status != domainv2.RunStatusFailed {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if daily.called != 1 || global.called != 0 {
		t.Fatalf("daily=%d global=%d", daily.called, global.called)
	}
	if store.failedStage != "projecting_org_snapshot" || store.failedCode != "publish_watermark_regression" {
		t.Fatalf("stage=%s code=%s", store.failedStage, store.failedCode)
	}
}

func TestCoordinatorPersistsPartialCollectorCountsBeforeFailure(t *testing.T) {
	coordinator, collector, daily, global, store, _ := newCoordinatorForTest(t, nil)
	collector.result = domainv2.CollectResult{Collector: "assessment", SourceCount: 4, InsertedCount: 2, ExistingCount: 1, ConflictCount: 1, FactTypeCounts: map[string]int64{"assessment_created": 4}}
	collector.err = errors.New("fact conflict")
	run, err := coordinator.Run(context.Background(), RunRequest{
		OrgID: 7, FromDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai),
		ToDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), TriggerType: "manual", Mode: domainv2.RunModeRepair,
	})
	if err == nil || run == nil || run.Status != domainv2.RunStatusFailed {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if daily.called != 0 || global.called != 0 || store.failedStage != "collecting_assessment" || store.failedCode != "fact_conflict" {
		t.Fatalf("daily=%d global=%d stage=%s code=%s", daily.called, global.called, store.failedStage, store.failedCode)
	}
	if run.SourceCounts["assessment"] != 4 || run.FactCounts["assessment.conflict"] != 1 || run.FactCounts["assessment.type.assessment_created"] != 4 {
		t.Fatalf("source_counts=%v fact_counts=%v", run.SourceCounts, run.FactCounts)
	}
}

func TestCoordinatorResumeCacheDoesNotRecollectOrReproject(t *testing.T) {
	coordinator, collector, projection, global, store, _ := newCoordinatorForTest(t, cachePublisherStub{generation: 4})
	store.run = Run{ID: 9, OrgID: 7, Mode: domainv2.RunModePublish, AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, domainv2.Shanghai), Status: domainv2.RunStatusDataCommitted}
	run, err := coordinator.ResumeCache(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainv2.RunStatusSucceeded || collector.called != 0 || projection.called != 0 || global.called != 0 || store.succeeded != 1 {
		t.Fatalf("run=%+v collector=%d projection=%d succeeded=%d", run, collector.called, projection.called, store.succeeded)
	}
	if run.CacheGeneration != 4 || run.CachePublishedAt == nil {
		t.Fatalf("cache publication was not persisted: %+v", run)
	}
}
