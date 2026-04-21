package cache

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

func TestCachedAssessmentRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedAssessmentRepositoryWithBuilderAndPolicy(nil, nil, rediskey.NewBuilderWithNamespace("prod:cache:object"), cachepolicy.CachePolicy{})
	cached, ok := repo.(*CachedAssessmentRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(assessment.ID(meta.MustFromUint64(42)))
	if got != "prod:cache:object:assessment:detail:42" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}
