package evaluationcache

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
)

func TestCachedAssessmentRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedAssessmentRepositoryWithBuilderAndPolicy(nil, nil, keyspace.NewBuilderWithNamespace("prod:cache:object"), cachepolicy.CachePolicy{})
	cached, ok := repo.(*CachedAssessmentRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(assessment.ID(meta.MustFromUint64(42)))
	if got != "prod:cache:object:assessment:detail:42" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}
