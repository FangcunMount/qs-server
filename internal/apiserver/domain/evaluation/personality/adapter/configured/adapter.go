package configured

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/configured"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Adapter implements ModelAdapter through the configured personality evaluator.
type Adapter struct {
	algorithm assessmentmodel.Algorithm
	evaluator personalityconfigured.Evaluator
}

// NewAdapter returns a configured model adapter for the given algorithm alias.
func NewAdapter(algorithm assessmentmodel.Algorithm) Adapter {
	return NewAdapterWithEvaluator(algorithm, personalityconfigured.NewEvaluator())
}

// NewAdapterWithEvaluator returns a configured model adapter bound to a specific evaluator.
func NewAdapterWithEvaluator(algorithm assessmentmodel.Algorithm, evaluator personalityconfigured.Evaluator) Adapter {
	return Adapter{
		algorithm: algorithm,
		evaluator: evaluator,
	}
}

func (a Adapter) Algorithm() assessmentmodel.Algorithm {
	return a.algorithm
}

// NewRuntimeAdapter returns a configured adapter that routes purely by payload runtime spec.
func NewRuntimeAdapter() Adapter {
	return NewRuntimeAdapterWithEvaluator(personalityconfigured.NewEvaluator())
}

// NewRuntimeAdapterWithEvaluator returns a runtime adapter bound to a specific evaluator.
func NewRuntimeAdapterWithEvaluator(evaluator personalityconfigured.Evaluator) Adapter {
	return Adapter{evaluator: evaluator}
}

func (a Adapter) Score(
	payload *modeltypology.Payload,
	sheet *evaluationinput.AnswerSheet,
) (evaluationtypology.ScoringResult, error) {
	if payload == nil {
		return evaluationtypology.ScoringResult{}, fmt.Errorf("typology payload is required")
	}
	if a.algorithm != "" && payload.Algorithm != a.algorithm {
		return evaluationtypology.ScoringResult{}, fmt.Errorf(
			"typology algorithm %s does not match adapter %s",
			payload.Algorithm,
			a.algorithm,
		)
	}
	return a.evaluator.Score(payload, sheet)
}
