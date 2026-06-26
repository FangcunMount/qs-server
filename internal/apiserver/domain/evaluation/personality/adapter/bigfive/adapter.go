package bigfive

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Adapter implements the personality typology model adapter for Big Five.
type Adapter struct{}

func (Adapter) Algorithm() assessmentmodel.Algorithm {
	return assessmentmodel.AlgorithmBigFive
}

func (Adapter) Score(
	payload *modeltypology.Payload,
	sheet *evaluationinput.AnswerSheet,
) (evaluationtypology.ScoringResult, error) {
	detail, err := Score(payload, sheet)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}
	return evaluationtypology.ScoringResult{Detail: detail}, nil
}
