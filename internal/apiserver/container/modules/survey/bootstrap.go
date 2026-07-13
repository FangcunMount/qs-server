package survey

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"go.mongodb.org/mongo-driver/mongo"

	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// BootstrapInput carries container integration inputs for survey module bootstrap.
type BootstrapInput struct {
	MongoDB             *mongo.Database
	EventPublisher      event.EventPublisher
	IdentityService     *iam.IdentityService
	HotsetRecorder      cachetarget.HotsetRecorder
	QuestionnaireRepo   questionnaire.Repository
	QuestionnaireReader surveyreadmodel.QuestionnaireReader
	AnswerSheetRepo     AnswerSheetStore
	AnswerSheetReader   surveyreadmodel.AnswerSheetReader
	CacheSignalNotifier quesApp.CacheSignalNotifier
	OutboxProfile       appEventing.ProfileBinding
}

// Bootstrap assembles the survey module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
