package cachequery

import (
	"bytes"
	"context"
	"testing"
	"time"

	domainscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestPublishedScaleListCacheCompressedRoundTrip(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	builder := keyspace.NewBuilderWithNamespace("scale-list-test")
	repo := &scaleListCacheRepo{
		count: 2,
		pages: map[int][]scalereadmodel.ScaleSummaryRow{
			1: {
				newScaleListCacheScale(t, "SCALE_A", "Scale A"),
				newScaleListCacheScale(t, "SCALE_B", "Scale B"),
			},
		},
	}
	cache := NewPublishedScaleListCacheWithPolicyAndKeyBuilder(cacheentry.NewRedisCache(client), repo, nil, builder, cachepolicy.CachePolicy{
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

func TestPublishedScaleListCacheGetPageMissAndRedisErrorFallback(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	cache := NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
		cacheentry.NewRedisCache(client),
		&scaleListCacheRepo{},
		nil,
		keyspace.NewBuilderWithNamespace("scale-list-miss"),
		cachepolicy.CachePolicy{},
	)
	if result, ok := cache.GetPage(ctx, 1, 10); ok || result != nil {
		t.Fatalf("GetPage() on Redis miss = (%#v, %v), want nil,false", result, ok)
	}

	closedClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := closedClient.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	errorCache := NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
		cacheentry.NewRedisCache(closedClient),
		&scaleListCacheRepo{},
		nil,
		keyspace.NewBuilderWithNamespace("scale-list-error"),
		cachepolicy.CachePolicy{},
	)
	if result, ok := errorCache.GetPage(ctx, 1, 10); ok || result != nil {
		t.Fatalf("GetPage() on Redis error = (%#v, %v), want nil,false", result, ok)
	}
}

func TestPublishedScaleListCacheRebuildDeletesCacheWhenListEmpty(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	builder := keyspace.NewBuilderWithNamespace("scale-list-empty")
	key := builder.BuildScaleListKey()
	if err := client.Set(ctx, key, []byte("stale"), time.Minute).Err(); err != nil {
		t.Fatalf("redis Set() error = %v", err)
	}

	cache := NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
		cacheentry.NewRedisCache(client),
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

func TestPublishedScaleListCacheGetPageUsesLocalMemoryAfterRedisHit(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	builder := keyspace.NewBuilderWithNamespace("scale-list-memory")
	cache := NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
		cacheentry.NewRedisCache(client),
		&scaleListCacheRepo{
			count: 1,
			pages: map[int][]scalereadmodel.ScaleSummaryRow{
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
	pages    map[int][]scalereadmodel.ScaleSummaryRow
	findErr  error
}

func (r *scaleListCacheRepo) ListScales(_ context.Context, _ scalereadmodel.ScaleFilter, page scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.pages[page.Page], nil
}

func (r *scaleListCacheRepo) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	return r.count, r.countErr
}

func newScaleListCacheScale(t *testing.T, code, title string) scalereadmodel.ScaleSummaryRow {
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
	return scalereadmodel.ScaleSummaryRow{
		Code:              scale.GetCode().String(),
		Title:             scale.GetTitle(),
		Description:       scale.GetDescription(),
		Category:          scale.GetCategory().String(),
		Stages:            []string{domainscale.StageDeepAssessment.String()},
		ApplicableAges:    []string{domainscale.ApplicableAgeSchoolChild.String()},
		Reporters:         []string{domainscale.ReporterParent.String()},
		Tags:              []string{"tag"},
		QuestionnaireCode: scale.GetQuestionnaireCode().String(),
		Status:            scale.GetStatus().String(),
		CreatedBy:         scale.GetCreatedBy(),
		CreatedAt:         scale.GetCreatedAt(),
		UpdatedBy:         scale.GetUpdatedBy(),
		UpdatedAt:         scale.GetUpdatedAt(),
	}
}
