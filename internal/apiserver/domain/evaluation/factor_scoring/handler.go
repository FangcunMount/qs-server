package factor_scoring

import (
	"context"
	"fmt"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
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

// Score 执行量表计分与风险分级，不生成解读文案。
func (h *Handler) Score(ctx context.Context, input EvaluateInput) (*ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	return h.evaluator.Score(ctx, assembleInterpretationInput(input))
}
