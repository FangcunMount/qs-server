package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type SBTIModelInputProvider struct {
	TypologyModelInputProvider
}

// NewSBTIModelInputProvider is a compatibility wrapper around TypologyModelInputProvider.
// Prefer MaterializeInputProviders with RuleSetTypologyCatalog for new wiring.
func NewSBTIModelInputProvider(
	catalog port.TypologyModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) SBTIModelInputProvider {
	return SBTIModelInputProvider{
		TypologyModelInputProvider: NewTypologyModelInputProvider(
			assessmentmodel.AlgorithmSBTI,
			catalog,
			answerSheetReader,
			questionnaireReader,
		),
	}
}

func (SBTIModelInputProvider) EvaluatorKey() evaldomain.EvaluatorKey {
	return evaldomain.EvaluatorKeySBTI
}
