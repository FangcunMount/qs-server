package statistics

import (
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with statistics module bindings.
type InstallHost interface {
	compose.Host
	SurveyRuntimeInfra() *surveymod.SurveyRuntimeInfra
	SetStatisticsModule(*Module)
}

// InstallFrom wires and registers the statistics module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	var answerSheetScanSource statisticsApp.AnswerSheetScanSource
	if infra := host.SurveyRuntimeInfra(); infra != nil {
		answerSheetScanSource = infra.AnswerSheetRepo
	}
	provider := host.CachePolicyProvider()
	binding := compose.ResolveCacheCapability(provider, cachepolicy.CapabilityStatisticsQuery)
	queryRedis := host.CacheClient(redisruntime.FamilyQuery)
	if !binding.Enabled {
		queryRedis = nil
	}
	module, err := Wire(WireInput{
		MySQLDB:               host.MySQLDB(),
		FallbackRedisClient:   queryRedis,
		CacheBuilder:          host.CacheBuilder(redisruntime.FamilyQuery),
		AnswerSheetScanSource: answerSheetScanSource,
		MongoDB:               host.MongoDB(),
		RepairWindowDays:      host.StatisticsRepairWindowDays(),
		CachePolicies:         provider,
		OverviewGuardOpts:     host.StatisticsOverviewGuardOptions(),
		HotsetRecorder:        host.HotsetRecorder(),
		LockManager:           host.LockManager(),
		Observer:              host.CacheObserver(),
		MySQLLimiter:          host.MySQLLimiter(),
		WarmupCoordinator:     host.WarmupCoordinator(),
		StatusService:         host.CacheGovernanceStatusService(),
		MetaRedisClient:       host.CacheClient(redisruntime.FamilyMeta),
	})
	if err != nil {
		return err
	}
	host.SetStatisticsModule(module)
	host.RegisterModule("statistics", module)
	host.Printf("📦 Statistics module initialized\n")
	return nil
}
