package cache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	questionnaireInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultQuestionnaireCacheTTL         = 12 * time.Hour
	defaultNegativeQuestionnaireCacheTTL = 5 * time.Minute
)

type CachedQuestionnaireRepository struct {
	repo     domainQuestionnaire.Repository
	client   redis.UniversalClient
	ttl      time.Duration
	mapper   *questionnaireInfra.QuestionnaireMapper
	keys     *rediskey.Builder
	policy   cachepolicy.CachePolicy
	observer *Observer
}

func NewCachedQuestionnaireRepositoryWithBuilderAndPolicy(repo domainQuestionnaire.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) domainQuestionnaire.Repository {
	return NewCachedQuestionnaireRepositoryWithBuilderPolicyAndObserver(repo, client, builder, policy, nil)
}

func NewCachedQuestionnaireRepositoryWithBuilderPolicyAndObserver(repo domainQuestionnaire.Repository, client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy, observer *Observer) domainQuestionnaire.Repository {
	if builder == nil {
		panic("redis builder is required")
	}
	return &CachedQuestionnaireRepository{
		repo:     repo,
		client:   client,
		ttl:      policy.TTLOr(defaultQuestionnaireCacheTTL),
		mapper:   questionnaireInfra.NewQuestionnaireMapper(),
		keys:     builder,
		policy:   policy,
		observer: observer,
	}
}

func (r *CachedQuestionnaireRepository) WithTTL(ttl time.Duration) *CachedQuestionnaireRepository {
	r.ttl = ttl
	return r
}

func (r *CachedQuestionnaireRepository) Create(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	if err := r.repo.Create(ctx, qDomain); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.setCache(ctx, r.headKey(qDomain.GetCode().Value()), qDomain, r.ttl)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) CreatePublishedSnapshot(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire, active bool) error {
	if err := r.repo.CreatePublishedSnapshot(ctx, qDomain, active); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.setCache(ctx, r.versionKey(qDomain.GetCode().Value(), qDomain.GetVersion().Value()), qDomain, r.ttl)
		if active {
			_ = r.setCache(ctx, r.publishedKey(qDomain.GetCode().Value()), qDomain, r.ttl)
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

func (r *CachedQuestionnaireRepository) FindBaseList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return r.repo.FindBaseList(ctx, page, pageSize, conditions)
}

func (r *CachedQuestionnaireRepository) FindBasePublishedList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return r.repo.FindBasePublishedList(ctx, page, pageSize, conditions)
}

func (r *CachedQuestionnaireRepository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	return r.repo.CountWithConditions(ctx, conditions)
}

func (r *CachedQuestionnaireRepository) CountPublishedWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	return r.repo.CountPublishedWithConditions(ctx, conditions)
}

func (r *CachedQuestionnaireRepository) Update(ctx context.Context, qDomain *domainQuestionnaire.Questionnaire) error {
	if err := r.repo.Update(ctx, qDomain); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.deleteCacheByCode(ctx, qDomain.GetCode().Value())
	}
	return nil
}

func (r *CachedQuestionnaireRepository) SetActivePublishedVersion(ctx context.Context, code, version string) error {
	if err := r.repo.SetActivePublishedVersion(ctx, code, version); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) ClearActivePublishedVersion(ctx context.Context, code string) error {
	if err := r.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) Remove(ctx context.Context, code string) error {
	if err := r.repo.Remove(ctx, code); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) HardDelete(ctx context.Context, code string) error {
	if err := r.repo.HardDelete(ctx, code); err != nil {
		return err
	}
	if r.client != nil {
		_ = r.deleteCacheByCode(ctx, code)
	}
	return nil
}

