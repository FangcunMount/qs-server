package scale

import (
	"bytes"
	"context"
	"testing"
	"time"

	domainscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestScaleListCacheCompressedRoundTrip(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	builder := rediskey.NewBuilderWithNamespace("scale-list-test")
	repo := &scaleListCacheRepo{
		count: 2,
		pages: map[int][]*domainscale.MedicalScale{
			1: {
				newScaleListCacheScale(t, "SCALE_A", "Scale A"),
				newScaleListCacheScale(t, "SCALE_B", "Scale B"),
			},
		},
	}
	cache := NewScaleListCacheWithPolicyAndKeyBuilder(client, repo, nil, builder, cachepolicy.CachePolicy{
		TTL:      time.Minute,
		Compress: cachepolicy.PolicySwitchEnabled,
	})

	if err := cache.Rebuild(ctx); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	raw, err := client.Get(ctx, builder.BuildScaleListKey()).Bytes()
	if err != nil {
		t.Fatalf("redis Get() error = %v", err)
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		t.Fatalf("payload should be gzip-compressed, got prefix %v", raw[:min(2, len(raw))])
	}
	if bytes.Contains(raw, []byte("SCALE_A")) {
		t.Fatal("compressed payload should not contain raw scale code")
	}

	result, ok := cache.GetPage(ctx, 1, 1)
	if !ok {
		t.Fatal("GetPage() hit = false, want true")
	}
	if result.Total != 2 || len(result.Items) != 1 || result.Items[0].Code != "SCALE_A" {
		t.Fatalf("GetPage() = %#v, want first item SCALE_A with total 2", result)
	}
}

func TestScaleListCacheGetPageMissAndRedisErrorFallback(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	cache := NewScaleListCacheWithPolicyAndKeyBuilder(
		client,
		&scaleListCacheRepo{},
		nil,
		rediskey.NewBuilderWithNamespace("scale-list-miss"),
		cachepolicy.CachePolicy{},
	)
	if result, ok := cache.GetPage(ctx, 1, 10); ok || result != nil {
		t.Fatalf("GetPage() on Redis miss = (%#v, %v), want nil,false", result, ok)
	}

	closedClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := closedClient.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	errorCache := NewScaleListCacheWithPolicyAndKeyBuilder(
		closedClient,
		&scaleListCacheRepo{},
		nil,
		rediskey.NewBuilderWithNamespace("scale-list-error"),
		cachepolicy.CachePolicy{},
	)
	if result, ok := errorCache.GetPage(ctx, 1, 10); ok || result != nil {
		t.Fatalf("GetPage() on Redis error = (%#v, %v), want nil,false", result, ok)
	}
}

func TestScaleListCacheRebuildDeletesCacheWhenListEmpty(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	builder := rediskey.NewBuilderWithNamespace("scale-list-empty")
	key := builder.BuildScaleListKey()
	if err := client.Set(ctx, key, []byte("stale"), time.Minute).Err(); err != nil {
		t.Fatalf("redis Set() error = %v", err)
	}

	cache := NewScaleListCacheWithPolicyAndKeyBuilder(
		client,
		&scaleListCacheRepo{count: 0},
		nil,
		builder,
		cachepolicy.CachePolicy{},
	)
	if err := cache.Rebuild(ctx); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
		t.Fatalf("redis Exists() = (%d, %v), want 0,nil", exists, err)
	}
}

func TestScaleListCacheGetPageUsesLocalMemoryAfterRedisHit(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	builder := rediskey.NewBuilderWithNamespace("scale-list-memory")
	cache := NewScaleListCacheWithPolicyAndKeyBuilder(
		client,
		&scaleListCacheRepo{
			count: 1,
			pages: map[int][]*domainscale.MedicalScale{
				1: {newScaleListCacheScale(t, "SCALE_MEMORY", "Memory Scale")},
			},
		},
		nil,
		builder,
		cachepolicy.CachePolicy{TTL: time.Minute},
	)
	if err := cache.Rebuild(ctx); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if _, ok := cache.GetPage(ctx, 1, 10); !ok {
		t.Fatal("first GetPage() hit = false, want true")
	}
	if err := client.Del(ctx, builder.BuildScaleListKey()).Err(); err != nil {
		t.Fatalf("redis Del() error = %v", err)
	}

	result, ok := cache.GetPage(ctx, 1, 10)
	if !ok {
		t.Fatal("second GetPage() should hit local memory after Redis key deletion")
	}
	if len(result.Items) != 1 || result.Items[0].Code != "SCALE_MEMORY" {
		t.Fatalf("GetPage() = %#v, want cached SCALE_MEMORY", result)
	}
}

type scaleListCacheRepo struct {
	count    int64
	countErr error
	pages    map[int][]*domainscale.MedicalScale
	findErr  error
}

func (r *scaleListCacheRepo) Create(context.Context, *domainscale.MedicalScale) error {
	return nil
}

func (r *scaleListCacheRepo) FindByCode(context.Context, string) (*domainscale.MedicalScale, error) {
	return nil, nil
}

func (r *scaleListCacheRepo) FindByQuestionnaireCode(context.Context, string) (*domainscale.MedicalScale, error) {
	return nil, nil
}

func (r *scaleListCacheRepo) FindSummaryList(_ context.Context, page, _ int, _ map[string]interface{}) ([]*domainscale.MedicalScale, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.pages[page], nil
}

func (r *scaleListCacheRepo) CountWithConditions(context.Context, map[string]interface{}) (int64, error) {
	return r.count, r.countErr
}

func (r *scaleListCacheRepo) Update(context.Context, *domainscale.MedicalScale) error {
	return nil
}

func (r *scaleListCacheRepo) Remove(context.Context, string) error {
	return nil
}

func (r *scaleListCacheRepo) ExistsByCode(context.Context, string) (bool, error) {
	return false, nil
}

func newScaleListCacheScale(t *testing.T, code, title string) *domainscale.MedicalScale {
	t.Helper()

	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	scale, err := domainscale.NewMedicalScale(
		meta.NewCode(code),
		title,
		domainscale.WithDescription("description"),
		domainscale.WithQuestionnaire(meta.NewCode("Q_"+code), "v1"),
		domainscale.WithStatus(domainscale.StatusPublished),
		domainscale.WithCategory(domainscale.CategoryADHD),
		domainscale.WithCreatedBy(meta.ID(101)),
		domainscale.WithUpdatedBy(meta.ID(102)),
		domainscale.WithCreatedAt(now),
		domainscale.WithUpdatedAt(now),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return scale
}
