package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	surveyAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func (c *Container) buildStatisticsModuleDeps() assembler.StatisticsModuleDeps {
	versionStore := cachequery.NewStaticVersionTokenStore(0)
	if c == nil {
		return assembler.StatisticsModuleDeps{
			VersionStore: versionStore,
		}
	}

	disableStatisticsCache := c.cacheOptions.DisableStatisticsCache
	redisClient := c.redisCache
	if !disableStatisticsCache {
		redisClient = c.CacheClient(redisplane.FamilyQuery)
	}
	if disableStatisticsCache {
		redisClient = nil
	}

	var answerSheetRepo surveyAnswerSheet.Repository
	if c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil {
		answerSheetRepo = c.SurveyModule.AnswerSheet.Repo
	}
	if !disableStatisticsCache {
		versionStore = cachequery.NewRedisVersionTokenStoreWithKindAndObserver(
			c.CacheClient(redisplane.FamilyMeta),
			string(cachepolicy.PolicyStatsQuery),
			c.cacheObserver(),
		)
		if versionStore == nil {
			versionStore = cachequery.NewStaticVersionTokenStore(0)
		}
	}

	return assembler.StatisticsModuleDeps{
		MySQLDB:          c.mysqlDB,
		RedisClient:      redisClient,
		CacheBuilder:     c.CacheBuilder(redisplane.FamilyQuery),
		AnswerSheetRepo:  answerSheetRepo,
		RepairWindowDays: c.statisticsRepairWindowDays,
		QueryPolicy:      c.CachePolicy(cachepolicy.PolicyStatsQuery),
		HotsetRecorder:   c.hotsetRecorder(),
		LockManager:      c.CacheLockManager(),
		VersionStore:     versionStore,
		Observer:         c.cacheObserver(),
	}
}

func (c *Container) buildStatisticsModule() (*assembler.StatisticsModule, error) {
	return assembler.NewStatisticsModule(c.buildStatisticsModuleDeps())
}

// initStatisticsModule 初始化 Statistics 模块。
func (c *Container) initStatisticsModule() error {
	statisticsModule, err := c.buildStatisticsModule()
	if err != nil {
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
	if c.cache != nil {
		c.cache.BindGovernance(newCacheGovernanceAdapter(c).bindings())
	}
	if c.StatisticsModule != nil {
		c.StatisticsModule.SetWarmupCoordinator(c.WarmupCoordinator())
		if c.StatisticsModule.Handler != nil {
			c.StatisticsModule.Handler.SetCacheGovernanceStatusService(c.CacheGovernanceStatusService())
		}
	}
	return nil
}
