package compose

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
)

// EvaluationCatalog carries shared model descriptors for report/evaluation wiring.
type EvaluationCatalog struct {
	Descriptors               []evaldomain.ModelDescriptor
	TypologyRegistry          evalregistry.TypologyRegistry
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
