package survey

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
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
		MongoDB:             host.MongoDB(),
		EventPublisher:      host.EventPublisher(),
		IdentityService:     host.IdentityService(),
		HotsetRecorder:      host.HotsetRecorder(),
		CacheSignalNotifier: host.CacheSignalNotifier(),
		SurveyRuntimeInfra:  infra,
		OutboxProfile:       host.EventProfile(eventcatalog.OutboxProfileMongoDomain),
	})
	if err != nil {
		return err
	}
	host.SetSurveyModule(module)
	host.RegisterModule("survey", module)
	host.Printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}
