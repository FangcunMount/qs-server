package statistics

import (
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
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
	var answerSheetReader surveyreadmodel.AnswerSheetReader
	var answerSheetScanSource statisticsApp.AnswerSheetScanSource
	if infra := host.SurveyRuntimeInfra(); infra != nil {
		answerSheetReader = infra.AnswerSheetReader
		answerSheetScanSource = infra.AnswerSheetRepo
	}
	module, err := Wire(WireInput{
		MySQLDB:                host.MySQLDB(),
		RedisClient:            host.RedisCache(),
		FallbackRedisClient:    host.CacheClient(redisruntime.FamilyQuery),
		CacheBuilder:           host.CacheBuilder(redisruntime.FamilyQuery),
		AnswerSheetReader:      answerSheetReader,
		AnswerSheetScanSource:  answerSheetScanSource,
		MongoDB:                host.MongoDB(),
		RepairWindowDays:       host.StatisticsRepairWindowDays(),
		QueryPolicy:            host.CachePolicy(cachepolicy.PolicyStatsQuery),
		SystemStatisticsOpts:   host.StatisticsSystemOptions(),
		OverviewGuardOpts:      host.StatisticsOverviewGuardOptions(),
		QuestionnaireGuardOpts: host.StatisticsQuestionnaireGuardOptions(),
		HotsetRecorder:         host.HotsetRecorder(),
		LockManager:            host.CacheLockManager(),
		Observer:               host.CacheObserver(),
		MySQLLimiter:           host.MySQLLimiter(),
		WarmupCoordinator:      host.WarmupCoordinator(),
		StatusService:          host.CacheGovernanceStatusService(),
		DisableStatisticsCache: host.DisableStatisticsCache(),
		MetaRedisClient:        host.CacheClient(redisruntime.FamilyMeta),
	})
	if err != nil {
		return err
	}
	host.SetStatisticsModule(module)
	host.RegisterModule("statistics", module)
	host.Printf("📦 Statistics module initialized\n")
	return nil
}
