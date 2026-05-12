package evaluation

import (
	"context"
	"fmt"

	evaluationengine "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Executor adapts the Scale interpretation model to the generic Evaluation executor contract.
type Executor struct {
	evaluator *domainScale.Evaluator
}

func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutorWithEvaluator(domainScale.NewEvaluator(scaleScoringRegistry{scorer: scorer}))
}

func NewExecutorWithEvaluator(evaluator *domainScale.Evaluator) *Executor {
	if evaluator == nil {
		evaluator = domainScale.NewDefaultEvaluator()
	}
	return &Executor{evaluator: evaluator}
}

func (e *Executor) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

func (e *Executor) Execute(ctx context.Context, input evaluationengine.ExecutionInput) (*assessment.EvaluationResult, error) {
	if err := validateScaleExecutionInput(input); err != nil {
		return nil, err
	}
	return e.EvaluateScale(ctx, input.Assessment, input.Input)
}

func (e *Executor) EvaluateScale(ctx context.Context, a *assessment.Assessment, snapshot *evaluationinput.InputSnapshot) (*assessment.EvaluationResult, error) {
	evaluator := e.evaluator
	if evaluator == nil {
		evaluator = domainScale.NewDefaultEvaluator()
	}
	result, err := evaluator.Evaluate(ctx, InputFromSnapshot(snapshot))
	if err != nil {
		return nil, err
	}
	return ConvertScaleResult(result, a, snapshot), nil
}

func InputFromSnapshot(snapshot *evaluationinput.InputSnapshot) domainScale.ScaleEvaluationInput {
	if snapshot == nil {
		return domainScale.ScaleEvaluationInput{}
	}
	return domainScale.ScaleEvaluationInput{
		Scale:         modelFromSnapshot(snapshot.MedicalScale),
		AnswerSheet:   answerSheetFromSnapshot(snapshot.AnswerSheet),
		Questionnaire: questionnaireFromSnapshot(snapshot.Questionnaire),
	}
}

func ConvertScaleResult(result *domainScale.ScaleEvaluationResult, a *assessment.Assessment, snapshot *evaluationinput.InputSnapshot) *assessment.EvaluationResult {
	if result == nil {
		return nil
	}
	factorScores := make([]assessment.FactorScoreResult, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorScores = append(factorScores, assessment.NewFactorScoreResult(
			assessment.NewFactorCode(string(fs.FactorCode)),
			fs.FactorName,
			fs.RawScore,
			assessment.RiskLevel(fs.RiskLevel),
			fs.Conclusion,
			fs.Suggestion,
			fs.IsTotalScore,
		))
	}
	evalResult := assessment.NewEvaluationResult(
		result.TotalScore,
		assessment.RiskLevel(result.RiskLevel),
		result.Conclusion,
		result.Suggestion,
		factorScores,
	)
	if a != nil && a.EvaluationModelRef() != nil {
		evalResult.WithModelRef(*a.EvaluationModelRef())
	} else if snapshot != nil && snapshot.Model != nil {
		evalResult.WithModelRef(assessment.NewEvaluationModelRefByCode(
			assessment.EvaluationModelKind(snapshot.Model.Kind),
			meta.NewCode(snapshot.Model.Code),
			snapshot.Model.Version,
			snapshot.Model.Title,
		))
	}
	return evalResult
}

func validateScaleExecutionInput(input evaluationengine.ExecutionInput) error {
	if input.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if !input.Assessment.Status().IsSubmitted() {
		return fmt.Errorf("assessment is not submitted")
	}
	if input.Input == nil {
		return fmt.Errorf("evaluation input snapshot is required")
	}
	scale := input.Input.MedicalScale
	if scale == nil {
		return fmt.Errorf("medical scale is required")
	}
	if len(scale.Factors) == 0 {
		return fmt.Errorf("medical scale has no factors")
	}
	if !scale.IsPublished() {
		return fmt.Errorf("medical scale is not published")
	}
	if scale.QuestionnaireCode != input.Assessment.QuestionnaireRef().Code().String() {
		return fmt.Errorf("medical scale does not match the questionnaire")
	}
	if input.Input.AnswerSheet == nil {
		return fmt.Errorf("answer sheet not found")
	}
	return nil
}

type scaleScoringRegistry struct {
	scorer ruleengine.ScaleFactorScorer
}

func (r scaleScoringRegistry) ScoreFactor(ctx context.Context, factor domainScale.FactorSnapshot, values []float64) (float64, error) {
	if r.scorer == nil {
		return domainScale.DefaultScoringStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, string(factor.Code), values, string(factor.ScoringStrategy), nil)
}

func modelFromSnapshot(snapshot *evaluationinput.ScaleSnapshot) domainScale.ScaleEvaluationModel {
	if snapshot == nil {
		return domainScale.ScaleEvaluationModel{}
	}
	factors := make([]domainScale.FactorSnapshot, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, factorFromSnapshot(factor))
	}
	return domainScale.ScaleEvaluationModel{
		Code:                 snapshot.Code,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               domainScale.Status(snapshot.Status),
		Factors:              factors,
	}
}

func factorFromSnapshot(snapshot evaluationinput.FactorSnapshot) domainScale.FactorSnapshot {
	questionCodes := make([]meta.Code, 0, len(snapshot.QuestionCodes))
	for _, code := range snapshot.QuestionCodes {
		questionCodes = append(questionCodes, meta.NewCode(code))
	}
	rules := make([]domainScale.InterpretationRule, 0, len(snapshot.InterpretRules))
	for _, rule := range snapshot.InterpretRules {
		rules = append(rules, domainScale.NewInterpretationRule(
			domainScale.NewScoreRange(rule.Min, rule.Max),
			domainScale.RiskLevel(rule.RiskLevel),
			rule.Conclusion,
			rule.Suggestion,
		))
	}
	return domainScale.FactorSnapshot{
		Code:            domainScale.NewFactorCode(snapshot.Code),
		Title:           snapshot.Title,
		IsTotalScore:    snapshot.IsTotalScore,
		QuestionCodes:   questionCodes,
		ScoringStrategy: domainScale.ScoringStrategyCode(snapshot.ScoringStrategy),
		ScoringParams:   domainScale.NewScoringParams().WithCntOptionContents(snapshot.ScoringParams.CntOptionContents),
		MaxScore:        cloneFloat64Ptr(snapshot.MaxScore),
		InterpretRules:  rules,
	}
}

func answerSheetFromSnapshot(snapshot *evaluationinput.AnswerSheetSnapshot) *domainScale.ScaleAnswerSheetSnapshot {
	if snapshot == nil {
		return nil
	}
	answers := make([]domainScale.ScaleAnswerSnapshot, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, domainScale.ScaleAnswerSnapshot{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &domainScale.ScaleAnswerSheetSnapshot{
		ID:                   snapshot.ID,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func questionnaireFromSnapshot(snapshot *evaluationinput.QuestionnaireSnapshot) *domainScale.ScaleQuestionnaireSnapshot {
	if snapshot == nil {
		return nil
	}
	questions := make([]domainScale.ScaleQuestionSnapshot, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]domainScale.ScaleOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, domainScale.ScaleOptionSnapshot{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, domainScale.ScaleQuestionSnapshot{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &domainScale.ScaleQuestionnaireSnapshot{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Questions: questions,
	}
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

var _ evaluationengine.Evaluator = (*Executor)(nil)
