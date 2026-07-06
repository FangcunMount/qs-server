package bigfive

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Adapter implements the personality typology model adapter for Big Five.
type Adapter struct{}

func (Adapter) Algorithm() modelcatalog.Algorithm {
	return modelcatalog.AlgorithmBigFive
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
