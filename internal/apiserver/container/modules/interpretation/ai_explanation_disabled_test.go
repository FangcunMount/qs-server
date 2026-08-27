package interpretation

import (
	"context"
	"testing"

	aiexplanationevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	aiexplanationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/execution"
	aiexplanationparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/participant"
	aiexplanationrecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	aiexplanationsubjectexport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/subjectexport"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

type stagedRolloutExecutorStub struct{}

func (stagedRolloutExecutorStub) Execute(context.Context, aiexplanationexecution.Command) (*aiexplanationexecution.Result, error) {
	return nil, nil
}

type stagedRolloutParticipantStub struct{}

func (stagedRolloutParticipantStub) Capability(context.Context, aiexplanationparticipant.Actor, aiexplanationparticipant.RequestInput) (*aiexplanationparticipant.Result, error) {
	return nil, nil
}

func (stagedRolloutParticipantStub) Request(context.Context, aiexplanationparticipant.Actor, aiexplanationparticipant.RequestInput) (*aiexplanationparticipant.Result, error) {
	return nil, nil
}

func (stagedRolloutParticipantStub) Get(context.Context, aiexplanationparticipant.Actor, aiexplanationparticipant.GetInput) (*aiexplanationparticipant.Result, error) {
	return nil, nil
}

func TestAssembleAIExplanationDisabledHasNoRuntimeDependencies(t *testing.T) {
	module := &Module{}
	err := module.assembleAIExplanation(Deps{
		AIExplanation: &apiserveroptions.AIExplanationOptions{Enabled: false},
	}, mongoBase.BaseRepositoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if module.aiExplanationEnabled || module.aiParticipantEnabled || module.aiProfileRepo != nil || module.aiEvaluationRepo != nil ||
		module.aiExplanationExecutor != nil || module.aiExplanationService != nil || module.aiAdministration != nil {
		t.Fatal("disabled AI explanation must not initialize repositories, services, or transports")
	}
}

func TestAIExplanationParticipantTrafficRemainsClosedDuringEvaluationRollout(t *testing.T) {
	module := &Module{
		aiExplanationEnabled:        true,
		aiParticipantEnabled:        false,
		aiExplanationExecutor:       stagedRolloutExecutorStub{},
		aiExplanationService:        stagedRolloutParticipantStub{},
		aiSubjectExport:             &aiexplanationsubjectexport.Service{},
		aiOnlineEvalRunner:          &aiexplanationevaluation.OnlineRunner{},
		aiParticipantLeaseRecoverer: &aiexplanationrecovery.LeaseRecoverer{},
	}

	deps := module.ExportGRPCDeps()
	if deps.AIExplanationEvaluation == nil {
		t.Fatal("operator evaluation must remain available while participant traffic is disabled")
	}
	if deps.AIExplanationExecutor != nil || deps.AIExplanationParticipant != nil || deps.AIExplanationSubjectExport != nil {
		t.Fatal("participant executor, API, and subject export must remain unavailable before traffic enablement")
	}
	if module.AIExplanationParticipantLeaseRecoverer() != nil || module.AIExplanationSubjectExport() != nil {
		t.Fatal("participant recovery and export must remain unavailable before traffic enablement")
	}

	module.aiParticipantEnabled = true
	deps = module.ExportGRPCDeps()
	if deps.AIExplanationExecutor == nil || deps.AIExplanationParticipant == nil || deps.AIExplanationSubjectExport == nil {
		t.Fatal("participant runtime dependencies must be exported after traffic enablement")
	}
	if module.AIExplanationParticipantLeaseRecoverer() == nil || module.AIExplanationSubjectExport() == nil {
		t.Fatal("participant recovery and export must be available after traffic enablement")
	}
}
