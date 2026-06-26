package survey

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// InstallHost extends the shared compose seam with survey-specific bindings.
type InstallHost interface {
	compose.Host
	EnsureSurveyScaleInfra() (*ScaleInfra, error)
	SetSurveyModule(*Module)
}

// InstallFrom wires and registers the survey module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	infra, err := host.EnsureSurveyScaleInfra()
	if err != nil {
		return err
	}
	module, err := Wire(WireInput{
		MongoDB:              host.MongoDB(),
		EventPublisher:       host.EventPublisher(),
		RankRedisClient:      host.CacheClient(cacheplane.FamilyRank),
		RankCacheBuilder:     host.CacheBuilder(cacheplane.FamilyRank),
		IdentityService:      host.IdentityService(),
		HotsetRecorder:       host.HotsetRecorder(),
		TopicResolver:        host.TopicResolver(),
		OutboxRelayBatchSize: host.OutboxRelayMongoBatchSize(),
		CacheSignalNotifier:  host.CacheSignalNotifier(),
		OpsHandle:            host.CacheHandle(cacheplane.FamilyOps),
		ScaleInfra:           infra,
	})
	if err != nil {
		return err
	}
	host.SetSurveyModule(module)
	host.RegisterModule("survey", module)
	host.Printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}
