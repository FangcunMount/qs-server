package statistics

import (
	"context"
	actorctx "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type readStoreStub struct {
	scopedRange     appauthz.StoreRange
	cutoff          time.Time
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
	gets  int
	hit   bool
	stale bool
	value Overview
	sets  int
}

func (s *readCacheStub) Get(_ context.Context, _ int64, _ string, out any) (bool, bool) {
	s.gets++
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
	service := newScopedTestReadService(store)
	service.now = func() time.Time { return time.Date(2026, 7, 22, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)) }
	value, err := service.Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:evaluation:collection:assessments", "statistics"), 7, QueryFilter{})
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
	service := newScopedTestReadService(&readStoreStub{})
	_, err := service.Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:evaluation:collection:assessments", "statistics"), 7, QueryFilter{})
	if err == nil || !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("err=%v", err)
	}
}

func TestReadServiceRejectsTodayAndOversizedCustomWindow(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), DatabaseReadable: true}}
	service := newScopedTestReadService(store)
	if _, err := service.Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:evaluation:collection:assessments", "statistics"), 7, QueryFilter{Preset: "today"}); err == nil {
		t.Fatal("today must not be accepted")
	}
	if _, err := service.Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:evaluation:collection:assessments", "statistics"), 7, QueryFilter{Preset: "custom", From: "2025-01-01", To: "2026-07-21"}); err == nil {
		t.Fatal("oversized custom window must not be accepted")
	}
}

func TestReadServiceRejectsColdDatabaseFallbackWhilePublicationIsIncomplete(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)}}
	service := newScopedTestReadService(store)
	_, err := service.Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:evaluation:collection:assessments", "statistics"), 7, QueryFilter{})
	if err == nil || !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("err=%v", err)
	}
	if store.overviewReadHit != 0 {
		t.Fatalf("unsafe result tables were read %d times", store.overviewReadHit)
	}
}

func TestScopedOverviewDoesNotServeCompanyCacheWhenDatabaseUnsafe(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Now()}}
	cache := &readCacheStub{hit: true, value: Overview{OrgID: 7}}
	_, err := newScopedTestReadService(store, cache).Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AssessmentResource, "statistics"), 7, QueryFilter{})
	if !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("unsafe cache fallback: %v", err)
	}
	if store.overviewReadHit != 0 || cache.gets != 0 || cache.sets != 0 {
		t.Fatal("company cache or unsafe data read")
	}
}
func TestScopedOverviewBypassesWarmCompanyCache(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Now().AddDate(0, 0, -1), DatabaseReadable: true}}
	cache := &readCacheStub{hit: true, value: Overview{Metrics: OverviewMetrics{TesteeCount: 999}}}
	value, err := newScopedTestReadService(store, cache).Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AssessmentResource, "statistics"), 7, QueryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if value.Metrics.TesteeCount == 999 || store.overviewReadHit != 1 || cache.gets != 0 || cache.sets != 0 {
		t.Fatal("scoped request used company cache")
	}
}

func TestContentBatchIsBoundedByPublishedAsOfDate(t *testing.T) {
	asOf := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: asOf, DatabaseReadable: true}}
	service := newScopedTestReadService(store)
	if _, err := service.Contents(actorctx.WithGrantingUserID(context.Background(), 9), 7, []ContentRef{{Kind: "scale", Code: "S-1"}}); err != nil {
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
	service := newScopedTestReadService(store, cache)
	_, err := service.Overview(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:evaluation:collection:assessments", "statistics"), 7, QueryFilter{})
	if err == nil || !componenterrors.IsCode(err, code.ErrStatisticsNotReady) {
		t.Fatalf("err=%v", err)
	}
	if store.overviewReadHit != 1 || cache.sets != 0 {
		t.Fatalf("reads=%d cache_sets=%d", store.overviewReadHit, cache.sets)
	}
}

type scopedStatisticsAccessStub struct{}

func (scopedStatisticsAccessStub) ResolveStoreRange(context.Context, int64, int64, string, string) (appauthz.StoreRange, error) {
	return appauthz.StoreRange{StoreIDs: []uint64{7}}, nil
}
func newScopedTestReadService(store ReadStore, caches ...ReadCache) *ReadService {
	return NewScopedReadService(store, scopedStatisticsAccessStub{}, caches...)
}
func (s *readStoreStub) ScopedOverview(ctx context.Context, orgID int64, stores appauthz.StoreRange, from, to, cutoff time.Time) (ScopedOverviewData, error) {
	s.scopedRange = stores
	s.cutoff = cutoff
	metrics, err := s.Overview(ctx, orgID, from, to)
	return ScopedOverviewData{Metrics: metrics}, err
}

func TestScopedOverviewRequiresAuthenticatedOperator(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), DatabaseReadable: true}}
	service := newScopedTestReadService(store)
	_, err := service.Overview(authztest.WithPermission(context.Background(), appauthz.AssessmentResource, "statistics"), 7, QueryFilter{})
	if !componenterrors.IsCode(err, code.ErrPermissionDenied) {
		t.Fatalf("missing user accepted: %v", err)
	}
	if store.overviewReadHit != 0 {
		t.Fatal("denied request read scoped aggregates")
	}
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AssessmentResource, "statistics")
	if _, err := service.Overview(ctx, 7, QueryFilter{}); err != nil {
		t.Fatal(err)
	}
	if len(store.scopedRange.StoreIDs) != 1 || store.scopedRange.StoreIDs[0] != 7 || store.cutoff.Format("2006-01-02") != "2026-07-22" {
		t.Fatalf("wrong scope/cutoff %v %v", store.scopedRange, store.cutoff)
	}
}

