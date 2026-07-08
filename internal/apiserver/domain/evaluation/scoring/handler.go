package scoring

import (
	"context"
	"fmt"

	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

type EvaluateInput struct {
	Scale         *scalesnapshot.ScaleSnapshot
	AnswerSheet   *evaluationinput.AnswerSheet
	Questionnaire *evaluationinput.Questionnaire
}

type Handler struct {
	evaluator *calcscoring.Evaluator
}

func NewHandler(registry ScoringStrategyRegistry) *Handler {
	return &Handler{evaluator: calcscoring.NewEvaluator(registry)}
}

func NewDefaultHandler() *Handler {
	return &Handler{evaluator: calcscoring.NewDefaultEvaluator()}
}

func (h *Handler) Score(ctx context.Context, input EvaluateInput) (*ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	return h.evaluator.Score(ctx, assembleInterpretationInput(input))
}
