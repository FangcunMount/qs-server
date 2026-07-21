package statisticsv2

import (
	"context"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type readStoreStub struct {
	snapshot        *Snapshot
	nextSnapshot    *Snapshot
	snapshotCalls   int
	from, to        time.Time
	contentAsOf     time.Time
	overviewReadHit int
}

func (s *readStoreStub) LatestVisibleSnapshot(context.Context, int64) (*Snapshot, error) {
	s.snapshotCalls++
	if s.snapshotCalls > 1 && s.nextSnapshot != nil {
		return s.nextSnapshot, nil
	}
	return s.snapshot, nil
}
func (s *readStoreStub) SnapshotForDate(context.Context, int64, time.Time) (*Snapshot, error) {
	return s.snapshot, nil
}
func (s *readStoreStub) Overview(_ context.Context, _ int64, from, to time.Time) (OverviewMetrics, error) {
	s.from, s.to = from, to
	s.overviewReadHit++
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
func (s *readStoreStub) ContentBatch(_ context.Context, _ int64, asOf time.Time, _ []ContentRef) ([]ContentItem, error) {
	s.contentAsOf = asOf
	return nil, nil
}

type readCacheStub struct {
	hit   bool
	stale bool
	value Overview
	sets  int
}

func (s *readCacheStub) Get(_ context.Context, _ int64, _ string, out any) (bool, bool) {
	if !s.hit {
		return false, false
	}
	value, ok := out.(*Overview)
	if ok {
		*value = s.value
	}
	return true, s.stale
}
func (s *readCacheStub) Set(context.Context, int64, string, any) { s.sets++ }

func TestReadServiceDefaultsToSevenCompleteShanghaiDays(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), SnapshotAt: time.Date(2026, 7, 22, 0, 30, 0, 0, time.FixedZone("CST", 8*3600)), DatabaseReadable: true}}
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
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), DatabaseReadable: true}}
	service := NewReadService(store)
	if _, err := service.Overview(context.Background(), 7, QueryFilter{Preset: "today"}); err == nil {
		t.Fatal("today must not be accepted")
	}
	if _, err := service.Overview(context.Background(), 7, QueryFilter{Preset: "custom", From: "2025-01-01", To: "2026-07-21"}); err == nil {
		t.Fatal("oversized custom window must not be accepted")
	}
}

func TestReadServiceRejectsColdDatabaseFallbackWhilePublicationIsIncomplete(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)}}
	service := NewReadService(store)
	_, err := service.Overview(context.Background(), 7, QueryFilter{})
	if err == nil || !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("err=%v", err)
	}
	if store.overviewReadHit != 0 {
		t.Fatalf("unsafe result tables were read %d times", store.overviewReadHit)
	}
}

func TestReadServiceKeepsServingPublishedCacheWhileDatabaseIsUnsafe(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)}}
	cache := &readCacheStub{hit: true, value: Overview{OrgID: 7, Freshness: Freshness{AsOfDate: "2026-07-20"}}}
	service := NewReadService(store, cache)
	value, err := service.Overview(context.Background(), 7, QueryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if value.Freshness.AsOfDate != "2026-07-20" || store.overviewReadHit != 0 {
		t.Fatalf("value=%+v reads=%d", value, store.overviewReadHit)
	}
}

func TestContentBatchIsBoundedByPublishedAsOfDate(t *testing.T) {
	asOf := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: asOf, DatabaseReadable: true}}
	service := NewReadService(store)
	if _, err := service.Contents(context.Background(), 7, []ContentRef{{Kind: "scale", Code: "S-1"}}); err != nil {
		t.Fatal(err)
	}
	if store.contentAsOf.Format("2006-01-02") != "2026-07-21" {
		t.Fatalf("content as_of=%s", store.contentAsOf)
	}
}

func TestReadServiceDiscardsDatabaseResultWhenPublicationChangesDuringRead(t *testing.T) {
	asOf := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	store := &readStoreStub{
		snapshot:     &Snapshot{VisibleRunID: 10, AsOfDate: asOf, DatabaseReadable: true},
		nextSnapshot: &Snapshot{VisibleRunID: 11, AsOfDate: asOf, DatabaseReadable: true},
	}
	cache := &readCacheStub{}
	service := NewReadService(store, cache)
	_, err := service.Overview(context.Background(), 7, QueryFilter{})
	if err == nil || !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("err=%v", err)
	}
	if store.overviewReadHit != 1 || cache.sets != 0 {
		t.Fatalf("reads=%d cache_sets=%d", store.overviewReadHit, cache.sets)
	}
}
