package survey

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with survey-specific bindings.
type InstallHost interface {
	compose.Host
	EnsureSurveyRuntimeInfra() (*SurveyRuntimeInfra, error)
	SetSurveyModule(*Module)
}

// InstallFrom wires and registers the survey module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	infra, err := host.EnsureSurveyRuntimeInfra()
	if err != nil {
		return err
	}
	module, err := Wire(WireInput{
		MongoDB:                           host.MongoDB(),
		EventPublisher:                    host.EventPublisher(),
		RankRedisClient:                   host.CacheClient(redisruntime.FamilyRank),
		RankCacheBuilder:                  host.CacheBuilder(redisruntime.FamilyRank),
		IdentityService:                   host.IdentityService(),
		HotsetRecorder:                    host.HotsetRecorder(),
		TopicResolver:                     host.TopicResolver(),
		OutboxRelayBatchSize:              host.OutboxRelayMongoBatchSize(),
		OutboxRelayPublishWorkers:         host.OutboxRelayMongoPublishWorkers(),
		OutboxRelayImmediateMaxConcurrent: host.OutboxRelayMongoImmediateMaxConcurrent(),
		CacheSignalNotifier:               host.CacheSignalNotifier(),
		OpsHandle:                         host.CacheHandle(redisruntime.FamilyOps),
		SurveyRuntimeInfra:                infra,
	})
	if err != nil {
		return err
	}
	host.SetSurveyModule(module)
	host.RegisterModule("survey", module)
	host.Printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}
