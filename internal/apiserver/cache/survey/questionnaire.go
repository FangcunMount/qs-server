package surveycache

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	questionnaireInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

type CachedQuestionnaireRepository struct {
	repo     domainQuestionnaire.Repository
	client   redis.UniversalClient
	keys     *keyspace.Builder
	policies sharedcache.PolicyProvider
	observer *observability.ComponentObserver
	store    *adapterkit.ObjectCacheStore[domainQuestionnaire.Questionnaire]
}

func NewCachedQuestionnaireRepositoryWithBuilderAndProvider(repo domainQuestionnaire.Repository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider) domainQuestionnaire.Repository {
	return NewCachedQuestionnaireRepositoryWithBuilderProviderAndObserver(repo, client, builder, policies, nil)
}

func NewCachedQuestionnaireRepositoryWithBuilderProviderAndObserver(repo domainQuestionnaire.Repository, client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider, observer *observability.ComponentObserver) domainQuestionnaire.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	mapper := questionnaireInfra.NewQuestionnaireMapper()
	return &CachedQuestionnaireRepository{
		repo:     repo,
		client:   client,
		keys:     builder,
		policies: policies,
		observer: observer,
		store: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[domainQuestionnaire.Questionnaire]{
			Cache:     adapterkit.NewRedisStoreIfAvailable(client),
			PolicyKey: cachepolicy.CapabilitySurveyQuestionnaire,
			Codec:     newQuestionnaireCacheEntryCodec(mapper),
		}),
	}
}

func newQuestionnaireCacheEntryCodec(mapper *questionnaireInfra.QuestionnaireMapper) adapterkit.CacheEntryCodec[domainQuestionnaire.Questionnaire] {
	return adapterkit.CacheEntryCodec[domainQuestionnaire.Questionnaire]{
		EncodeFunc: func(domain *domainQuestionnaire.Questionnaire) ([]byte, error) {
			return json.Marshal(mapper.ToPO(domain))
		},
		DecodeFunc: func(data []byte) (*domainQuestionnaire.Questionnaire, error) {
			var po questionnaireInfra.QuestionnairePO
			if err := json.Unmarshal(data, &po); err != nil {
				return nil, err
			}
			return mapper.ToBO(&po), nil
		},
	}
}

func (r *CachedQuestionnaireRepository) Create(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	if err := r.repo.Create(ctx, qDomain); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.setCache(ctx, r.headKey(qDomain.GetCode().Value()), qDomain)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) CreatePublishedSnapshot(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire, active bool) error {
	if err := r.repo.CreatePublishedSnapshot(ctx, qDomain, active); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.setCache(ctx, r.versionKey(qDomain.GetCode().Value(), qDomain.GetVersion().Value()), qDomain)
		if active {
			_ = r.setCache(ctx, r.publishedKey(qDomain.GetCode().Value()), qDomain)
		}
	}
	return nil
}

func (r *CachedQuestionnaireRepository) FindByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return r.loadWithCache(ctx, r.headKey(code), func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
		return r.repo.FindByCode(ctx, code)
	})
}

func (r *CachedQuestionnaireRepository) FindPublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return r.loadWithCache(ctx, r.publishedKey(code), func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
		return r.repo.FindPublishedByCode(ctx, code)
	})
}

func (r *CachedQuestionnaireRepository) FindLatestPublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return r.repo.FindLatestPublishedByCode(ctx, code)
}

func (r *CachedQuestionnaireRepository) FindByCodeVersion(ctx context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	if version == "" {
		return nil, nil
	}
	key := r.versionKey(code, version)
	return r.loadWithCache(ctx, key, func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
		return r.repo.FindByCodeVersion(ctx, code, version)
	})
}

func (r *CachedQuestionnaireRepository) FindBaseByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return r.repo.FindBaseByCode(ctx, code)
}

func (r *CachedQuestionnaireRepository) FindBasePublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return r.repo.FindBasePublishedByCode(ctx, code)
}

func (r *CachedQuestionnaireRepository) FindBaseByCodeVersion(ctx context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	return r.repo.FindBaseByCodeVersion(ctx, code, version)
}

func (r *CachedQuestionnaireRepository) LoadQuestions(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	return r.repo.LoadQuestions(ctx, qDomain)
}

func (r *CachedQuestionnaireRepository) Update(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	if err := r.repo.Update(ctx, qDomain); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.deleteCacheByCode(ctx, qDomain.GetCode().Value())
	}
	return nil
}

