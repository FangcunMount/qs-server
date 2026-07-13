package survey

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for survey module installation.
type WireInput struct {
	MongoDB             *mongo.Database
	EventPublisher      event.EventPublisher
	IdentityService     *iam.IdentityService
	HotsetRecorder      cachetarget.HotsetRecorder
	CacheSignalNotifier quesApp.CacheSignalNotifier
	SurveyRuntimeInfra  *SurveyRuntimeInfra
	OutboxProfile       appEventing.ProfileBinding
}

// Wire builds and bootstraps the survey module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	bootstrap := BootstrapInput{
		MongoDB:             in.MongoDB,
		EventPublisher:      in.EventPublisher,
		IdentityService:     in.IdentityService,
		HotsetRecorder:      in.HotsetRecorder,
		CacheSignalNotifier: in.CacheSignalNotifier,
		OutboxProfile:       in.OutboxProfile,
	}
	if infra := in.SurveyRuntimeInfra; infra != nil {
		bootstrap.QuestionnaireRepo = infra.QuestionnaireRepo
		bootstrap.QuestionnaireReader = infra.QuestionnaireReader
		bootstrap.AnswerSheetRepo = infra.AnswerSheetRepo
		bootstrap.AnswerSheetReader = infra.AnswerSheetReader
	}
	return Bootstrap(bootstrap)
}
