package definition

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestValidateAlgorithmBindingRequiresBrief2Execution(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &modeldefinition.Definition{},
	}
	issues := ValidateAlgorithmBinding(model)
	if !hasIssueCode(issues, "brief2.execution.required") {
		t.Fatalf("issues = %#v, want brief2.execution.required", issues)
	}
}

func TestValidateAlgorithmBindingRequiresSPMExecution(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		DefinitionV2: &modeldefinition.Definition{},
	}
	issues := ValidateAlgorithmBinding(model)
	if !hasIssueCode(issues, "spm.execution.required") {
		t.Fatalf("issues = %#v, want spm.execution.required", issues)
	}
}

func TestValidateAlgorithmBindingRejectsEmptyFactorNormAlgorithm(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: "",
		DefinitionV2: &modeldefinition.Definition{},
	}
	issues := ValidateAlgorithmBinding(model)
	if !hasIssueCode(issues, "algorithm.publish.required") {
		t.Fatalf("issues = %#v, want algorithm.publish.required", issues)
	}
}

func TestValidateAlgorithmBindingRejectsEmptyCognitiveAlgorithm(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindCognitive, Algorithm: "",
		DefinitionV2: &modeldefinition.Definition{},
	}
	issues := ValidateAlgorithmBinding(model)
	if !hasIssueCode(issues, "algorithm.publish.required") {
		t.Fatalf("issues = %#v, want algorithm.publish.required", issues)
	}
}

func TestValidateAlgorithmBindingAcceptsPersonalityTypology(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmPersonalityTypology,
		DefinitionV2: &modeldefinition.Definition{},
	}
	if issues := ValidateAlgorithmBinding(model); len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateBehavioralSemanticRequiresNormRefs(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind:         domain.KindBehavioralRating,
		DefinitionV2: &modeldefinition.Definition{},
	}
	issues := ValidateBehavioralSemantic(model)
	if !hasIssueCode(issues, "behavioral_rating.norm_refs.required") {
		t.Fatalf("issues = %#v, want behavioral_rating.norm_refs.required", issues)
	}
}
