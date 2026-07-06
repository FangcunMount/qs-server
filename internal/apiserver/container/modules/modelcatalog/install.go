package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// InstallHost extends the shared compose seam with assessment-model bindings.
type InstallHost interface {
	compose.Host
	EnsureSurveyScaleInfra() (*surveymod.ScaleInfra, error)
	SetAssessmentModelModule(*Module)
}

// InstallFrom wires and registers the assessment-model module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	infra, err := host.EnsureSurveyScaleInfra()
	if err != nil {
		return err
	}
	module, err := Wire(WireInput{
		MongoDB:                host.MongoDB(),
		MongoLimiter:           host.MongoLimiter(),
		EventPublisher:         host.EventPublisher(),
		RankRedisClient:        host.CacheClient(cacheplane.FamilyRank),
		RankCacheBuilder:       host.CacheBuilder(cacheplane.FamilyRank),
		IdentityService:        host.IdentityService(),
		HotsetRecorder:         host.HotsetRecorder(),
		CacheSignalNotifier:    host.CacheSignalNotifier(),
		ScaleInfra:             infra,
		QuestionnairePublisher: host.SurveyPorts().QuestionnairePublisher,
		QuestionnaireQuery:     host.SurveyPorts().QuestionnaireQuery,
		StaticRedisClient:      host.CacheClient(cacheplane.FamilyStatic),
		StaticCacheBuilder:     host.CacheBuilder(cacheplane.FamilyStatic),
		PublishedModelPolicy:   host.CachePolicy(cachepolicy.PolicyPublishedModel),
		CacheObserver:          host.CacheObserver(),
	})
	if err != nil {
		return err
	}
	host.SetAssessmentModelModule(module)
	host.Printf("📦 Assessment model module initialized (scale + personality catalog)\n")
	return nil
}
