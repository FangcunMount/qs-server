package testee

import (
	"context"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestListTesteesUsesUnifiedFilterForUnrestrictedQueries(t *testing.T) {
	repo := &queryServiceRepoStub{
		listByOrgItems: []*domain.Testee{makeQueryServiceTestee(21, time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC))},
		countValue:     7,
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
	if repo.listByOrgCalls != 1 || repo.countCalls != 1 {
		t.Fatalf("expected unrestricted list/count to be called once, got list=%d count=%d", repo.listByOrgCalls, repo.countCalls)
	}
	if repo.listByOrgAndIDsCalls != 0 || repo.countByOrgAndIDsCalls != 0 {
		t.Fatalf("expected restricted methods not to be used")
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
		listByOrgAndIDsItems:  []*domain.Testee{makeQueryServiceTestee(31, time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC))},
		countByOrgAndIDsValue: 1,
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
	if repo.listByOrgAndIDsCalls != 1 || repo.countByOrgAndIDsCalls != 1 {
		t.Fatalf("expected restricted list/count to be called once, got list=%d count=%d", repo.listByOrgAndIDsCalls, repo.countByOrgAndIDsCalls)
	}
	if len(repo.lastRestrictedIDs) != 2 || repo.lastRestrictedIDs[0] != 31 || repo.lastRestrictedIDs[1] != 32 {
		t.Fatalf("unexpected restricted ids: %+v", repo.lastRestrictedIDs)
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
		listByOrgItems: []*domain.Testee{makeQueryServiceTestee(41, time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC))},
		countValue:     3,
	}
	service := NewQueryService(repo)

	result, err := service.ListKeyFocus(context.Background(), 1, 0, 10)
	if err != nil {
		t.Fatalf("ListKeyFocus returned error: %v", err)
	}
	if repo.listByOrgCalls != 1 || repo.countCalls != 1 {
		t.Fatalf("expected ListKeyFocus to use unified unrestricted list/count, got list=%d count=%d", repo.listByOrgCalls, repo.countCalls)
	}
	if repo.lastFilter.KeyFocus == nil || *repo.lastFilter.KeyFocus != true {
		t.Fatalf("expected key focus filter=true, got %+v", repo.lastFilter.KeyFocus)
	}
	if result.TotalCount != 3 || len(result.Items) != 1 || result.Items[0].ID != 41 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

type queryServiceRepoStub struct {
	listByOrgItems        []*domain.Testee
	listByOrgAndIDsItems  []*domain.Testee
	countValue            int64
	countByOrgAndIDsValue int64
	listByOrgCalls        int
	listByOrgAndIDsCalls  int
	countCalls            int
	countByOrgAndIDsCalls int
	lastFilter            domain.ListFilter
	lastRestrictedIDs     []domain.ID
}

func (s *queryServiceRepoStub) Save(context.Context, *domain.Testee) error   { return nil }
func (s *queryServiceRepoStub) Update(context.Context, *domain.Testee) error { return nil }
func (s *queryServiceRepoStub) FindByID(context.Context, domain.ID) (*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) FindByIDs(context.Context, []domain.ID) ([]*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) FindByProfile(context.Context, int64, uint64) (*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) FindByOrgAndName(context.Context, int64, string) ([]*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) ListByOrg(_ context.Context, _ int64, filter domain.ListFilter, _ int, _ int) ([]*domain.Testee, error) {
	s.listByOrgCalls++
	s.lastFilter = filter
	return s.listByOrgItems, nil
}
func (s *queryServiceRepoStub) ListByOrgAndIDs(_ context.Context, _ int64, ids []domain.ID, filter domain.ListFilter, _ int, _ int) ([]*domain.Testee, error) {
	s.listByOrgAndIDsCalls++
	s.lastFilter = filter
	s.lastRestrictedIDs = append([]domain.ID(nil), ids...)
	return s.listByOrgAndIDsItems, nil
}
func (s *queryServiceRepoStub) ListByTags(context.Context, int64, []string, int, int) ([]*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) ListKeyFocus(context.Context, int64, int, int) ([]*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) ListByProfileIDs(context.Context, []uint64, int, int) ([]*domain.Testee, error) {
	return nil, nil
}
func (s *queryServiceRepoStub) Delete(context.Context, domain.ID) error { return nil }
func (s *queryServiceRepoStub) Count(_ context.Context, _ int64, filter domain.ListFilter) (int64, error) {
	s.countCalls++
	s.lastFilter = filter
	return s.countValue, nil
}
func (s *queryServiceRepoStub) CountByOrgAndIDs(_ context.Context, _ int64, ids []domain.ID, filter domain.ListFilter) (int64, error) {
	s.countByOrgAndIDsCalls++
	s.lastFilter = filter
	s.lastRestrictedIDs = append([]domain.ID(nil), ids...)
	return s.countByOrgAndIDsValue, nil
}

func makeQueryServiceTestee(id uint64, createdAt time.Time) *domain.Testee {
	item := domain.NewTestee(1, "testee", domain.GenderMale, nil)
	item.SetID(domain.ID(id))
	item.SetCreatedAt(createdAt)
	item.SetUpdatedAt(createdAt)
	return item
}
