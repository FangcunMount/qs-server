package evaluation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ScaleEvaluationInput 是量表解释执行的纯领域输入。
type ScaleEvaluationInput struct {
	Scale         ScaleEvaluationModel
	AnswerSheet   *ScaleAnswerSheetSnapshot
	Questionnaire *ScaleQuestionnaireSnapshot
}

type ScaleEvaluationModel struct {
	Code                 string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               scale.Status
	Factors              []scale.FactorSnapshot
}

type ScaleAnswerSheetSnapshot struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	Answers              []ScaleAnswerSnapshot
}

type ScaleAnswerSnapshot struct {
	QuestionCode meta.Code
	Score        float64
	Value        any
}

type ScaleQuestionnaireSnapshot struct {
	Code      string
	Version   string
	Questions []ScaleQuestionSnapshot
}

type ScaleQuestionSnapshot struct {
	Code    meta.Code
	Options []ScaleOptionSnapshot
}

type ScaleOptionSnapshot struct {
	Code    string
	Content string
	Score   float64
}

type ScaleEvaluationResult struct {
	TotalScore   float64
	RiskLevel    scale.RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []ScaleFactorScore
}

type ScaleFactorScore struct {
	FactorCode   scale.FactorCode
	FactorName   string
	RawScore     float64
	MaxScore     *float64
	RiskLevel    scale.RiskLevel
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}

// ScoringStrategyRegistry 执行量表因子聚合策略。
type ScoringStrategyRegistry interface {
	ScoreFactor(ctx context.Context, factor scale.FactorSnapshot, values []float64) (float64, error)
}
