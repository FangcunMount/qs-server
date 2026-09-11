package actorcache

import (
	"context"
	"fmt"
	testeeInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestCachedTesteeRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedTesteeRepositoryWithBuilderProviderAndObserver(nil, nil, keyspace.NewBuilderWithNamespace("prod:cache:object"), testeePolicyProvider(sharedcache.Policy{}), nil)
	cached, ok := repo.(*CachedTesteeRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(testee.ID(meta.MustFromUint64(7)))
	if got != "prod:cache:object:testee:info:7" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}

func TestCachedTesteeRepositoryCachesNegativeResult(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	repo := &testeeNegativeRepo{}
	cached := NewCachedTesteeRepositoryWithBuilderProviderAndObserver(
		repo,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		testeePolicyProvider(cachepolicy.CachePolicy{
			Negative:    cachepolicy.PolicySwitchEnabled,
			NegativeTTL: time.Minute,
		}),
		nil,
	).(*CachedTesteeRepository)

	ctx := context.Background()
	id := testee.ID(meta.MustFromUint64(99))
	got, err := cached.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got != nil {
		t.Fatalf("FindByID() value = %#v, want nil", got)
	}

	key := cached.buildCacheKey(id)
	waitFor(t, func() bool {
		return hasRedisKey(t, client, key)
	})

	got, err = cached.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() cached negative error = %v", err)
	}
	if got != nil {
		t.Fatalf("FindByID() cached negative value = %#v, want nil", got)
	}
	if repo.findByIDCalls != 1 {
		t.Fatalf("FindByID() repo calls = %d, want 1", repo.findByIDCalls)
	}
}

func testeePolicyProvider(policy sharedcache.Policy) sharedcache.PolicyProvider {
	return sharedcache.NewRegistry(sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityActorTestee, Policy: policy})
}

type testeeNegativeRepo struct {
	testee.Repository
	findByIDCalls int
}

func (r *testeeNegativeRepo) FindByID(context.Context, testee.ID) (*testee.Testee, error) {
	r.findByIDCalls++
	return nil, nil
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func hasRedisKey(t *testing.T, client redis.UniversalClient, key string) bool {
	t.Helper()
	return client.Exists(context.Background(), key).Val() > 0
}

func TestTesteeTransactionReadsBypassCachedNegative(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	repo := &testeeNegativeRepo{}
	cached := NewCachedTesteeRepositoryWithBuilderProviderAndObserver(repo, client, keyspace.NewBuilderWithNamespace("tx-test"), testeePolicyProvider(cachepolicy.CachePolicy{Negative: cachepolicy.PolicySwitchEnabled, NegativeTTL: time.Minute}), nil).(*CachedTesteeRepository)
	id := testee.ID(99)
	if _, err := cached.FindByID(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return hasRedisKey(t, client, cached.buildCacheKey(id)) })
	ctx := dbctx.WithTx(context.Background(), &gorm.DB{})
	if _, err := cached.FindByID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if repo.findByIDCalls != 2 {
		t.Fatal("transaction trusted a cached absence instead of current database facts")
	}
}

func TestTesteeCacheRejectsPayloadWithoutOwnershipVersion(t *testing.T) {
	codec := newTesteeCacheEntryCodec(testeeInfra.NewTesteeMapper())
	if _, err := codec.DecodeFunc([]byte(`{"OrgID":7,"Name":"legacy"}`)); err == nil {
		t.Fatal("legacy cache treated as current unassigned state")
	}
	subject := testee.NewTestee(7, "current", testee.Gender(0), nil)
	id := uint64(2)
	subject.RestoreStore(&id, 3)
	data, err := codec.EncodeFunc(subject)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := codec.DecodeFunc(data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.StoreID() == nil || *restored.StoreID() != 2 || restored.StoreVersion() != 3 {
		t.Fatal("current cache lost ownership")
	}
}

type ownershipRepo struct {
	testee.Repository
	current testee.Ownership
	err     error
	reads   int
}

func (r *ownershipRepo) FindByID(context.Context, testee.ID) (*testee.Testee, error) {
	panic("cached profile should not be reloaded")
}
func (r *ownershipRepo) FindCurrentOwnership(context.Context, testee.ID) (testee.Ownership, error) {
	r.reads++
	return r.current, r.err
}
func TestCachedTesteeOwnershipUsesCurrentFactsAndFailsClosed(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	oldStore, newStore := uint64(1), uint64(2)
	repo := &ownershipRepo{current: testee.Ownership{OrgID: 7, StoreID: &newStore, Version: 3}}
	policy := sharedcache.Policy{TTL: time.Minute}
	cached := NewCachedTesteeRepositoryWithBuilderProviderAndObserver(repo, client, keyspace.NewBuilderWithNamespace("ownership"), testeePolicyProvider(policy), nil).(*CachedTesteeRepository)
	old := testee.NewTestee(7, "cached", testee.Gender(0), nil)
	old.RestoreStore(&oldStore, 2)
	ctx, id := context.Background(), testee.ID(10)
	if err := cached.store.Set(ctx, cached.buildCacheKey(id), old, policy); err != nil {
		t.Fatal(err)
	}
	got, err := cached.FindByID(ctx, id)
	if err != nil || got.StoreID() == nil || *got.StoreID() != 2 || got.StoreVersion() != 3 || repo.reads != 1 {
		t.Fatalf("stale ownership: %+v %v", got, err)
	}
	// A compensating migration restores unassigned with a higher version.
	repo.current.StoreID, repo.current.Version = nil, 4
	got, err = cached.FindByID(ctx, id)
	if err != nil || got.StoreID() != nil || got.StoreVersion() != 4 {
		t.Fatalf("rollback not visible: %v", err)
	}
	repo.err = fmt.Errorf("database unavailable")
	if got, err = cached.FindByID(ctx, id); err == nil || got != nil {
		t.Fatal("cached ownership bypassed storage failure")
	}
	repo.err = nil
	repo.current.Version = 0
	if got, err = cached.FindByID(ctx, id); err == nil || got != nil {
		t.Fatal("invalid ownership version accepted")
	}
	repo.current.Version = 4
	repo.current.OrgID = 8
	if got, err = cached.FindByID(ctx, id); err == nil || got != nil {
		t.Fatal("mismatched company accepted")
	}
}
