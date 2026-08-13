package evaluationcache

import (
	"context"
	"testing"
	"time"

	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type deletingAssessmentRepo struct{ assessment.Repository }

func (deletingAssessmentRepo) Delete(context.Context, assessment.ID) error { return nil }

func TestAssessmentCachesUseIndependentVersionedKeys(t *testing.T) {
	builder := keyspace.NewBuilderWithNamespace("prod:cache:object")
	if got := builder.BuildAssessmentAccessKey(42); got != "prod:cache:object:assessment:access:v1:42" {
		t.Fatalf("access key = %q", got)
	}
	if got := builder.BuildAssessmentOutcomeDetailKey(42); got != "prod:cache:object:assessment:outcome:v1:42" {
		t.Fatalf("detail key = %q", got)
	}
}

func TestAssessmentCachesStoreOwnerAndOnlyEvaluatedDetail(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close redis client: %v", err)
		}
	})
	policies := sharedcache.NewRegistry(
		sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityEvaluationAssessmentAccess, Policy: sharedcache.Policy{TTL: time.Minute}},
		sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityEvaluationAssessmentDetail, Policy: sharedcache.Policy{TTL: time.Minute}},
	)
	caches := NewAssessmentCaches(client, keyspace.NewBuilderWithNamespace("cache:object"), policies, nil)
	ctx := context.Background()

	ownerLoads := 0
	loadOwner := func(context.Context) (uint64, error) { ownerLoads++; return 7, nil }
	for range 2 {
		owner, err := caches.ReadOwner(ctx, 42, loadOwner)
		if err != nil || owner != 7 {
			t.Fatalf("owner = %d, err=%v", owner, err)
		}
	}
	if ownerLoads != 1 {
		t.Fatalf("owner loads = %d, want 1", ownerLoads)
	}

	mutableLoads := 0
	loadMutable := func(context.Context) (*evaluationtestee.Assessment, error) {
		mutableLoads++
		return &evaluationtestee.Assessment{ID: 42, Status: "submitted"}, nil
	}
	for range 2 {
		if _, err := caches.ReadDetail(ctx, 42, loadMutable); err != nil {
			t.Fatal(err)
		}
	}
	if mutableLoads != 2 {
		t.Fatalf("mutable loads = %d, want 2", mutableLoads)
	}
	evaluatedLoads := 0
	loadEvaluated := func(context.Context) (*evaluationtestee.Assessment, error) {
		evaluatedLoads++
		return &evaluationtestee.Assessment{ID: 43, TesteeID: 7, Status: "evaluated"}, nil
	}
	for range 2 {
		detail, err := caches.ReadDetail(ctx, 43, loadEvaluated)
		if err != nil || detail.TesteeID != 7 {
			t.Fatalf("evaluated detail=%#v err=%v", detail, err)
		}
	}
	if evaluatedLoads != 1 {
		t.Fatalf("evaluated loads = %d, want 1", evaluatedLoads)
	}

	caches.Evict(ctx, 42)
	caches.Evict(ctx, 43)
	if exists, _ := redisstore.NewStore(client).Exists(ctx, "cache:object:assessment:access:v1:42"); exists {
		t.Fatal("access cache remained after eviction")
	}
	if exists, _ := redisstore.NewStore(client).Exists(ctx, "cache:object:assessment:outcome:v1:43"); exists {
		t.Fatal("detail cache remained after eviction")
	}
}

func TestAssessmentDeleteEvictsAccessAndDetailL2(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close redis client: %v", err)
		}
	})
	policies := sharedcache.NewRegistry(
		sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityEvaluationAssessmentAccess, Policy: sharedcache.Policy{TTL: time.Minute}},
		sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityEvaluationAssessmentDetail, Policy: sharedcache.Policy{TTL: time.Minute}},
	)
	caches := NewAssessmentCaches(client, keyspace.NewBuilderWithNamespace("cache:object"), policies, nil)
	ctx := context.Background()
	if _, err := caches.ReadOwner(ctx, 42, func(context.Context) (uint64, error) { return 7, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := caches.ReadDetail(ctx, 42, func(context.Context) (*evaluationtestee.Assessment, error) {
		return &evaluationtestee.Assessment{ID: 42, TesteeID: 7, Status: "evaluated"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	repo := NewInvalidatingAssessmentRepository(deletingAssessmentRepo{}, caches)
	if err := repo.Delete(ctx, assessment.NewID(42)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cache:object:assessment:access:v1:42", "cache:object:assessment:outcome:v1:42"} {
		if exists, _ := redisstore.NewStore(client).Exists(ctx, key); exists {
			t.Fatalf("cache key %q remained after delete", key)
		}
	}
}
