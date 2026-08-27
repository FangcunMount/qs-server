package interpretation

import (
	"testing"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestAssembleAIExplanationDisabledHasNoRuntimeDependencies(t *testing.T) {
	module := &Module{}
	err := module.assembleAIExplanation(Deps{
		AIExplanation: &apiserveroptions.AIExplanationOptions{Enabled: false},
	}, mongoBase.BaseRepositoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if module.aiExplanationEnabled || module.aiProfileRepo != nil || module.aiEvaluationRepo != nil ||
		module.aiExplanationExecutor != nil || module.aiExplanationService != nil || module.aiAdministration != nil {
		t.Fatal("disabled AI explanation must not initialize repositories, services, or transports")
	}
}
