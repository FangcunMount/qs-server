package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type MBTIModelInputProvider struct {
	TypologyModelInputProvider
}

// NewMBTIModelInputProvider is a compatibility wrapper around TypologyModelInputProvider.
// Prefer MaterializeInputProviders with RuleSetTypologyCatalog for new wiring.
func NewMBTIModelInputProvider(
	catalog port.TypologyModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) MBTIModelInputProvider {
	return MBTIModelInputProvider{
		TypologyModelInputProvider: NewTypologyModelInputProvider(
			assessmentmodel.AlgorithmMBTI,
			catalog,
			answerSheetReader,
			questionnaireReader,
		),
	}
}

func (MBTIModelInputProvider) EvaluatorKey() evaldomain.EvaluatorKey {
	return evaldomain.EvaluatorKeyMBTI
}
