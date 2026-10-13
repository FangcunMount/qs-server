package modelcatalogcache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestPublishedCacheProjectionVersionSurvivesExactLatestQuestionnaireAndList(t *testing.T) {
	for _, version := range []interpretationassets.ProjectionVersion{0, 1, 2, 99} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			model := &port.PublishedModel{Kind: domain.KindScale, Code: "MODEL", Version: "v1", ReportAssetsProjectionVersion: version}
			inner := &publishedModelStoreStub{getByRef: model, findByCode: model, findByQuestionnaire: model, listPublished: []*port.PublishedModel{model}, listPublishedTotal: 1}
			newCache := func() *CachedPublishedModelStore {
				return NewCachedPublishedModelStore(inner, client, keyspace.NewBuilderWithNamespace("test"), publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)
			}
			for i := 0; i < 2; i++ {
				// Second process-shaped adapter has an empty L1 and must decode L2.
				cached := newCache()
				exact, err := cached.GetPublishedModelByRef(context.Background(), port.Ref{Kind: model.Kind, Code: model.Code, Version: model.Version})
				if err != nil || exact.ReportAssetsProjectionVersion != version {
					t.Fatalf("exact: %#v %v", exact, err)
				}
				latest, err := cached.FindPublishedModelByCode(context.Background(), model.Kind, model.Code)
				if err != nil || latest.ReportAssetsProjectionVersion != version {
					t.Fatalf("latest: %#v %v", latest, err)
				}
				byQuestionnaire, err := cached.FindPublishedModelByQuestionnaire(context.Background(), "Q", "v1")
				if err != nil || byQuestionnaire.ReportAssetsProjectionVersion != version {
					t.Fatalf("questionnaire: %#v %v", byQuestionnaire, err)
				}
				list, _, err := cached.ListPublishedModels(context.Background(), port.ListPublishedFilter{})
				if err != nil || len(list) != 1 || list[0].ReportAssetsProjectionVersion != version {
					t.Fatalf("list: %#v %v", list, err)
				}
			}
			if inner.getByRefCalls != 1 || inner.findByCodeCalls != 1 || inner.findByQuestionnaireCalls != 1 || inner.listPublishedCalls != 1 {
				t.Fatalf("L2 failed: %#v", inner)
			}
		})
	}
}

func TestPublishedPayloadSchemaBypassesOldExactAndLatestEntries(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	keys := keyspace.NewBuilderWithNamespace("test")
	stale := &port.PublishedModel{Kind: domain.KindScale, Code: "MODEL", Version: "v1"}
	data, _ := json.Marshal(stale)
	oldRef := keys.BuildPublishedAssessmentModelByRefKey("scale", "", "", "model", "v1")
	oldLatest := keys.BuildPublishedAssessmentModelLatestByCodeKey("scale", "model")
	if err := client.Set(context.Background(), oldRef, data, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(context.Background(), oldLatest, data, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	fresh := *stale
	fresh.ReportAssetsProjectionVersion = 1
	inner := &publishedModelStoreStub{getByRef: &fresh, findByCode: &fresh}
	cached := NewCachedPublishedModelStore(inner, client, keys, publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)
	exact, err := cached.GetPublishedModelByRef(context.Background(), port.Ref{Kind: fresh.Kind, Code: fresh.Code, Version: fresh.Version})
	if err != nil || exact.ReportAssetsProjectionVersion != 1 || inner.getByRefCalls != 1 {
		t.Fatalf("old exact payload reused: %#v %v", exact, err)
	}
	latest, err := cached.FindPublishedModelByCode(context.Background(), fresh.Kind, fresh.Code)
	if err != nil || latest.ReportAssetsProjectionVersion != 1 || inner.findByCodeCalls != 1 {
		t.Fatalf("old latest payload reused: %#v %v", latest, err)
	}
	for _, key := range []string{cached.refCacheKey(port.Ref{}), cached.latestByCodeCacheKey("", ""), cached.questionnaireCacheKey("Q", "v1", 0), cached.listCatalogCacheKeyAtVersion(port.ListPublishedFilter{}, 0)} {
		if !strings.HasSuffix(key, publishedModelPayloadSchemaSuffix) {
			t.Fatalf("payload not versioned: %q", key)
		}
	}
	if !mr.Exists(oldRef) || !mr.Exists(oldLatest) {
		t.Fatal("preparation deleted old-process cache entries")
	}
}
