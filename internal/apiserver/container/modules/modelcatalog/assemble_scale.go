package assessmentmodel

import (
	"context"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale"
	scaleLifecycle "github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/lifecycle"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// Scale assembles scale-definition application services.
type Scale struct {
	LifecycleService scaleApp.ScaleLifecycleService
	FactorService    scaleApp.ScaleFactorService
	QueryService     scaleApp.ScaleQueryService
	CategoryService  scaleApp.ScaleCategoryService

	eventPublisher event.EventPublisher
}

// ScaleDeps defines explicit constructor dependencies for the scale capability.
type ScaleDeps struct {
	EventPublisher         event.EventPublisher
	Repo                   scaledefinition.Repository
	Reader                 scalereadmodel.ScaleReader
	ListCache              scalelistcache.PublishedListCache
	HotListCache           scalelistcache.HotListCache
	QuestionnaireCatalog   questionnairecatalog.Catalog
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
	RankRedisClient        redis.UniversalClient
	RankCacheBuilder       *keyspace.Builder
	IdentityService        *iam.IdentityService
	HotsetRecorder         cachetarget.HotsetRecorder
	CacheSignalNotifier    scaleLifecycle.CacheSignalNotifier
	RuleSetPublisher       scaleLifecycle.RuleSetPublisher
}

// NewScale assembles the scale capability.
func NewScale(deps ScaleDeps) (*Scale, error) {
	normalized, err := normalizeScaleDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &Scale{}
	module.eventPublisher = normalized.EventPublisher

	module.LifecycleService = scaleApp.NewLifecycleService(
		normalized.Repo,
		normalized.QuestionnaireCatalog,
		module.eventPublisher,
		normalized.ListCache,
		scaleApp.WithQuestionnairePublisher(newScaleQuestionnairePublisher(normalized.QuestionnairePublisher)),
		scaleApp.WithCacheSignalNotifier(normalized.CacheSignalNotifier),
		scaleApp.WithRuleSetPublisher(normalized.RuleSetPublisher),
	)
	module.FactorService = scaleApp.NewFactorService(normalized.Repo, normalized.ListCache, module.eventPublisher)
	hotRankReader := scaleCache.NewRedisScaleHotRankProjection(normalized.RankRedisClient, normalized.RankCacheBuilder)
	module.QueryService = scaleApp.NewQueryServiceWithHotListCache(
		normalized.Repo,
		normalized.Reader,
		normalized.IdentityService,
		normalized.ListCache,
		normalized.HotListCache,
		normalized.HotsetRecorder,
		hotRankReader,
	)
	module.CategoryService = scaleApp.NewCategoryService()

	return module, nil
}

func newScaleQuestionnairePublisher(service quesApp.QuestionnaireLifecycleService) scaleApp.QuestionnairePublisherFunc {
	if service == nil {
		return nil
	}
	return func(ctx context.Context, code string) (string, error) {
		result, err := service.Publish(ctx, code)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Version, nil
	}
}

func normalizeScaleDeps(deps ScaleDeps) (ScaleDeps, error) {
	if deps.Repo == nil || deps.Reader == nil {
		return ScaleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "scale repository and read model are required")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// Cleanup releases module resources.
func (m *Scale) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Scale) CheckHealth() error {
	return nil
}

// ModuleInfo returns legacy scale module metadata.
func (m *Scale) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        "scale",
		Version:     "2.0.0",
		Description: "量表管理模块（重构版）",
	}
}
