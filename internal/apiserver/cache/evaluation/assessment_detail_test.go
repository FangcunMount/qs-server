package evaluationcache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type deletingAssessmentRepo struct{ assessment.Repository }

func (deletingAssessmentRepo) Delete(context.Context, assessment.ID) error { return nil }

func TestAssessmentInvalidationWaitsForOriginalRootCommit(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "commit-unknown"} {
		t.Run(outcome, func(t *testing.T) {
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			keys := keyspace.NewBuilderWithNamespace("cache:object")
			caches := NewAssessmentCaches(client, keys, nil, nil)
			cacheKeys := []string{keys.BuildAssessmentAccessKey(42), keys.BuildAssessmentOutcomeDetailKey(42)}
			for _, key := range cacheKeys {
				require.NoError(t, mr.Set(key, "original-cache-entry"))
			}
			pool, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() {
				mock.ExpectClose()
				require.NoError(t, pool.Close())
				require.NoError(t, mock.ExpectationsWereMet())
			})
			db, err := gorm.Open(driver.New(driver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			mock.ExpectBegin()
			switch outcome {
			case "commit":
				mock.ExpectCommit()
			case "rollback":
				mock.ExpectRollback()
			case "commit-unknown":
				mock.ExpectCommit().WillReturnError(errors.New("commit reply lost"))
			}
			repo := NewInvalidatingAssessmentRepository(deletingAssessmentRepo{}, caches)
			err = hostmysql.NewUnitOfWork(db).WithinTransaction(t.Context(), func(ctx context.Context) error {
				require.NoError(t, repo.Delete(ctx, assessment.NewID(42)))
				for _, key := range cacheKeys {
					require.True(t, mr.Exists(key), "Redis invalidation ran before root commit")
				}
				if outcome == "rollback" {
					return errors.New("business write rejected")
				}
				return nil
			})
			if outcome == "commit" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			for _, key := range cacheKeys {
				require.Equal(t, outcome != "commit", mr.Exists(key), "unconfirmed commit must not run hooks")
			}
		})
	}
}

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
