package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// OutcomeAssembler maps scoring results to assessment outcomes using outcome mapping spec.
type OutcomeAssembler struct {
	registry OutcomeAdapterRegistry
}

// NewOutcomeAssembler returns the default typology outcome assembler.
func NewOutcomeAssembler() OutcomeAssembler {
	return NewOutcomeAssemblerWithRegistry(DefaultOutcomeAdapterRegistry())
}

// NewOutcomeAssemblerWithRegistry returns an outcome assembler bound to a specific adapter registry.
func NewOutcomeAssemblerWithRegistry(registry OutcomeAdapterRegistry) OutcomeAssembler {
	return OutcomeAssembler{registry: registry}
}

// Assemble converts a scoring result into an AssessmentOutcome.
func (a OutcomeAssembler) Assemble(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
	mapping modeltypology.OutcomeMappingSpec,
) (*assessment.AssessmentOutcome, error) {
	adapterKey := mapping.ResolvedDetailAdapterKey(decisionKindFromResult(result))
	return a.registry.Assemble(adapterKey, modelRef, result)
}

func decisionKindFromResult(result evaluationtypology.ScoringResult) modelcatalog.DecisionKind {
	if result.Runtime != nil {
		return result.Runtime.Decision.Kind
	}
	return ""
}

func assembleGenericTraitProfileOutcome(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.TraitProfileDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromTraitProfile(modelRef, detail), nil
}

func assembleGenericPersonalityTypeOutcome(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.PersonalityTypeDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromPersonalityType(modelRef, detail), nil
}

// AssembleFromPayload derives mapping from payload and assembles the outcome.
func (a OutcomeAssembler) AssembleFromPayload(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	if payload == nil {
		return nil, fmt.Errorf("typology payload is required")
	}
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		return nil, err
	}
	return a.Assemble(modelRef, result, spec.OutcomeMapping)
}
