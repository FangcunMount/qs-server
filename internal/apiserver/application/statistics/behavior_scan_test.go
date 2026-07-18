package statistics

import (
	"context"
	"errors"
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type behaviorScanStateStub struct {
	watermark *domainStatistics.ScanWatermark
	facts     []domainStatistics.EntryResolveFact
	saved     []domainStatistics.ScanWatermark
}

func (s *behaviorScanStateStub) LoadScanWatermark(context.Context, int64, string) (*domainStatistics.ScanWatermark, error) {
	if s.watermark == nil {
		return nil, nil
	}
	copy := *s.watermark
	return &copy, nil
}

func (s *behaviorScanStateStub) SaveScanWatermark(_ context.Context, watermark *domainStatistics.ScanWatermark) error {
	copy := *watermark
	s.saved = append(s.saved, copy)
	return nil
}

func (s *behaviorScanStateStub) ListEntryResolveFacts(context.Context, int64, uint64, time.Time, int) ([]domainStatistics.EntryResolveFact, error) {
	return append([]domainStatistics.EntryResolveFact(nil), s.facts...), nil
}

func (*behaviorScanStateStub) ListEntryIntakeFacts(context.Context, int64, uint64, time.Time, int) ([]domainStatistics.EntryIntakeFact, error) {
	return nil, nil
}

func (*behaviorScanStateStub) ListAssessmentCreatedFacts(context.Context, int64, uint64, time.Time, int) ([]domainStatistics.AssessmentCreatedFact, error) {
	return nil, nil
}

type behaviorScanRebuilderStub struct {
	calls int
	from  time.Time
	to    time.Time
}

func (s *behaviorScanRebuilderStub) RebuildJourneyDailyWindow(_ context.Context, _ int64, from, to time.Time) error {
	s.calls++
	s.from = from
	s.to = to
	return nil
}

func TestBehaviorJourneyScanAdvancesWatermarkOnlyAfterWholeSourceBatchSucceeds(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.Local)
	state := &behaviorScanStateStub{facts: []domainStatistics.EntryResolveFact{
		{OrgID: 9, EntryID: 10, LogID: 101, OccurredAt: now.Add(-2 * time.Minute)},
		{OrgID: 9, EntryID: 10, LogID: 102, OccurredAt: now.Add(-time.Minute)},
	}}
	repo := &behaviorProjectorRepoStub{}
	runner := &behaviorProjectorRunnerStub{}
	rebuilder := &behaviorScanRebuilderStub{}
	service := NewBehaviorJourneyScanService(runner, repo, state, rebuilder, nil, nil)

	result, err := service.ScanDue(context.Background(), BehaviorJourneyScanInput{
		OrgIDs: []int64{9}, Sources: []string{domainStatistics.ScanSourceEntryResolve}, Now: now, WindowRecalc: true,
	})
	if err != nil {
		t.Fatalf("ScanDue returned error: %v", err)
	}
	got := result.SourceResults[0]
	if got.Scanned != 2 || got.Projected != 2 || got.Failed != 0 || got.WatermarkID != 102 {
		t.Fatalf("unexpected source result: %+v", got)
	}
	if len(state.saved) != 2 || state.saved[0].Status != domainStatistics.ScanWatermarkStatusRunning || state.saved[1].Status != domainStatistics.ScanWatermarkStatusIdle || state.saved[1].LastSeenID != 102 {
		t.Fatalf("unexpected watermark sequence: %+v", state.saved)
	}
	if rebuilder.calls != 1 || len(result.RecalcResults) != 1 {
		t.Fatalf("window rebuild calls/results = %d/%d, want 1/1", rebuilder.calls, len(result.RecalcResults))
	}
}

func TestBehaviorJourneyScanFailureKeepsPreviousWatermarkAndRemainsReplayable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.Local)
	previous := now.Add(-time.Hour)
	state := &behaviorScanStateStub{
		watermark: &domainStatistics.ScanWatermark{SourceName: domainStatistics.ScanSourceEntryResolve, OrgID: 9, LastSeenID: 77, LastSeenTime: &previous},
		facts: []domainStatistics.EntryResolveFact{
			{OrgID: 9, EntryID: 10, LogID: 101, OccurredAt: now.Add(-2 * time.Minute)},
			{OrgID: 9, EntryID: 10, LogID: 102, OccurredAt: now.Add(-time.Minute)},
		},
	}
	repo := &behaviorProjectorRepoStub{mutationErr: errors.New("daily mutation failed")}
	service := NewBehaviorJourneyScanService(&behaviorProjectorRunnerStub{}, repo, state, &behaviorScanRebuilderStub{}, nil, nil)

	result, err := service.ScanDue(context.Background(), BehaviorJourneyScanInput{
		OrgIDs: []int64{9}, Sources: []string{domainStatistics.ScanSourceEntryResolve}, Now: now,
	})
	if err != nil {
		t.Fatalf("ScanDue returned top-level error: %v", err)
	}
	got := result.SourceResults[0]
	if got.Error == "" || got.Scanned != 2 || got.Projected != 0 || got.Failed != 2 || got.WatermarkID != 77 {
		t.Fatalf("unexpected failed source result: %+v", got)
	}
	if len(state.saved) != 2 || state.saved[1].Status != domainStatistics.ScanWatermarkStatusFailed || state.saved[1].LastSeenID != 77 {
		t.Fatalf("failed watermark advanced: %+v", state.saved)
	}
}

func TestBehaviorJourneyScanDryRunDoesNotPersistOrProject(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.Local)
	state := &behaviorScanStateStub{facts: []domainStatistics.EntryResolveFact{{OrgID: 9, EntryID: 10, LogID: 101, OccurredAt: now}}}
	runner := &behaviorProjectorRunnerStub{}
	service := NewBehaviorJourneyScanService(runner, &behaviorProjectorRepoStub{}, state, &behaviorScanRebuilderStub{}, nil, nil)

	result, err := service.ScanDue(context.Background(), BehaviorJourneyScanInput{
		OrgIDs: []int64{9}, Sources: []string{domainStatistics.ScanSourceEntryResolve}, Now: now, DryRun: true, WindowRecalc: true,
	})
	if err != nil {
		t.Fatalf("ScanDue returned error: %v", err)
	}
	if got := result.SourceResults[0]; got.Scanned != 1 || got.Projected != 1 || got.WatermarkID != 101 {
		t.Fatalf("unexpected dry-run result: %+v", got)
	}
	if len(state.saved) != 0 || runner.txCount != 0 || len(result.RecalcResults) != 0 {
		t.Fatalf("dry run persisted/projected/rebuilt: saved=%d tx=%d recalc=%d", len(state.saved), runner.txCount, len(result.RecalcResults))
	}
}
