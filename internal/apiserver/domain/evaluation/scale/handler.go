package scale

import (
	"context"
	"fmt"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

type EvaluateInput struct {
	Scale         *scalesnapshot.ScaleSnapshot
	AnswerSheet   *evaluationinput.AnswerSheet
	Questionnaire *evaluationinput.Questionnaire
}

type Handler struct {
	evaluator *Evaluator
}

func NewHandler(registry ScoringStrategyRegistry) *Handler {
	return &Handler{evaluator: NewEvaluator(registry)}
}

func NewDefaultHandler() *Handler {
	return &Handler{evaluator: NewDefaultEvaluator()}
}

func (h *Handler) Evaluate(ctx context.Context, input EvaluateInput) (*ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	return h.evaluator.Evaluate(ctx, assembleInterpretationInput(input))
}
