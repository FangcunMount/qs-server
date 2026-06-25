package evaluation

import (
	"context"
	"fmt"

	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/interpretation"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
)

type ScaleEvaluateInput struct {
	Scale         *rulesetscale.ScaleSnapshot
	AnswerSheet   *AnswerSheet
	Questionnaire *Questionnaire
}

type ScaleHandler struct {
	evaluator *scaleinterpretation.Evaluator
}

func NewScaleHandler(registry scaleinterpretation.ScoringStrategyRegistry) *ScaleHandler {
	return &ScaleHandler{evaluator: scaleinterpretation.NewEvaluator(registry)}
}

func NewDefaultScaleHandler() *ScaleHandler {
	return &ScaleHandler{evaluator: scaleinterpretation.NewDefaultEvaluator()}
}

func (h *ScaleHandler) Evaluate(ctx context.Context, input ScaleEvaluateInput) (*scaleinterpretation.ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	return h.evaluator.Evaluate(ctx, assembleScaleInterpretationInput(input))
}
