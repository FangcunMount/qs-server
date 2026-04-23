package container

import (
	"context"
	"fmt"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

func (c *Container) buildStatisticsModuleInitializeParams() []interface{} {
	redisClient := c.redisCache
	if !c.cacheOptions.DisableStatisticsCache {
		redisClient = c.queryRedisCache
	}
	if c.cacheOptions.DisableStatisticsCache {
		redisClient = nil
	}

	var answerSheetRepo interface{}
	if c != nil && c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil {
		answerSheetRepo = c.SurveyModule.AnswerSheet.Repo
	}

	return []interface{}{
		c.mysqlDB,
		redisClient,
		redisHandleBuilder(c.queryRedisHandle),
		answerSheetRepo,
		c.statisticsRepairWindowDays,
		c.policyCatalog.Policy(cachepolicy.PolicyStatsQuery),
		c.hotsetRecorder,
		redislock.NewManager("apiserver", "statistics_sync", c.lockRedisHandle),
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
	if c.staticRedisCache != nil {
		warmScale = c.warmScaleCacheTarget
		warmQuestionnaire = c.warmQuestionnaireCacheTarget
		warmScaleList = c.warmScaleListTarget
	}
	var warmStatsSystem func(context.Context, int64) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c.queryRedisCache != nil && !c.cacheOptions.DisableStatisticsCache {
		warmStatsSystem = c.warmSystemStatsTarget
		warmStatsQuestionnaire = c.warmQuestionnaireStatsTarget
		warmStatsPlan = c.warmPlanStatsTarget
	}
	c.WarmupCoordinator = cachegov.NewCoordinator(cachegov.Config{
		Enable:          c.cacheOptions.Warmup.Enable,
		StartupStatic:   c.cacheOptions.Warmup.StartupStatic,
		StartupQuery:    c.cacheOptions.Warmup.StartupQuery,
		HotsetEnable:    c.cacheOptions.Warmup.HotsetEnable,
		HotsetTopN:      c.cacheOptions.Warmup.HotsetTopN,
		MaxItemsPerKind: c.cacheOptions.Warmup.MaxItemsPerKind,
	}, cachegov.Dependencies{
		Runtime:                         cachegov.NewFamilyRuntime(c.staticRedisHandle, c.queryRedisHandle),
		StatisticsSeeds:                 c.cacheOptions.StatisticsWarmup,
		Hotset:                          c.hotsetRecorder,
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
	if c.StatisticsModule != nil {
		c.StatisticsModule.SetWarmupCoordinator(c.WarmupCoordinator)
	}
	c.CacheGovernanceStatusService = cachegov.NewStatusService("apiserver", nil, c.hotsetInspector, c.WarmupCoordinator)
	return nil
}
