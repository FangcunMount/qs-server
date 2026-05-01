package testee

import (
	"context"
	"testing"
	"time"

	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

func TestListTesteesUsesUnifiedFilterForUnrestrictedQueries(t *testing.T) {
	repo := &queryServiceRepoStub{
		listItems:  []actorreadmodel.TesteeRow{makeQueryServiceTesteeRow(21, time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC))},
		countValue: 7,
	}
	service := NewQueryService(repo)
	keyFocus := false
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local)

	result, err := service.ListTestees(context.Background(), ListTesteeDTO{
		OrgID:          1,
		Name:           "张",
		Tags:           []string{"重点"},
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
	if repo.lastFilter.Name != "张" || len(repo.lastFilter.Tags) != 1 || repo.lastFilter.Tags[0] != "重点" {
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

func TestListTesteesUsesUnifiedFilterForRestrictedQueries(t *testing.T) {
	repo := &queryServiceRepoStub{
		listItems:  []actorreadmodel.TesteeRow{makeQueryServiceTesteeRow(31, time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC))},
		countValue: 1,
	}
	service := NewQueryService(repo)
	keyFocus := true

	result, err := service.ListTestees(context.Background(), ListTesteeDTO{
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
	service := NewQueryService(repo)

	result, err := service.ListKeyFocus(context.Background(), 1, 0, 10)
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
	return s.listItems, nil
}
func (s *queryServiceRepoStub) CountTesteesByProfileIDs(context.Context, []uint64) (int64, error) {
	return s.countValue, nil
}

func makeQueryServiceTesteeRow(id uint64, createdAt time.Time) actorreadmodel.TesteeRow {
	return actorreadmodel.TesteeRow{
		ID:        id,
		OrgID:     1,
		Name:      "testee",
		Gender:    1,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}
