package scale

import (
	"context"
	"fmt"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
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

// Score 执行量表计分与风险分级，不包含解读文案。
func (h *Handler) Score(ctx context.Context, input EvaluateInput) (*ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	interpInput := assembleInterpretationInput(input)
	factorScores, totalScore, riskLevel, err := h.evaluator.runScoring(ctx, interpInput)
	if err != nil {
		return nil, err
	}
	return &ScaleInterpretationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		FactorScores: factorScores,
	}, nil
}

func (h *Handler) Evaluate(ctx context.Context, input EvaluateInput) (*ScaleInterpretationResult, error) {
	if h == nil || h.evaluator == nil {
		return nil, fmt.Errorf("scale handler is not configured")
	}
	return h.evaluator.Evaluate(ctx, assembleInterpretationInput(input))
}
