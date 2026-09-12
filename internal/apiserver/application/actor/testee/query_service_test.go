package testee

import (
	"context"
	"errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"testing"
	"time"

	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

func TestListTesteesUsesUnifiedFilterForUnrestrictedQueries(t *testing.T) {
	repo := &queryServiceRepoStub{
		listItems:  []actorreadmodel.TesteeRow{makeQueryServiceTesteeRow(21, time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC))},
		countValue: 7,
	}
	service := NewQueryServiceWithAssessmentSummary(repo, nil)
	keyFocus := false
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local)

	result, err := service.ListTestees(scopeQueryContext(authztest.WithPermission(context.Background(), "qs:evaluation:collection:assessments", "read")), ListTesteeDTO{
		OrgID:          1,
		Name:           "张",
		KeyFocus:       &keyFocus,
		CreatedAtStart: &start,
		CreatedAtEnd:   &end,
		Offset:         10,
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("ListTestees returned error: %v", err)
	}
	if repo.listCalls != 1 || repo.countCalls != 1 {
		t.Fatalf("expected unrestricted list/count to be called once, got list=%d count=%d", repo.listCalls, repo.countCalls)
	}
	if repo.lastFilter.RestrictToAccessScope {
		t.Fatalf("expected unrestricted filter, got %+v", repo.lastFilter)
	}
	if repo.lastFilter.Name != "张" {
		t.Fatalf("unexpected filter passed to repo: %+v", repo.lastFilter)
	}
	if repo.lastFilter.KeyFocus == nil || *repo.lastFilter.KeyFocus != false {
		t.Fatalf("expected key focus filter=false, got %+v", repo.lastFilter.KeyFocus)
	}
	if repo.lastFilter.CreatedAtStart == nil || !repo.lastFilter.CreatedAtStart.Equal(start) {
		t.Fatalf("expected created_at start to be passed through")
	}
	if repo.lastFilter.CreatedAtEnd == nil || !repo.lastFilter.CreatedAtEnd.Equal(end) {
		t.Fatalf("expected created_at end to be passed through")
	}
	if result.TotalCount != 7 || len(result.Items) != 1 || result.Items[0].ID != 21 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

type assessmentSummaryReaderStub struct {
	calls  int
	ids    []uint64
	err    error
	values map[uint64]actorreadmodel.AssessmentSummary
}

func (s *assessmentSummaryReaderStub) ReadAssessmentSummaries(_ context.Context, _ int64, ids []uint64) (map[uint64]actorreadmodel.AssessmentSummary, error) {
	s.calls++
	s.ids = append([]uint64(nil), ids...)
	return s.values, s.err
}

func TestListTesteesReadsAssessmentSummaryOnceAndIgnoresSnapshots(t *testing.T) {
	stale := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	store := uint64(7)
	repo := &queryServiceRepoStub{listItems: []actorreadmodel.TesteeRow{{ID: 21, OrgID: 1, StoreID: &store, Name: "testee", LastAssessmentAt: &stale, TotalAssessments: 99, LastRiskLevel: "low"}}, countValue: 1}
	summaries := &assessmentSummaryReaderStub{values: map[uint64]actorreadmodel.AssessmentSummary{21: {TesteeID: 21, TotalEvaluated: 2, LastEvaluatedAt: &latest, RiskLevel: "high"}}}
	service := NewQueryServiceWithAssessmentSummary(repo, summaries)
	result, err := service.ListTestees(scopeQueryContext(authztest.WithPermission(context.Background(), "qs:evaluation:collection:assessments", "read")), ListTesteeDTO{OrgID: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if summaries.calls != 1 {
		t.Fatalf("summary calls=%d want=1", summaries.calls)
	}
	item := result.Items[0]
	if item.TotalAssessments != 2 || item.LastAssessmentAt == nil || !item.LastAssessmentAt.Equal(latest) || item.LastRiskLevel != "high" {
		t.Fatalf("item=%+v", item)
	}
}

func TestListTesteesPropagatesAssessmentSummaryFailure(t *testing.T) {
	repo := &queryServiceRepoStub{listItems: []actorreadmodel.TesteeRow{makeQueryServiceTesteeRow(21, time.Now())}, countValue: 1}
	summaries := &assessmentSummaryReaderStub{err: errors.New("summary database unavailable")}
	service := NewQueryServiceWithAssessmentSummary(repo, summaries)
	if _, err := service.ListTestees(scopeQueryContext(authztest.WithPermission(context.Background(), "qs:evaluation:collection:assessments", "read")), ListTesteeDTO{OrgID: 1, Limit: 10}); err == nil {
		t.Fatal("summary failure must fail the page")
	}
}

func TestListTesteesUsesUnifiedFilterForRestrictedQueries(t *testing.T) {
	repo := &queryServiceRepoStub{
		listItems:  []actorreadmodel.TesteeRow{makeQueryServiceTesteeRow(31, time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC))},
		countValue: 1,
	}
	service := NewQueryServiceWithAssessmentSummary(repo, nil)
	keyFocus := true

	result, err := service.ListTestees(scopeQueryContext(authztest.WithPermission(context.Background(), "qs:evaluation:collection:assessments", "read")), ListTesteeDTO{
		OrgID:                 1,
		KeyFocus:              &keyFocus,
		AccessibleTesteeIDs:   []uint64{31, 32},
		RestrictToAccessScope: true,
		Offset:                0,
		Limit:                 10,
	})
	if err != nil {
		t.Fatalf("ListTestees returned error: %v", err)
	}
	if repo.listCalls != 1 || repo.countCalls != 1 {
		t.Fatalf("expected restricted list/count to be called once, got list=%d count=%d", repo.listCalls, repo.countCalls)
	}
	if len(repo.lastFilter.AccessibleTesteeIDs) != 2 || repo.lastFilter.AccessibleTesteeIDs[0] != 31 || repo.lastFilter.AccessibleTesteeIDs[1] != 32 {
		t.Fatalf("unexpected restricted ids: %+v", repo.lastFilter.AccessibleTesteeIDs)
	}
	if repo.lastFilter.KeyFocus == nil || *repo.lastFilter.KeyFocus != true {
		t.Fatalf("expected key focus filter=true, got %+v", repo.lastFilter.KeyFocus)
	}
	if result.TotalCount != 1 || len(result.Items) != 1 || result.Items[0].ID != 31 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestListKeyFocusDelegatesToUnifiedListFlow(t *testing.T) {
	repo := &queryServiceRepoStub{
		listItems:  []actorreadmodel.TesteeRow{makeQueryServiceTesteeRow(41, time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC))},
		countValue: 3,
	}
	service := NewQueryServiceWithAssessmentSummary(repo, nil)

	result, err := service.ListKeyFocus(scopeQueryContext(context.Background()), 1, 0, 10)
	if err != nil {
		t.Fatalf("ListKeyFocus returned error: %v", err)
	}
	if repo.listCalls != 1 || repo.countCalls != 1 {
		t.Fatalf("expected ListKeyFocus to use unified unrestricted list/count, got list=%d count=%d", repo.listCalls, repo.countCalls)
	}
	if repo.lastFilter.KeyFocus == nil || *repo.lastFilter.KeyFocus != true {
		t.Fatalf("expected key focus filter=true, got %+v", repo.lastFilter.KeyFocus)
	}
	if result.TotalCount != 3 || len(result.Items) != 1 || result.Items[0].ID != 41 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

type queryServiceRepoStub struct {
	item       *actorreadmodel.TesteeRow
	listItems  []actorreadmodel.TesteeRow
	countValue int64
	listCalls  int
	countCalls int
	lastFilter actorreadmodel.TesteeFilter
}

func (s *queryServiceRepoStub) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return s.item, nil
}
func (s *queryServiceRepoStub) FindTesteeByProfile(context.Context, int64, uint64) (*actorreadmodel.TesteeRow, error) {
	return s.item, nil
}
func (s *queryServiceRepoStub) ListTestees(_ context.Context, filter actorreadmodel.TesteeFilter) ([]actorreadmodel.TesteeRow, error) {
	s.listCalls++
	s.lastFilter = filter
	return s.listItems, nil
}
func (s *queryServiceRepoStub) CountTestees(_ context.Context, filter actorreadmodel.TesteeFilter) (int64, error) {
	s.countCalls++
	s.lastFilter = filter
	return s.countValue, nil
}
func (s *queryServiceRepoStub) ListTesteesByProfileIDs(context.Context, []uint64, int, int) ([]actorreadmodel.TesteeRow, error) {
	s.listCalls++
	return s.listItems, nil
}
func (s *queryServiceRepoStub) CountTesteesByProfileIDs(context.Context, []uint64) (int64, error) {
	s.countCalls++
	return s.countValue, nil
}

func makeQueryServiceTesteeRow(id uint64, createdAt time.Time) actorreadmodel.TesteeRow {
	store := uint64(7)
	return actorreadmodel.TesteeRow{
		ID:        id,
		StoreID:   &store,
		OrgID:     1,
		Name:      "testee",
		Gender:    1,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func TestStoreFilterRetainsAccessScopeAndRejectsAmbiguity(t *testing.T) {
	repo := &queryServiceRepoStub{}
	service := NewQueryServiceWithAssessmentSummary(repo, nil)
	id := uint64(7)
	dto := ListTesteeDTO{OrgID: 1, StoreID: &id, UnassignedStore: true}
	if _, err := service.ListTestees(scopeQueryContext(context.Background()), dto); err == nil {
		t.Fatal("ambiguous store filter accepted")
	}
	if repo.listCalls != 0 {
		t.Fatal("invalid filter read data")
	}
	dto.UnassignedStore = false
	dto.RestrictToAccessScope = true
	dto.AccessibleTesteeIDs = []uint64{21}
	dto.Limit = 1
	if _, err := service.ListTestees(scopeQueryContext(context.Background()), dto); err != nil {
		t.Fatal(err)
	}
	if repo.lastFilter.StoreID == nil || *repo.lastFilter.StoreID != 7 || !repo.lastFilter.RestrictToAccessScope || len(repo.lastFilter.AccessibleTesteeIDs) != 1 {
		t.Fatal("store filter replaced access boundary")
	}
}

func scopeQueryContext(ctx context.Context) context.Context {
	old, _ := appauthz.FromContext(ctx)
	snapshot := appauthz.Snapshot{AuthzVersion: 1, ScopeContractVersion: 1}
	if old != nil {
		snapshot.Permissions = append(snapshot.Permissions, old.Permissions...)
		for i := range snapshot.Permissions {
			snapshot.Permissions[i].Scopes = []appauthz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}
		}
	}
	snapshot.Permissions = append(snapshot.Permissions, appauthz.Permission{Resource: "qs:actor:collection:testees", Action: "list", Mode: appauthz.AuthorizationModeUnconditional, Scopes: []appauthz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}})
	return appauthz.WithSnapshot(ctx, &snapshot)
}

func TestBackendListRequiresStoreScopeBeforeReading(t *testing.T) {
	repo := &queryServiceRepoStub{}
	service := NewQueryServiceWithAssessmentSummary(repo, nil)
	if _, err := service.ListTestees(context.Background(), ListTesteeDTO{OrgID: 1}); err == nil {
		t.Fatal("missing scope accepted")
	}
	if repo.listCalls != 0 || repo.countCalls != 0 {
		t.Fatal("denied list read data")
	}
	if _, err := service.ListTestees(scopeQueryContext(context.Background()), ListTesteeDTO{OrgID: 1}); err != nil {
		t.Fatal(err)
	}
	if !repo.lastFilter.RestrictToStoreScope || len(repo.lastFilter.AllowedStoreIDs) != 1 || repo.lastFilter.AllowedStoreIDs[0] != 7 {
		t.Fatal("range not forwarded to list/count")
	}
}

func TestSummaryUsesItsOwnActionRange(t *testing.T) {
	rowA := makeQueryServiceTesteeRow(21, time.Now())
	rowB := makeQueryServiceTesteeRow(22, time.Now())
	storeB := uint64(8)
	rowB.StoreID = &storeB
	rowB.LastRiskLevel = "high"
	rowB.TotalAssessments = 9
	rowB.LastAssessmentAt = &rowB.CreatedAt
	repo := &queryServiceRepoStub{listItems: []actorreadmodel.TesteeRow{rowA, rowB}, countValue: 2}
	summaries := &assessmentSummaryReaderStub{values: map[uint64]actorreadmodel.AssessmentSummary{21: {TesteeID: 21, TotalEvaluated: 3}, 22: {TesteeID: 22, TotalEvaluated: 9, RiskLevel: "high"}}}
	ctx := scopeQueryContext(authztest.WithPermission(context.Background(), appauthz.AssessmentResource, "read"))
	snapshot, _ := appauthz.FromContext(ctx)
	snapshot.Permissions[1].Scopes[0].StoreIDs = []uint64{7, 8}
	result, err := NewQueryServiceWithAssessmentSummary(repo, summaries).ListTestees(ctx, ListTesteeDTO{OrgID: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if summaries.calls != 1 || len(summaries.ids) != 1 || summaries.ids[0] != 21 {
		t.Fatalf("read unauthorized summary IDs: %v", summaries.ids)
	}
	if result.Items[0].TotalAssessments != 3 {
		t.Fatal("authorized summary missing")
	}
	hidden := result.Items[1]
	if hidden.TotalAssessments != 0 || hidden.LastRiskLevel != "" || hidden.LastAssessmentAt != nil {
		t.Fatalf("summary leaked: %+v", hidden)
	}
}

func TestTesteeDetailsRejectOutsideRangeBeforeSummaryRead(t *testing.T) {
	for _, unassigned := range []bool{false, true} {
		row := makeQueryServiceTesteeRow(21, time.Now())
		if unassigned {
			row.StoreID = nil
		} else {
			store := uint64(8)
			row.StoreID = &store
		}
		repo := &queryServiceRepoStub{item: &row}
		summaries := &assessmentSummaryReaderStub{}
		service := NewQueryServiceWithAssessmentSummary(repo, summaries)
		ctx := scopeQueryContext(authztest.WithPermission(context.Background(), "qs:actor:collection:testees", "read"))
		if _, err := service.GetByID(ctx, 21); err == nil {
			t.Fatal("out-of-range detail accepted")
		}
		if _, err := service.FindByProfile(ctx, 1, 21); err == nil {
			t.Fatal("out-of-range profile accepted")
		}
		if summaries.calls != 0 {
			t.Fatal("denied detail queried summaries")
		}
	}
}

func TestProfileListReservedForSelfService(t *testing.T) {
	row := makeQueryServiceTesteeRow(21, time.Now())
	row.StoreID = nil // Guardian access does not require a service store.
	repo := &queryServiceRepoStub{listItems: []actorreadmodel.TesteeRow{row}, countValue: 1}
	backend := NewQueryServiceWithAssessmentSummary(repo, nil)
	if _, err := backend.ListByProfileIDs(scopeQueryContext(context.Background()), []uint64{1}, 0, 10); err == nil {
		t.Fatal("backend bypassed company and store filters")
	}
	if repo.listCalls != 0 || repo.countCalls != 0 {
		t.Fatal("denied backend request read data")
	}
	self := NewSelfServiceQueryServiceWithAssessmentSummary(repo, nil)
	result, err := self.ListByProfileIDs(context.Background(), []uint64{1}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalCount != 1 || len(result.Items) != 1 || repo.listCalls != 1 || repo.countCalls != 1 {
		t.Fatalf("self-service changed: %+v", result)
	}
}

func TestUnassignedInventoryRequiresHeadquartersAndNeverEnrichesResults(t *testing.T) {
	for _, kind := range []string{"store", "all_stores", "headquarters"} {
		t.Run(kind, func(t *testing.T) {
			ctx := scopeQueryContext(context.Background())
			snapshot, _ := appauthz.FromContext(ctx)
			if kind != "store" {
				snapshot.Permissions[0].Scopes = []appauthz.DataScope{{OrgID: 1, Kind: "all_stores"}}
			}
			if kind == "headquarters" {
				snapshot.Permissions[0].Resource = "qs:*:*:*"
				snapshot.Permissions[0].Action = "*"
			}
			repo := &queryServiceRepoStub{listItems: []actorreadmodel.TesteeRow{{ID: 1, OrgID: 1, TotalAssessments: 99, LastRiskLevel: "high"}}, countValue: 1}
			summary := &assessmentSummaryReaderStub{}
			result, err := NewQueryServiceWithAssessmentSummary(repo, summary).ListTestees(ctx, ListTesteeDTO{OrgID: 1, UnassignedStore: true, Limit: 10})
			if kind != "headquarters" {
				if err == nil || repo.listCalls != 0 || repo.countCalls != 0 {
					t.Fatal("unassigned scope leaked")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !repo.lastFilter.UnassignedStore || !repo.lastFilter.AllAssignedStores || !repo.lastFilter.RestrictToStoreScope {
				t.Fatal("lost inventory filter")
			}
			if summary.calls != 0 || result.Items[0].TotalAssessments != 0 || result.Items[0].LastRiskLevel != "" {
				t.Fatal("unassigned professional data leaked")
			}
		})
	}
}