func (r *CachedQuestionnaireRepository) SetActivePublishedVersion(ctx context.Context, code, version string) error {
	if err := r.repo.SetActivePublishedVersion(ctx, code, version); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) ClearActivePublishedVersion(ctx context.Context, code string) error {
	if err := r.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) Remove(ctx context.Context, code string) error {
	if err := r.repo.Remove(ctx, code); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) HardDelete(ctx context.Context, code string) error {
	if err := r.repo.HardDelete(ctx, code); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) HardDeleteFamily(ctx context.Context, code string) error {
	if err := r.repo.HardDeleteFamily(ctx, code); err != nil {
		return err
	}
	if r.shouldMutateCache(ctx) {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.repo.ExistsByCode(ctx, code)
}

func (r *CachedQuestionnaireRepository) HasPublishedSnapshots(ctx context.Context, code string) (bool, error) {
	return r.repo.HasPublishedSnapshots(ctx, code)
}

func (r *CachedQuestionnaireRepository) headKey(code string) string {
	return r.keys.BuildQuestionnaireKey(strings.ToLower(code), "")
}

func (r *CachedQuestionnaireRepository) publishedKey(code string) string {
	return r.keys.BuildPublishedQuestionnaireKey(strings.ToLower(code))
}

func (r *CachedQuestionnaireRepository) versionKey(code, version string) string {
	return r.keys.BuildQuestionnaireKey(strings.ToLower(code), version)
}

func (r *CachedQuestionnaireRepository) setCache(ctx context.Context, key string, qDomain *domainQuestionnaire.Questionnaire) error {
	policy := sharedcache.Policy{}
	if r.policies != nil {
		if effective, ok := r.policies.Resolve(cachepolicy.CapabilitySurveyQuestionnaire); ok {
			policy = effective.Policy
		}
	}
	return r.store.Set(ctx, key, qDomain, policy)
}

func (r *CachedQuestionnaireRepository) loadWithCache(
	ctx context.Context,
	key string,
	fallback func(context.Context) (*domainQuestionnaire.Questionnaire, error),
) (*domainQuestionnaire.Questionnaire, error) {
	return adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[domainQuestionnaire.Questionnaire]{
		PolicyKey:      cachepolicy.CapabilitySurveyQuestionnaire,
		CacheKey:       key,
		PolicyProvider: r.policies,
		Observer:       r.observer,
		Store:          r.store,
		Load:           fallback,
		CacheNegative:  true,
		AsyncSetCached: true,
	})
}

func (r *CachedQuestionnaireRepository) deleteCacheByCode(ctx context.Context, code string) error {
	patterns := []string{
		r.headKey(code),
		r.publishedKey(code),
		r.keys.BuildQuestionnaireKey(strings.ToLower(code), "*"),
	}

	for _, pattern := range patterns[:2] {
		if err := r.store.Delete(ctx, pattern); err != nil {
			logger.L(ctx).Warnw("failed to delete questionnaire cache key", "key", pattern, "error", err)
		}
	}

	iter := r.client.Scan(ctx, 0, patterns[2], 100).Iterator()
	for iter.Next(ctx) {
		_ = r.store.Delete(ctx, iter.Val())
	}
	return iter.Err()
}

func (r *CachedQuestionnaireRepository) shouldMutateCache(ctx context.Context) bool {
	return r != nil && r.client != nil && mongo.SessionFromContext(ctx) == nil
}

// InvalidateQuestionnaireCache is called after a paired release transaction
// commits. Exact-version entries are removed too because legacy cache entries
// may contain mutable release metadata.
func (r *CachedQuestionnaireRepository) InvalidateQuestionnaireCache(ctx context.Context, code string) {
	if r == nil || r.client == nil || code == "" {
		return
	}
	_ = r.deleteCacheByCode(ctx, code)
}

// WarmupCache 预热工作版本和当前已发布版本缓存
func (r *CachedQuestionnaireRepository) WarmupCache(ctx context.Context, codes []string) error {
	if !r.store.Available() {
		return nil
	}

	for _, code := range codes {
		headExists, _ := r.store.Exists(ctx, r.headKey(code))
		if !headExists {
			if qDomain, err := r.repo.FindByCode(ctx, code); err == nil && qDomain != nil {
				_ = r.setCache(ctx, r.headKey(code), qDomain)
			}
		}
		publishedExists, _ := r.store.Exists(ctx, r.publishedKey(code))
		if !publishedExists {
			if qDomain, err := r.repo.FindPublishedByCode(ctx, code); err == nil && qDomain != nil {
				_ = r.setCache(ctx, r.publishedKey(code), qDomain)
			}
		}
	}

	return nil
}