func (r *CachedQuestionnaireRepository) HardDeleteFamily(ctx context.Context, code string) error {
	if err := r.repo.HardDeleteFamily(ctx, code); err != nil {
		return err
	}
	if r.client != nil {
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

func (r *CachedQuestionnaireRepository) getCache(ctx context.Context, key string) (*domainQuestionnaire.Questionnaire, error) {
	if r.client == nil {
		return nil, ErrCacheNotFound
	}
	result := r.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return nil, ErrCacheNotFound
	}
	if result.Err() != nil {
		return nil, result.Err()
	}

	dataBytes, err := result.Bytes()
	if err != nil {
		return nil, err
	}
	if len(dataBytes) == 0 {
		return nil, nil
	}

	data := r.policy.DecompressValue(dataBytes)
	observePayload(cachepolicy.PolicyQuestionnaire, len(data), len(dataBytes))
	var po questionnaireInfra.QuestionnairePO
	if err := json.Unmarshal(data, &po); err != nil {
		logger.L(ctx).Warnw("failed to unmarshal cached questionnaire", "key", key, "error", err)
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}

func (r *CachedQuestionnaireRepository) setCache(ctx context.Context, key string, qDomain *domainQuestionnaire.Questionnaire, ttl time.Duration) error {
	if r.client == nil {
		return nil
	}
	po := r.mapper.ToPO(qDomain)
	data, err := json.Marshal(po)
	if err != nil {
		return err
	}
	payload := r.policy.CompressValue(data)
	observePayload(cachepolicy.PolicyQuestionnaire, len(data), len(payload))
	return r.client.Set(ctx, key, payload, r.policy.JitterTTL(ttl)).Err()
}

func (r *CachedQuestionnaireRepository) loadWithCache(
	ctx context.Context,
	key string,
	fallback func(context.Context) (*domainQuestionnaire.Questionnaire, error),
) (*domainQuestionnaire.Questionnaire, error) {
	return ReadThrough(ctx, ReadThroughOptions[domainQuestionnaire.Questionnaire]{
		PolicyKey: cachepolicy.PolicyQuestionnaire,
		CacheKey:  key,
		Policy:    r.policy,
		Observer:  r.observer,
		GetCached: func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
			return r.getCache(ctx, key)
		},
		Load: fallback,
		SetCached: func(ctx context.Context, value *domainQuestionnaire.Questionnaire) error {
			return r.setCache(ctx, key, value, r.ttl)
		},
		SetNegativeCached: func(ctx context.Context) error {
			if r.client == nil {
				return nil
			}
			return r.client.Set(ctx, key, []byte{}, r.policy.JitterTTL(r.policy.NegativeTTLOr(defaultNegativeQuestionnaireCacheTTL))).Err()
		},
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
		if err := r.client.Del(ctx, pattern).Err(); err != nil {
			observeInvalidate(cachepolicy.PolicyQuestionnaire, "error")
			logger.L(ctx).Warnw("failed to delete questionnaire cache key", "key", pattern, "error", err)
		} else {
			observeInvalidate(cachepolicy.PolicyQuestionnaire, "ok")
		}
	}

	iter := r.client.Scan(ctx, 0, patterns[2], 100).Iterator()
	for iter.Next(ctx) {
		if err := r.client.Del(ctx, iter.Val()).Err(); err != nil {
			observeInvalidate(cachepolicy.PolicyQuestionnaire, "error")
		} else {
			observeInvalidate(cachepolicy.PolicyQuestionnaire, "ok")
		}
	}
	return iter.Err()
}

// WarmupCache 预热工作版本和当前已发布版本缓存
func (r *CachedQuestionnaireRepository) WarmupCache(ctx context.Context, codes []string) error {
	if r.client == nil {
		return nil
	}

	for _, code := range codes {
		if r.client.Exists(ctx, r.headKey(code)).Val() == 0 {
			if qDomain, err := r.repo.FindByCode(ctx, code); err == nil && qDomain != nil {
				_ = r.setCache(ctx, r.headKey(code), qDomain, r.ttl)
			}
		}
		if r.client.Exists(ctx, r.publishedKey(code)).Val() == 0 {
			if qDomain, err := r.repo.FindPublishedByCode(ctx, code); err == nil && qDomain != nil {
				_ = r.setCache(ctx, r.publishedKey(code), qDomain, r.ttl)
			}
		}
	}

	return nil
}
