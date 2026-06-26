package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// OutcomeAssembler maps scoring results to assessment outcomes using outcome mapping spec.
type OutcomeAssembler struct{}

// NewOutcomeAssembler returns the default typology outcome assembler.
func NewOutcomeAssembler() OutcomeAssembler {
	return OutcomeAssembler{}
}

// Assemble converts a scoring result into an AssessmentOutcome.
func (OutcomeAssembler) Assemble(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
	mapping modeltypology.OutcomeMappingSpec,
) (*assessment.AssessmentOutcome, error) {
	adapterKey := mapping.ResolvedDetailAdapterKey(decisionKindFromResult(result))
	switch adapterKey {
	case modeltypology.DetailAdapterBigFive:
		return assembleTraitProfileOutcome(modelRef, result)
	case modeltypology.DetailAdapterSBTI:
		return assemblePersonalityTypeFromSBTI(modelRef, result)
	default:
		return assemblePersonalityTypeFromMBTI(modelRef, result)
	}
}

func decisionKindFromResult(result evaluationtypology.ScoringResult) assessmentmodel.DecisionKind {
	if result.Runtime != nil {
		return result.Runtime.Decision.Kind
	}
	return ""
}

func assembleTraitProfileOutcome(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.BigFiveResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromBigFive(modelRef, detail), nil
}

func assemblePersonalityTypeFromMBTI(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromMBTI(modelRef, detail), nil
}

func assemblePersonalityTypeFromSBTI(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.SBTIResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromSBTI(modelRef, detail), nil
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
