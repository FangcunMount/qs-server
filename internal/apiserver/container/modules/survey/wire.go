package survey

import (
	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for survey module installation.
type WireInput struct {
	MongoDB             *mongo.Database
	MySQLDB             *gorm.DB
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
		MySQLDB:             in.MySQLDB,
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
