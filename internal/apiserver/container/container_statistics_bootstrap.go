package container

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildStatisticsModuleInitializeParams() []interface{} {
	redisClient := c.redisCache
	if !c.cacheOptions.DisableStatisticsCache {
		redisClient = c.CacheClient(redisplane.FamilyQuery)
	}
	if c.cacheOptions.DisableStatisticsCache {
		redisClient = nil
	}

	var answerSheetRepo interface{}
	if c != nil && c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil {
		answerSheetRepo = c.SurveyModule.AnswerSheet.Repo
	}
	versionStore := scaleCache.NewStaticVersionTokenStore(0)
	if !c.cacheOptions.DisableStatisticsCache {
		versionStore = scaleCache.NewRedisVersionTokenStoreWithKindAndObserver(
			c.CacheClient(redisplane.FamilyMeta),
			string(cachepolicy.PolicyStatsQuery),
			c.cacheObserver(),
		)
		if versionStore == nil {
			versionStore = scaleCache.NewStaticVersionTokenStore(0)
		}
	}

	return []interface{}{
		c.mysqlDB,
		redisClient,
		c.CacheBuilder(redisplane.FamilyQuery),
		answerSheetRepo,
		c.statisticsRepairWindowDays,
		c.CachePolicy(cachepolicy.PolicyStatsQuery),
		c.hotsetRecorder(),
		c.CacheLockManager(),
		versionStore,
		c.cacheObserver(),
	}
}

// initStatisticsModule 初始化 Statistics 模块。
func (c *Container) initStatisticsModule() error {
	statisticsModule := assembler.NewStatisticsModule()
	if err := statisticsModule.Initialize(c.buildStatisticsModuleInitializeParams()...); err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}

	c.StatisticsModule = statisticsModule
	c.registerModule("statistics", statisticsModule)

	c.printf("📦 Statistics module initialized\n")
	return nil
}

func (c *Container) initWarmupCoordinator() error {
	if c == nil {
		return nil
	}
	var warmScale func(context.Context, string) error
	var warmQuestionnaire func(context.Context, string) error
	var warmScaleList func(context.Context) error
	if c.CacheClient(redisplane.FamilyStatic) != nil {
		warmScale = c.warmScaleCacheTarget
		warmQuestionnaire = c.warmQuestionnaireCacheTarget
		warmScaleList = c.warmScaleListTarget
	}
	var warmStatsSystem func(context.Context, int64) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c.CacheClient(redisplane.FamilyQuery) != nil && !c.cacheOptions.DisableStatisticsCache {
		warmStatsSystem = c.warmSystemStatsTarget
		warmStatsQuestionnaire = c.warmQuestionnaireStatsTarget
		warmStatsPlan = c.warmPlanStatsTarget
	}
	if c.cache != nil {
		c.cache.BindGovernance(cachebootstrap.GovernanceBindings{
			ListPublishedScaleCodes:         c.listPublishedScaleCodes,
			ListPublishedQuestionnaireCodes: c.listPublishedQuestionnaireCodes,
			LookupScaleQuestionnaireCode:    c.lookupScaleQuestionnaireCode,
			WarmScale:                       warmScale,
			WarmQuestionnaire:               warmQuestionnaire,
			WarmScaleList:                   warmScaleList,
			WarmStatsSystem:                 warmStatsSystem,
			WarmStatsQuestionnaire:          warmStatsQuestionnaire,
			WarmStatsPlan:                   warmStatsPlan,
		})
	}
	if c.StatisticsModule != nil {
		c.StatisticsModule.SetWarmupCoordinator(c.WarmupCoordinator())
		if c.StatisticsModule.Handler != nil {
			c.StatisticsModule.Handler.SetCacheGovernanceStatusService(c.CacheGovernanceStatusService())
		}
	}
	return nil
}
