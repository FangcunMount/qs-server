package statisticsv2

import (
	"context"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type readStoreStub struct {
	snapshot *Snapshot
	from, to time.Time
}

func (s *readStoreStub) LatestSuccessfulSnapshot(context.Context, int64) (*Snapshot, error) {
	return s.snapshot, nil
}
func (s *readStoreStub) SnapshotForDate(context.Context, int64, time.Time) (*Snapshot, error) {
	return s.snapshot, nil
}
func (s *readStoreStub) Overview(_ context.Context, _ int64, from, to time.Time) (OverviewMetrics, error) {
	s.from, s.to = from, to
	return OverviewMetrics{}, nil
}
func (*readStoreStub) OverviewTrends(context.Context, int64, time.Time, time.Time) (OverviewTrends, error) {
	return OverviewTrends{}, nil
}
func (*readStoreStub) ListClinicians(context.Context, int64, *uint64, *int64, time.Time, time.Time, int, int) ([]ClinicianItem, int64, error) {
	return nil, 0, nil
}
func (*readStoreStub) ListEntries(context.Context, int64, *uint64, *uint64, *bool, time.Time, time.Time, int, int) ([]EntryItem, int64, error) {
	return nil, 0, nil
}
func (*readStoreStub) CurrentClinicianID(context.Context, int64, int64) (uint64, error) {
	return 1, nil
}
func (*readStoreStub) CurrentClinicianTesteeSummary(context.Context, int64, uint64, time.Time, time.Time) (TesteeSummary, error) {
	return TesteeSummary{}, nil
}
func (*readStoreStub) ContentBatch(context.Context, int64, []ContentRef) ([]ContentItem, error) {
	return nil, nil
}

func TestReadServiceDefaultsToSevenCompleteShanghaiDays(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), SnapshotAt: time.Date(2026, 7, 22, 0, 30, 0, 0, time.FixedZone("CST", 8*3600))}}
	service := NewReadService(store)
	service.now = func() time.Time { return time.Date(2026, 7, 22, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)) }
	value, err := service.Overview(context.Background(), 7, QueryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if value.TimeRange.Preset != "7d" || value.TimeRange.From.Format("2006-01-02") != "2026-07-15" || value.TimeRange.To.Format("2006-01-02") != "2026-07-21" || value.Freshness.IsStale {
		t.Fatalf("value=%+v", value)
	}
	if store.from.Format("2006-01-02") != "2026-07-15" || store.to.Format("2006-01-02") != "2026-07-22" {
		t.Fatalf("bounds=%s..%s", store.from, store.to)
	}
}

func TestReadServiceReturnsStatisticsNotReadyWithoutSuccessfulRun(t *testing.T) {
	service := NewReadService(&readStoreStub{})
	_, err := service.Overview(context.Background(), 7, QueryFilter{})
	if err == nil || !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("err=%v", err)
	}
}

func TestReadServiceRejectsTodayAndOversizedCustomWindow(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)}}
	service := NewReadService(store)
	if _, err := service.Overview(context.Background(), 7, QueryFilter{Preset: "today"}); err == nil {
		t.Fatal("today must not be accepted")
	}
	if _, err := service.Overview(context.Background(), 7, QueryFilter{Preset: "custom", From: "2025-01-01", To: "2026-07-21"}); err == nil {
		t.Fatal("oversized custom window must not be accepted")
	}
}
