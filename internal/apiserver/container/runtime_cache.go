package container

import (
	"context"
	"strings"
	"sync"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	redis "github.com/redis/go-redis/v9"
)

var initCacheSingleflight sync.Once

// ensureCacheSingleflightCoordinator 确保 cache singleflight coordinator 初始化
func ensureCacheSingleflightCoordinator() {
	initCacheSingleflight.Do(func() {
		cacheinfra.SetDefaultSingleflightCoordinator(cacheinfra.NewSingleflightCoordinator())
	})
}

func (c *Container) CacheHandle(family cacheplane.Family) *cacheplane.Handle {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Handle(family)
}

func (c *Container) CacheClient(family cacheplane.Family) redis.UniversalClient {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Client(family)
}

func (c *Container) CacheBuilder(family cacheplane.Family) *keyspace.Builder {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Builder(family)
}

func (c *Container) CachePolicy(key cachepolicy.CachePolicyKey) cachepolicy.CachePolicy {
	if c == nil || c.cache == nil {
		return cachepolicy.CachePolicy{}
	}
	return c.cache.Policy(key)
}

func (c *Container) cacheObserver() *observability.ComponentObserver {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Observer()
}

func (c *Container) hotsetRecorder() cachetarget.HotsetRecorder {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.HotsetRecorder()
}

func (c *Container) HotsetInspector() cachetarget.HotsetInspector {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.HotsetInspector()
}

func (c *Container) CacheLockManager() locklease.Manager {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.LockManager()
}

func (c *Container) WarmupCoordinator() cachegov.Coordinator {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.WarmupCoordinator()
}

func (c *Container) CacheGovernanceStatusService() cachegov.StatusService {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.StatusService()
}

func (c *Container) initCacheSignalNotifier() error {
	if c == nil {
		return nil
	}
	notifier, err := cachesignal.NewNotifier(
		c.CacheHandle(cacheplane.FamilyOps),
		cachesignal.ConfigFromReportStatus(c.reportStatusConfig),
	)
	if err != nil {
		return err
	}
	c.cacheSignalNotifier = notifier
	return nil
}

func (c *Container) CacheSignalNotifier() *cachesignal.Notifier {
	if c == nil {
		return nil
	}
	return c.cacheSignalNotifier
}

func (c *Container) StartCacheSignalWatcher(ctx context.Context) {
	if c == nil {
		return
	}
	notifier := c.CacheSignalNotifier()
	if notifier == nil {
		return
	}
	cachegov.StartCacheSignalWatcher(
		ctx,
		c.WarmupCoordinator(),
		notifier.QuestionnaireSignaler(),
		notifier.ScaleSignaler(),
		notifier.TypologyModelSignaler(),
	)
}

type cacheGovernanceAdapter struct {
	container *Container
}

func newCacheGovernanceAdapter(c *Container) cacheGovernanceAdapter {
	return cacheGovernanceAdapter{container: c}
}

func (a cacheGovernanceAdapter) bindings() cachebootstrap.GovernanceBindings {
	c := a.container
	var warmScale func(context.Context, string) error
	var warmQuestionnaire func(context.Context, string) error
	var warmScaleList func(context.Context) error
	if c != nil && c.CacheClient(cacheplane.FamilyStatic) != nil {
		warmScale = a.warmScaleCacheTarget
		warmQuestionnaire = a.warmQuestionnaireCacheTarget
		warmScaleList = a.warmScaleListTarget
	}

	var warmStatsSystem func(context.Context, int64) error
	var warmStatsOverview func(context.Context, int64, string) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c != nil && c.CacheClient(cacheplane.FamilyQuery) != nil && !c.cacheOptions.DisableStatisticsCache {
		warmStatsOverview = a.warmOverviewStatsTarget
		warmStatsSystem = a.warmSystemStatsTarget
		warmStatsQuestionnaire = a.warmQuestionnaireStatsTarget
		warmStatsPlan = a.warmPlanStatsTarget
	}

	return cachebootstrap.GovernanceBindings{
		ListPublishedScaleCodes:         a.listPublishedScaleCodes,
		ListPublishedQuestionnaireCodes: a.listPublishedQuestionnaireCodes,
		LookupScaleQuestionnaireCode:    a.lookupScaleQuestionnaireCode,
		WarmScale:                       warmScale,
		WarmQuestionnaire:               warmQuestionnaire,
		WarmScaleList:                   warmScaleList,
		WarmPublishedTypologyModel:      a.warmPublishedTypologyModel,
		WarmStatsOverview:               warmStatsOverview,
		WarmStatsSystem:                 warmStatsSystem,
		WarmStatsQuestionnaire:          warmStatsQuestionnaire,
		WarmStatsPlan:                   warmStatsPlan,
	}
}

