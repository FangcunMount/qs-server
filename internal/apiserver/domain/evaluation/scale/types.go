package scale

import (
	"context"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ScaleInterpretationInput 是量表解释执行的纯领域输入。
type ScaleInterpretationInput struct {
	Scale         ScaleInterpretationModel
	AnswerSheet   *ScaleAnswerSheetSnapshot
	Questionnaire *ScaleQuestionnaireSnapshot
}

type ScaleInterpretationModel struct {
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []scalesnapshot.FactorSnapshot
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

type ScaleInterpretationResult struct {
	TotalScore   float64
	RiskLevel    RiskLevel
	FactorScores []ScaleFactorScore
}

type ScaleFactorScore struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	MaxScore     *float64
	RiskLevel    RiskLevel
	IsTotalScore bool
}

// ScoringStrategyRegistry 执行量表因子聚合策略。
type ScoringStrategyRegistry interface {
	ScoreFactor(ctx context.Context, factor scalesnapshot.FactorSnapshot, values []float64) (float64, error)
}