func (s *readStoreStub) ScopedClinicians(_ context.Context, _ int64, stores appauthz.StoreRange, _ *uint64, _ *int64, _, _ time.Time, _, _ int) ([]ClinicianItem, int64, error) {
	s.scopedRange = stores
	return nil, 0, nil
}
func (s *readStoreStub) ScopedEntries(_ context.Context, _ int64, stores appauthz.StoreRange, _, _ *uint64, _ *bool, _, _ time.Time, _, _ int) ([]EntryItem, int64, error) {
	s.scopedRange = stores
	return nil, 0, nil
}
func TestScopedDimensionsNeverUseCompanyCache(t *testing.T) {
	for _, kind := range []string{"clinicians", "entries"} {
		t.Run(kind, func(t *testing.T) {
			store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Now().AddDate(0, 0, -1), DatabaseReadable: true}}
			cache := &readCacheStub{hit: true}
			service := newScopedTestReadService(store, cache)
			ctx := authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AssessmentResource, "statistics")
			var err error
			if kind == "clinicians" {
				_, err = service.Clinicians(ctx, 7, nil, nil, QueryFilter{}, 1, 10)
			} else {
				_, err = service.Entries(ctx, 7, nil, nil, nil, QueryFilter{}, 1, 10)
			}
			if err != nil {
				t.Fatal(err)
			}
			if cache.gets != 0 || cache.sets != 0 || len(store.scopedRange.StoreIDs) != 1 || store.scopedRange.StoreIDs[0] != 7 {
				t.Fatal("scope lost or company cache used")
			}
		})
	}
}

func (s *readStoreStub) ScopedContentBatch(_ context.Context, _ int64, asOf time.Time, _ []ScopedContentRef) ([]ContentItem, error) {
	s.contentAsOf = asOf
	return nil, nil
}

type contentScopeAccess struct{}

func (contentScopeAccess) ResolveStoreRange(_ context.Context, _ int64, _ int64, resource, action string) (appauthz.StoreRange, error) {
	if resource == appauthz.QuestionnaireResource && action == "statistics" {
		return appauthz.StoreRange{StoreIDs: []uint64{7}}, nil
	}
	if resource == appauthz.AssessmentModelResource && action == "read" {
		return appauthz.StoreRange{StoreIDs: []uint64{8}}, nil
	}
	return appauthz.StoreRange{}, componenterrors.WithCode(code.ErrPermissionDenied, "wrong content capability")
}
func TestContentBatchResolvesEachCapabilityAndBypassesCache(t *testing.T) {
	store := &readStoreStub{snapshot: &Snapshot{AsOfDate: time.Now().AddDate(0, 0, -1), DatabaseReadable: true}}
	cache := &readCacheStub{hit: true}
	service := NewScopedReadService(store, contentScopeAccess{}, cache)
	ctx := actorctx.WithGrantingUserID(context.Background(), 9)
	refs := []ContentRef{{Kind: "questionnaire", Code: "Q"}, {Kind: "scale", Code: "S"}}
	scopes, err := service.contentRanges(ctx, 7, refs)
	if err != nil {
		t.Fatal(err)
	}
	if scopes[0].Stores.StoreIDs[0] != 7 || scopes[1].Stores.StoreIDs[0] != 8 {
		t.Fatal("content scope borrowed")
	}
	if _, err := service.Contents(ctx, 7, refs); err != nil {
		t.Fatal(err)
	}
	if cache.gets != 0 || cache.sets != 0 {
		t.Fatal("company cache used")
	}
	if _, err := service.Contents(context.Background(), 7, refs); !componenterrors.IsCode(err, code.ErrPermissionDenied) {
		t.Fatalf("missing operator: %v", err)
	}
}