func (a cacheGovernanceAdapter) listPublishedScaleCodes(ctx context.Context) ([]string, error) {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.ScaleReader == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := infra.ScaleReader.ListScales(ctx, scalereadmodel.ScaleFilter{Status: scalereadmodel.ScaleStatusPublished, PublishedOnly: true}, scalereadmodel.PageRequest{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			codes = append(codes, item.Code)
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (a cacheGovernanceAdapter) listPublishedQuestionnaireCodes(ctx context.Context) ([]string, error) {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.QuestionnaireReader == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := infra.QuestionnaireReader.ListPublishedQuestionnaires(ctx, surveyreadmodel.QuestionnaireFilter{Status: "published"}, surveyreadmodel.PageRequest{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			codes = append(codes, item.Code)
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (a cacheGovernanceAdapter) lookupScaleQuestionnaireCode(ctx context.Context, code string) (string, error) {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.AssessmentModelRepo == nil {
		return "", nil
	}
	model, err := infra.AssessmentModelRepo.FindByCode(ctx, code)
	if err != nil || model == nil {
		return "", err
	}
	return model.Binding.QuestionnaireCode, nil
}

func (a cacheGovernanceAdapter) warmScaleCacheTarget(ctx context.Context, code string) error {
	if a.container == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	catalog, err := a.container.ensurePublishedModelCatalog()
	if err != nil {
		return err
	}
	lister, ok := catalog.(modelcatalogport.PublishedModelLister)
	if !ok || lister == nil {
		return nil
	}
	_, err = lister.FindPublishedModelByCode(ctx, domain.KindScale, code)
	return err
}

func (a cacheGovernanceAdapter) warmQuestionnaireCacheTarget(ctx context.Context, code string) error {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.QuestionnaireRepo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := infra.QuestionnaireRepo.(*cacheinfra.CachedQuestionnaireRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := infra.QuestionnaireRepo.FindBaseByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) warmScaleListTarget(ctx context.Context) error {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.ScaleListCache == nil {
		return nil
	}
	return infra.ScaleListCache.Rebuild(ctx)
}

func (a cacheGovernanceAdapter) warmPublishedTypologyModel(ctx context.Context, code string) error {
	c := a.container
	if c == nil || c.AssessmentModelModule == nil || c.AssessmentModelModule.Typology == nil {
		return nil
	}
	query := c.AssessmentModelModule.Typology.QueryService
	if query == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	_, err := query.GetPublishedByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) containerSurveyScaleInfra() *surveymod.ScaleInfra {
	if a.container == nil {
		return nil
	}
	return a.container.surveyScaleInfra
}

func (a cacheGovernanceAdapter) warmSystemStatsTarget(ctx context.Context, orgID int64) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.SystemStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.SystemStatisticsService.GetSystemStatistics(ctx, orgID)
	return err
}

func (a cacheGovernanceAdapter) warmOverviewStatsTarget(ctx context.Context, orgID int64, preset string) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.ReadService == nil {
		return nil
	}
	_, err := c.StatisticsModule.ReadService.GetOverview(ctx, orgID, statisticsApp.QueryFilter{Preset: preset})
	return err
}

func (a cacheGovernanceAdapter) warmQuestionnaireStatsTarget(ctx context.Context, orgID int64, code string) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.QuestionnaireStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.QuestionnaireStatisticsService.GetQuestionnaireStatistics(ctx, orgID, code)
	return err
}

func (a cacheGovernanceAdapter) warmPlanStatsTarget(ctx context.Context, orgID int64, planID uint64) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.PlanStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.PlanStatisticsService.GetPlanStatistics(ctx, orgID, planID)
	return err
}
