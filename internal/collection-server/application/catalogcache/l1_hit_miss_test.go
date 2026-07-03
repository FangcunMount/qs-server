package catalogcache

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
)

func TestScaleL1HitMissCycle(t *testing.T) {
	t.Parallel()

	cache := scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{
		TTL:        time.Minute,
		MaxEntries: 16,
	})
	svc := scale.NewQueryService(nil, cache, false)

	if svc.HasCachedDetail("demo") {
		t.Fatal("expected miss before set")
	}
	cache.SetDetail("demo", &scale.ScaleResponse{Code: "demo", Title: "cached"})
	if !svc.HasCachedDetail("demo") {
		t.Fatal("expected hit after set")
	}
	_ = context.Background()
}

func TestQuestionnaireL1HitMissSmoke(t *testing.T) {
	t.Parallel()

	cache := questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.Set("q1", "1.0", &questionnaire.QuestionnaireResponse{Code: "q1", Version: "1.0"})
	if _, ok := cache.Get("q1", "1.0"); !ok {
		t.Fatal("expected questionnaire L1 hit")
	}
}
