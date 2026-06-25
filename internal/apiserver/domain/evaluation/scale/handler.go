package scale

import (
	"context"
	"fmt"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scale"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
)

type EvaluateInput struct {
	Scale         *scalesnapshot.ScaleSnapshot
	AnswerSheet   *evaluationinput.AnswerSheet
	Questionnaire *evaluationinput.Questionnaire
}

type Handler struct {
	evaluator *scaleinterpretation.Evaluator
}

func NewHandler(registry scaleinterpretation.ScoringStrategyRegistry) *Handler {
	return &Handler{evaluator: scaleinterpretation.NewEvaluator(registry)}
}

func NewDefaultHandler() *Handler {
	return &Handler{evaluator: scaleinterpretation.NewDefaultEvaluator()}
}

func (h *Handler) Evaluate(ctx context.Context, input EvaluateInput) (*scaleinterpretation.ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	return h.evaluator.Evaluate(ctx, assembleInterpretationInput(input))
}
