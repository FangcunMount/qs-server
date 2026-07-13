package container

import (
	"context"
	"strings"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	surveycache "github.com/FangcunMount/qs-server/internal/apiserver/cache/survey"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

func (c *Container) CacheHandle(family redisruntime.Family) *redisruntime.Handle {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Handle(family)
}

func (c *Container) CacheClient(family redisruntime.Family) redis.UniversalClient {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Client(family)
}

func (c *Container) CacheBuilder(family redisruntime.Family) *keyspace.Builder {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Builder(family)
}

func (c *Container) CachePolicyProvider() sharedcache.PolicyProvider {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.EffectiveRegistry()
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

func (c *Container) WarmupCoordinator() statisticsApp.WarmupCoordinator {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.WarmupCoordinator()
}

func (c *Container) CacheGovernanceStatusService() statisticsApp.GovernanceStatusReader {
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
		c.CacheHandle(redisruntime.FamilyOps),
		cachesignal.ConfigFromReportStatus(c.reportStatusConfig),
	)
	if err != nil {
		return err
	}
	if c.cache != nil {
		c.cache.BindSignalNotifier(notifier)
	}
	return nil
}

func (c *Container) CacheSignalNotifier() *cachesignal.Notifier {
	if c == nil {
		return nil
	}
	if c.cache == nil {
		return nil
	}
	return c.cache.SignalNotifier()
}

func (c *Container) StartCacheSignalWatcher(ctx context.Context) {
	if c == nil || c.cache == nil {
		return
	}
	_ = c.cache.Start(ctx)
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
	if c != nil && c.CacheClient(redisruntime.FamilyStatic) != nil {
		warmScale = a.warmScaleCacheTarget
		warmQuestionnaire = a.warmQuestionnaireCacheTarget
	}

	var warmStatsSystem func(context.Context, int64) error
	var warmStatsOverview func(context.Context, int64, string) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c != nil && c.CacheClient(redisruntime.FamilyQuery) != nil && capabilityEnabled(c.CachePolicyProvider(), cachepolicy.CapabilityStatisticsQuery) {
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
		WarmPublishedTypologyModel:      a.warmPublishedTypologyModel,
		WarmStatsOverview:               warmStatsOverview,
		WarmStatsSystem:                 warmStatsSystem,
		WarmStatsQuestionnaire:          warmStatsQuestionnaire,
		WarmStatsPlan:                   warmStatsPlan,
	}
}

func capabilityEnabled(provider sharedcache.PolicyProvider, capability sharedcache.Capability) bool {
	if provider == nil {
		return false
	}
	effective, ok := provider.Resolve(capability)
	return ok && effective.Enabled
}

func (a cacheGovernanceAdapter) listPublishedScaleCodes(ctx context.Context) ([]string, error) {
	if a.container == nil {
		return nil, nil
	}
	lister := a.container.PublishedModelLister()
	if lister == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, _, err := lister.ListPublishedModels(ctx, modelcatalogport.ListPublishedFilter{Kind: domain.KindScale, Page: page, PageSize: pageSize})
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
	infra := a.containerSurveyRuntimeInfra()
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
	if a.container == nil {
		return "", nil
	}
	lister := a.container.PublishedModelLister()
	if lister == nil {
		return "", nil
	}
	model, err := lister.FindPublishedModelByCode(ctx, domain.KindScale, code)
	if err != nil || model == nil {
		return "", err
	}
	return model.QuestionnaireCode, nil
}

func (a cacheGovernanceAdapter) warmScaleCacheTarget(ctx context.Context, code string) error {
	if a.container == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	lister := a.container.PublishedModelLister()
	if lister == nil {
		return nil
	}
	_, err := lister.FindPublishedModelByCode(ctx, domain.KindScale, code)
	return err
}

func (a cacheGovernanceAdapter) warmQuestionnaireCacheTarget(ctx context.Context, code string) error {
	infra := a.containerSurveyRuntimeInfra()
	if infra == nil || infra.QuestionnaireRepo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := infra.QuestionnaireRepo.(*surveycache.CachedQuestionnaireRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := infra.QuestionnaireRepo.FindBaseByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) warmPublishedTypologyModel(ctx context.Context, code string) error {
	c := a.container
	if c == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	lister := c.PublishedModelLister()
	if lister == nil {
		return nil
	}
	_, err := lister.FindPublishedModelByCode(ctx, domain.KindTypology, code)
	return err
}

func (a cacheGovernanceAdapter) containerSurveyRuntimeInfra() *surveymod.SurveyRuntimeInfra {
	if a.container == nil {
		return nil
	}
	return a.container.surveyRuntimeInfra
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
