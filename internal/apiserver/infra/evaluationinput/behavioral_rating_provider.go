package evaluationinput

import (
	"context"
	"fmt"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type BehavioralRatingModelInputProvider struct {
	catalog             port.BehavioralRatingModelCatalog
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

func NewBehavioralRatingModelInputProvider(
	catalog port.BehavioralRatingModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) BehavioralRatingModelInputProvider {
	return BehavioralRatingModelInputProvider{
		catalog:             catalog,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (BehavioralRatingModelInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityBehavioralRatingDefault
}

// EvaluatorKey is deprecated; use ExecutionIdentity.
func (BehavioralRatingModelInputProvider) EvaluatorKey() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityBehavioralRatingDefault
}

func (BehavioralRatingModelInputProvider) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathBehavioralRatingDescriptor
}

func (p BehavioralRatingModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("behavioral_rating catalog is not configured"), "解释模型不存在", "加载解释模型失败")
	}
	model, err := p.catalog.GetBehavioralRatingByRef(ctx, ref.ModelRef)
	if err != nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "解释模型不存在", "加载解释模型失败")
	}
	answerSheet, err := p.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	qnr, err := p.questionnaireReader.GetQuestionnaire(ctx, answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}
	payload := port.BehavioralRatingModelPayload{Snapshot: model}
	return &port.InputSnapshot{
		Model:         port.NewBehavioralRatingModelSnapshot(model),
		ModelPayload:  payload,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}
