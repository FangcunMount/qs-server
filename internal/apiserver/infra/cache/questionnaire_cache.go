package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	questionnaireInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	redis "github.com/redis/go-redis/v9"
)

const (
	QuestionnaireCachePrefix          = "questionnaire:"
	QuestionnairePublishedCachePrefix = "questionnaire:published:"
)

var DefaultQuestionnaireCacheTTL = 12 * time.Hour

type CachedQuestionnaireRepository struct {
	repo   domainQuestionnaire.Repository
	client redis.UniversalClient
	ttl    time.Duration
	mapper *questionnaireInfra.QuestionnaireMapper
}

func NewCachedQuestionnaireRepository(repo domainQuestionnaire.Repository, client redis.UniversalClient) domainQuestionnaire.Repository {
	return &CachedQuestionnaireRepository{
		repo:   repo,
		client: client,
		ttl:    DefaultQuestionnaireCacheTTL,
		mapper: questionnaireInfra.NewQuestionnaireMapper(),
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
	return r.loadWithCache(ctx, r.headKey(code), "questionnaire:head:"+strings.ToLower(code), func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
		return r.repo.FindByCode(ctx, code)
	})
}

func (r *CachedQuestionnaireRepository) FindPublishedByCode(ctx context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return r.loadWithCache(ctx, r.publishedKey(code), "questionnaire:published:"+strings.ToLower(code), func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
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
	return r.loadWithCache(ctx, key, "questionnaire:version:"+strings.ToLower(code)+":"+version, func(ctx context.Context) (*domainQuestionnaire.Questionnaire, error) {
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
	return addNamespace(QuestionnaireCachePrefix + strings.ToLower(code))
}

func (r *CachedQuestionnaireRepository) publishedKey(code string) string {
	return addNamespace(QuestionnairePublishedCachePrefix + strings.ToLower(code))
}

func (r *CachedQuestionnaireRepository) versionKey(code, version string) string {
	return addNamespace(fmt.Sprintf("%s%s:%s", QuestionnaireCachePrefix, strings.ToLower(code), version))
}

func (r *CachedQuestionnaireRepository) getCache(ctx context.Context, key string) (*domainQuestionnaire.Questionnaire, error) {
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

	data := decompressIfNeeded(dataBytes)
	var po questionnaireInfra.QuestionnairePO
	if err := json.Unmarshal(data, &po); err != nil {
		logger.L(ctx).Warnw("failed to unmarshal cached questionnaire", "key", key, "error", err)
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}

func (r *CachedQuestionnaireRepository) setCache(ctx context.Context, key string, qDomain *domainQuestionnaire.Questionnaire, ttl time.Duration) error {
	po := r.mapper.ToPO(qDomain)
	data, err := json.Marshal(po)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, compressIfEnabled(data), JitterTTL(ttl)).Err()
}

func (r *CachedQuestionnaireRepository) loadWithCache(
	ctx context.Context,
	key string,
	groupKey string,
	fallback func(context.Context) (*domainQuestionnaire.Questionnaire, error),
) (*domainQuestionnaire.Questionnaire, error) {
	if r.client != nil {
		cached, err := r.getCache(ctx, key)
		if err == nil {
			return cached, nil
		}
		if err != ErrCacheNotFound {
			return nil, err
		}
	}

	val, err, _ := Group.Do(groupKey, func() (interface{}, error) {
		return fallback(ctx)
	})
	if err != nil {
		return nil, err
	}

	qDomain, _ := val.(*domainQuestionnaire.Questionnaire)
	if r.client != nil {
		if qDomain == nil {
			_ = r.client.Set(ctx, key, []byte{}, JitterTTL(NegativeCacheTTL)).Err()
			return nil, nil
		}
		go func(q *domainQuestionnaire.Questionnaire) {
			_ = r.setCache(context.Background(), key, q, r.ttl)
		}(qDomain)
	}
	return qDomain, nil
}

func (r *CachedQuestionnaireRepository) deleteCacheByCode(ctx context.Context, code string) error {
	patterns := []string{
		r.headKey(code),
		r.publishedKey(code),
		addNamespace(fmt.Sprintf("%s%s:*", QuestionnaireCachePrefix, strings.ToLower(code))),
	}

	for _, pattern := range patterns[:2] {
		if err := r.client.Del(ctx, pattern).Err(); err != nil {
			logger.L(ctx).Warnw("failed to delete questionnaire cache key", "key", pattern, "error", err)
		}
	}

	iter := r.client.Scan(ctx, 0, patterns[2], 100).Iterator()
	for iter.Next(ctx) {
		_ = r.client.Del(ctx, iter.Val()).Err()
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
