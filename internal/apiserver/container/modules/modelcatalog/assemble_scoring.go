package modelcatalog

import (
	"context"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	scoringApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring"
	scoringLifecycle "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/lifecycle"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// Scoring assembles scoring-definition application services.
type Scoring struct {
	LifecycleService scoringApp.ScaleLifecycleService
	FactorService    scoringApp.ScaleFactorService
	QueryService     scoringApp.ScaleQueryService
	CategoryService  scoringApp.ScaleCategoryService

	eventPublisher event.EventPublisher
}

// ScoringDeps defines explicit constructor dependencies for the scoring capability.
type ScoringDeps struct {
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
	CacheSignalNotifier    scoringLifecycle.CacheSignalNotifier
	ScalePublisher         scoringLifecycle.ScalePublisher
	ModelRepo              modelcatalogport.ModelRepository
	PublishedRepo          modelcatalogport.PublishedModelRepository
	PublishedReader        modelcatalogport.PublishedModelReader
}

// NewScoring assembles the scoring capability.
func NewScoring(deps ScoringDeps) (*Scoring, error) {
	normalized, err := normalizeScoringDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &Scoring{}
	module.eventPublisher = normalized.EventPublisher

	module.LifecycleService = scoringApp.NewLifecycleService(
		normalized.Repo,
		normalized.QuestionnaireCatalog,
		module.eventPublisher,
		normalized.ListCache,
		scoringApp.WithQuestionnairePublisher(newScoringQuestionnairePublisher(normalized.QuestionnairePublisher)),
		scoringApp.WithCacheSignalNotifier(normalized.CacheSignalNotifier),
		scoringApp.WithScalePublisher(normalized.ScalePublisher),
		scoringApp.WithAssessmentSnapshotPublisher(newScaleAssessmentSnapshotPublisher(normalized.ModelRepo, normalized.PublishedRepo)),
	)
	module.FactorService = scoringApp.NewFactorService(normalized.Repo, normalized.ListCache, module.eventPublisher)
	hotRankReader := scaleCache.NewRedisScaleHotRankProjection(normalized.RankRedisClient, normalized.RankCacheBuilder)
	module.QueryService = scoringApp.NewQueryServiceWithModelCatalogSources(
		normalized.Repo,
		normalized.Reader,
		normalized.IdentityService,
		normalized.ListCache,
		normalized.HotListCache,
		normalized.HotsetRecorder,
		queryModelCatalogSources(normalized),
		hotRankReader,
	)
	module.CategoryService = scoringApp.NewCategoryService()

	return module, nil
}

func queryModelCatalogSources(deps ScoringDeps) scoringApp.ModelCatalogSources {
	return scoringApp.ModelCatalogSources{
		ModelRepo:       deps.ModelRepo,
		PublishedRepo:   deps.PublishedRepo,
		PublishedReader: deps.PublishedReader,
	}
}

func newScaleAssessmentSnapshotPublisher(modelRepo modelcatalogport.ModelRepository, publishedRepo modelcatalogport.PublishedModelRepository) scoringLifecycle.AssessmentSnapshotPublisher {
	if publishedRepo == nil {
		return nil
	}
	return scoringApp.NewAssessmentSnapshotPublisher(modelRepo, publishedRepo)
}

func newScoringQuestionnairePublisher(service quesApp.QuestionnaireLifecycleService) scoringApp.QuestionnairePublisherFunc {
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

func normalizeScoringDeps(deps ScoringDeps) (ScoringDeps, error) {
	if deps.Repo == nil || deps.Reader == nil {
		return ScoringDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "scale repository and read model are required")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// Cleanup releases module resources.
func (m *Scoring) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Scoring) CheckHealth() error {
	return nil
}

// ModuleInfo returns scoring capability metadata under modelcatalog.
func (m *Scoring) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        "modelcatalog.scoring",
		Version:     "2.0.0",
		Description: "量表管理模块（重构版）",
	}
}

// Scale is a deprecated alias for Scoring (container compat).
type Scale = Scoring

// ScaleDeps is a deprecated alias for ScoringDeps (container compat).
type ScaleDeps = ScoringDeps

// NewScale is a deprecated alias for NewScoring (container compat).
func NewScale(deps ScaleDeps) (*Scale, error) {
	return NewScoring(deps)
}
