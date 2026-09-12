package statistics

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"testing"
	"time"
)

type operationsScope struct {
	value authz.StoreRange
	err   error
}

func (s *operationsScope) ResolveStoreRange(_ context.Context, _ int64, _ int64, resource, action string) (authz.StoreRange, error) {
	if resource != OperationsResource || action != "read" {
		return authz.StoreRange{}, errors.New("wrong action")
	}
	return s.value, s.err
}

type operationsStoreStub struct {
	readStoreStub
	coverage   []domain.InstantRange
	population int64
	reads      int
	scope      authz.StoreRange
}

func (s *operationsStoreStub) OperationsCoverage(context.Context, int64, uint64, time.Time, time.Time) ([]domain.InstantRange, error) {
	return s.coverage, nil
}
func (s *operationsStoreStub) OperationsPopulation(context.Context, int64, authz.StoreRange) ([]OperationStore, error) {
	return []OperationStore{{ID: 10, Name: "A", CurrentServiceCount: s.population}}, nil
}
func (s *operationsStoreStub) OperationsActivity(_ context.Context, _ int64, scope authz.StoreRange, from, to time.Time) ([]ActivityRow, error) {
	s.reads++
	s.scope = scope
	return []ActivityRow{{StoreID: 10, Date: from, Submissions: 4, Completions: 3}}, nil
}

type operationsCache struct {
	rows []ActivityRow
	key  string
	gets int
}

func (c *operationsCache) Get(_ context.Context, _ int64, key string, out any) (bool, bool) {
	c.gets++
	if key != c.key || c.rows == nil {
		return false, false
	}
	*out.(*[]ActivityRow) = append([]ActivityRow(nil), c.rows...)
	return true, false
}
func (c *operationsCache) Set(_ context.Context, _ int64, key string, value any) {
	c.key = key
	c.rows = append([]ActivityRow(nil), value.([]ActivityRow)...)
}
func operationsFixture() (*ReadService, *operationsStoreStub, *operationsScope, *operationsCache, context.Context) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, domain.Shanghai)
	to := from.AddDate(0, 0, 11)
	store := &operationsStoreStub{readStoreStub: readStoreStub{snapshot: &Snapshot{AsOfDate: to.AddDate(0, 0, -1), VisibleRunID: 7, DatabaseReadable: true}}, coverage: []domain.InstantRange{{From: from, To: to}}, population: 2}
	scope := &operationsScope{value: authz.StoreRange{StoreIDs: []uint64{10}}}
	cache := &operationsCache{}
	service := NewScopedReadService(store, scope, cache)
	service.now = func() time.Time { return to.Add(12 * time.Hour) }
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), OperationsResource, "read")
	return service, store, scope, cache, ctx
}
func TestOperationsAlwaysRechecksAuthorizationAndLivePopulation(t *testing.T) {
	service, store, scope, cache, ctx := operationsFixture()
	first, err := service.Operations(ctx, 1, OperationsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Submissions != 4 || first.Completions != 3 || first.Unknown != nil || first.CurrentServiceCount != 2 || first.WorkloadThrough != "2026-09-11" {
		t.Fatalf("first=%+v", first)
	}
	store.population = 7
	second, err := service.Operations(ctx, 1, OperationsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if second.CurrentServiceCount != 7 || store.reads != 1 {
		t.Fatal("population was cached or workload cache not used")
	}
	prior := cache.gets
	scope.err = errors.New("operator disabled")
	if _, err := service.Operations(ctx, 1, OperationsFilter{}); err == nil || cache.gets != prior {
		t.Fatal("disabled operator read cache")
	}
	if _, err := service.Operations(context.Background(), 1, OperationsFilter{}); err == nil {
		t.Fatal("missing permission allowed")
	}
}
func TestOperationsRejectsUnpublishedCoverageAndOutOfRangeStore(t *testing.T) {
	service, store, _, cache, ctx := operationsFixture()
	if _, err := service.Operations(ctx, 1, OperationsFilter{StoreIDs: []uint64{20}}); err == nil || cache.gets != 0 {
		t.Fatal("unauthorized store read")
	}
	store.coverage = nil
	if _, err := service.Operations(ctx, 1, OperationsFilter{}); err == nil {
		t.Fatal("unbuilt history presented as zero")
	}
}
func TestOperationsCompanyReconcilesStoresAndUnknownWithoutCompletionRate(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, domain.Shanghai)
	window := domain.InstantRange{From: from, To: from.AddDate(0, 0, 1)}
	rows := []ActivityRow{{StoreID: 10, Date: from, Submissions: 4, Completions: 2}, {StoreID: 20, Date: from, Submissions: 3, Completions: 4}, {UnknownReason: "legacy_start_not_captured", Date: from, Submissions: 2, Completions: 1}}
	result := buildOperations(authz.StoreRange{AllStores: true}, window, window, &Snapshot{}, []OperationStore{{ID: 10, CurrentServiceCount: 1}, {ID: 20, CurrentServiceCount: 2}}, rows, time.Now())
	if result.Submissions != 9 || result.Completions != 7 || result.Unknown.Submissions != 2 || result.Stores[0].Submissions+result.Stores[1].Submissions+result.Unknown.Submissions != result.Submissions {
		t.Fatal("company does not reconcile")
	}
	if result.CurrentServiceCount != 3 || len(result.Daily) != 1 || result.Daily[0].Completions != 7 {
		t.Fatal("population or daily mismatch")
	}
}
func TestOperationsRangeAndCoverageUseHalfOpenShanghaiDates(t *testing.T) {
	now := time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC)
	r, err := operationRange(now, OperationsFilter{})
	if err != nil || r.From.Format("2006-01-02") != "2026-10-01" || r.To.Format("2006-01-02") != "2026-10-02" {
		t.Fatal("month boundary wrong")
	}
	middle := r.From.Add(12 * time.Hour)
	if coversOperationWindow([]domain.InstantRange{{From: r.From, To: middle.Add(-time.Second)}, {From: middle, To: r.To}}, r) {
		t.Fatal("gap treated as covered")
	}
	if !coversOperationWindow([]domain.InstantRange{{From: middle, To: r.To}, {From: r.From, To: middle}}, r) {
		t.Fatal("adjacent repairs not combined")
	}
	all := authz.StoreRange{AllStores: true}
	selected, err := requestedOperationScope(all, []uint64{20, 10})
	if err != nil || selected.AllStores || selected.StoreIDs[0] != 10 {
		t.Fatal("single company selection still exposes unknown bucket")
	}
}
