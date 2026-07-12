package compose

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
)

// EvaluationCatalog carries the single runtime registration source for Evaluation wiring.
type EvaluationCatalog struct {
	RuntimeDescriptorRegistry *evalpipeline.RuntimeDescriptorRegistry
}

// ActorIAMPorts carries IAM integration inputs for actor module installation.
type ActorIAMPorts struct {
	Enabled             bool
	ProfileLinkService  *iam.ProfileLinkService
	IdentityService     *iam.IdentityService
	OperationAccountSvc *iam.OperationAccountService
	IAMClient           *iam.Client
	AuthzSnapshotLoader *iam.AuthzSnapshotLoader
}

// SurveyPorts exposes survey-side outputs needed by downstream modules.
type SurveyPorts struct {
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
	QuestionnaireQuery     quesApp.QuestionnaireQueryService
}

// ActorPorts exposes actor-side outputs needed by downstream modules.
type ActorPorts struct {
	TesteeAccess actorAccessApp.TesteeAccessService
}
