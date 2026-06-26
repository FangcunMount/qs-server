package typology

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

type outcomeAdapterFunc func(assessment.EvaluationModelRef, evaluationtypology.ScoringResult) (*assessment.AssessmentOutcome, error)

// OutcomeAdapterRegistry resolves assessment outcome assemblers by detail adapter key.
type OutcomeAdapterRegistry struct {
	adapters map[modeltypology.DetailAdapterKey]outcomeAdapterFunc
}

// DefaultOutcomeAdapterRegistry returns the built-in generic and legacy outcome adapters.
func DefaultOutcomeAdapterRegistry() OutcomeAdapterRegistry {
	return OutcomeAdapterRegistry{
		adapters: map[modeltypology.DetailAdapterKey]outcomeAdapterFunc{
			modeltypology.DetailAdapterPersonalityType: assembleGenericPersonalityTypeOutcome,
			modeltypology.DetailAdapterTraitProfile:    assembleGenericTraitProfileOutcome,
			modeltypology.DetailAdapterMBTI:            assemblePersonalityTypeFromMBTI,
			modeltypology.DetailAdapterSBTI:            assemblePersonalityTypeFromSBTI,
			modeltypology.DetailAdapterBigFive:         assembleTraitProfileOutcome,
		},
	}
}

func (r OutcomeAdapterRegistry) Assemble(
	key modeltypology.DetailAdapterKey,
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	if key == "" {
		return nil, fmt.Errorf("detail adapter key is required")
	}
	adapter, ok := r.adapters[key]
	if !ok {
		return nil, fmt.Errorf("unsupported detail adapter key: %s", key)
	}
	return adapter(modelRef, result)
}

func (r OutcomeAdapterRegistry) Len() int {
	return len(r.adapters)
}
