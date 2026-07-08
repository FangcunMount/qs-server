package scoring

import (
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

type RiskLevel = calcscoring.RiskLevel
type ScoringStrategy = calcscoring.Strategy

const (
	RiskLevelNone   = calcscoring.RiskLevelNone
	RiskLevelLow    = calcscoring.RiskLevelLow
	RiskLevelMedium = calcscoring.RiskLevelMedium
	RiskLevelHigh   = calcscoring.RiskLevelHigh
	RiskLevelSevere = calcscoring.RiskLevelSevere

	ScoringStrategySum = calcscoring.StrategySum
	ScoringStrategyAvg = calcscoring.StrategyAvg
	ScoringStrategyCnt = calcscoring.StrategyCnt
)

type ScaleInterpretationInput = calcscoring.Input
type ScaleInterpretationModel = calcscoring.Model
type ScaleAnswerSheetSnapshot = calcscoring.AnswerSheet
type ScaleAnswerSnapshot = calcscoring.Answer
type ScaleQuestionnaireSnapshot = calcscoring.Questionnaire
type ScaleQuestionSnapshot = calcscoring.Question
type ScaleOptionSnapshot = calcscoring.Option
type ScaleInterpretationResult = calcscoring.Result
type ScaleFactorScore = calcscoring.FactorScore

type ScoringStrategyRegistry = calcscoring.StrategyRegistry
type FactorScorer = calcscoring.FactorScorer
type DefaultScoringStrategyRegistry = calcscoring.DefaultStrategyRegistry

type Evaluator = calcscoring.Evaluator

var (
	NewEvaluator        = calcscoring.NewEvaluator
	NewDefaultEvaluator = calcscoring.NewDefaultEvaluator
)

// FactorFromSnapshot adapts a published factor snapshot for calculation scoring.
func FactorFromSnapshot(factor scalesnapshot.FactorSnapshot) calcscoring.Factor {
	return factorFromSnapshot(factor)
}

func factorFromSnapshot(factor scalesnapshot.FactorSnapshot) calcscoring.Factor {
	return calcscoring.Factor{
		Code:            factor.Code,
		Title:           factor.Title,
		ScoringStrategy: factor.ScoringStrategy,
		ScoringParams: calcscoring.CntParams{
			CntOptionContents: append([]string(nil), factor.ScoringParams.CntOptionContents...),
		},
		QuestionCodes:  append([]string(nil), factor.QuestionCodes...),
		MaxScore:       factor.MaxScore,
		IsTotalScore:   factor.IsTotalScore,
		InterpretRules: interpretRulesFromSnapshot(factor.InterpretRules),
	}
}

func interpretRulesFromSnapshot(rules []scalesnapshot.InterpretRuleSnapshot) []calcscoring.InterpretRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]calcscoring.InterpretRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, calcscoring.InterpretRule{
			Min:        rule.Min,
			Max:        rule.Max,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}
	return out
}

func modelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) calcscoring.Model {
	if snapshot == nil {
		return calcscoring.Model{}
	}
	factors := make([]calcscoring.Factor, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, factorFromSnapshot(factor))
	}
	return calcscoring.Model{
		Code:                 snapshot.Code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               snapshot.Status,
		Factors:              factors,
	}
}
