package cache

import (
	"context"
	"testing"
	"time"

	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestCachedTesteeRepositoryDefaultsToShorterTTL(t *testing.T) {
	repo := NewCachedTesteeRepository(&testeeRepoStub{}, nil).(*CachedTesteeRepository)
	if repo.ttl != 30*time.Minute {
		t.Fatalf("expected default testee cache ttl to be 30m, got %s", repo.ttl)
	}
}

func TestCachedTesteeRepositoryCachesSingleReadsButBypassesBatchCaching(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	backingRepo := &testeeRepoStub{
		byID: map[domainTestee.ID]*domainTestee.Testee{
			1: makeCachedTestee(1),
			2: makeCachedTestee(2),
		},
	}
	repo := NewCachedTesteeRepository(backingRepo, client).(*CachedTesteeRepository)
	ctx := context.Background()

	item, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("find by id failed: %v", err)
	}
	if item == nil || item.ID() != 1 {
		t.Fatalf("expected cached testee 1, got %#v", item)
	}

	if err := waitForMiniredisKey(mr, repo.buildCacheKey(1)); err != nil {
		t.Fatalf("expected single read to populate redis cache: %v", err)
	}

	if _, err := repo.FindByIDs(ctx, []domainTestee.ID{2}); err != nil {
		t.Fatalf("find by ids failed: %v", err)
	}
	if mr.Exists(repo.buildCacheKey(2)) {
		t.Fatalf("expected batch read not to populate redis cache")
	}
	if backingRepo.findByIDCalls != 1 {
		t.Fatalf("expected one backing FindByID call, got %d", backingRepo.findByIDCalls)
	}
	if backingRepo.findByIDsCalls != 1 {
		t.Fatalf("expected one backing FindByIDs call, got %d", backingRepo.findByIDsCalls)
	}
}

type testeeRepoStub struct {
	byID           map[domainTestee.ID]*domainTestee.Testee
	findByIDCalls  int
	findByIDsCalls int
}

func (s *testeeRepoStub) Save(ctx context.Context, testee *domainTestee.Testee) error   { return nil }
func (s *testeeRepoStub) Update(ctx context.Context, testee *domainTestee.Testee) error { return nil }
func (s *testeeRepoStub) FindByID(ctx context.Context, id domainTestee.ID) (*domainTestee.Testee, error) {
	s.findByIDCalls++
	return s.byID[id], nil
}
func (s *testeeRepoStub) FindByIDs(ctx context.Context, ids []domainTestee.ID) ([]*domainTestee.Testee, error) {
	s.findByIDsCalls++
	items := make([]*domainTestee.Testee, 0, len(ids))
	for _, id := range ids {
		if item := s.byID[id]; item != nil {
			items = append(items, item)
		}
	}
	return items, nil
}
func (s *testeeRepoStub) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) ListByOrgAndIDs(ctx context.Context, orgID int64, ids []domainTestee.ID, filter domainTestee.ListFilter, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]*domainTestee.Testee, error) {
	return nil, nil
}
func (s *testeeRepoStub) Delete(ctx context.Context, id domainTestee.ID) error  { return nil }
func (s *testeeRepoStub) Count(ctx context.Context, orgID int64) (int64, error) { return 0, nil }
func (s *testeeRepoStub) CountByOrgAndIDs(ctx context.Context, orgID int64, ids []domainTestee.ID, filter domainTestee.ListFilter) (int64, error) {
	return 0, nil
}

func makeCachedTestee(id uint64) *domainTestee.Testee {
	item := domainTestee.NewTestee(1, "testee", domainTestee.GenderMale, nil)
	item.SetID(domainTestee.ID(id))
	return item
}

func waitForMiniredisKey(mr *miniredis.Miniredis, key string) error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mr.Exists(key) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
