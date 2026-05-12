package scale

import (
	"context"
	"fmt"
	"slices"

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
	Status               Status
	Factors              []FactorSnapshot
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
	RiskLevel    RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []ScaleFactorScore
}

type ScaleFactorScore struct {
	FactorCode   FactorCode
	FactorName   string
	RawScore     float64
	MaxScore     *float64
	RiskLevel    RiskLevel
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}

// ScoringStrategyRegistry 执行量表因子聚合策略。
type ScoringStrategyRegistry interface {
	ScoreFactor(ctx context.Context, factor FactorSnapshot, values []float64) (float64, error)
}

type Evaluator struct {
	scoringRegistry ScoringStrategyRegistry
}

func NewEvaluator(scoringRegistry ScoringStrategyRegistry) *Evaluator {
	return &Evaluator{scoringRegistry: scoringRegistry}
}

func NewDefaultEvaluator() *Evaluator {
	return NewEvaluator(DefaultScoringStrategyRegistry{})
}

func (e *Evaluator) Evaluate(ctx context.Context, input ScaleEvaluationInput) (*ScaleEvaluationResult, error) {
	factorScores, totalScore := e.CalculateScores(ctx, input)
	factorScores, riskLevel := e.ClassifyRisk(input.Scale, factorScores)
	factorScores, conclusion, suggestion := e.Interpret(input.Scale, factorScores, totalScore, riskLevel)
	return &ScaleEvaluationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	}, nil
}

func (e *Evaluator) CalculateScores(ctx context.Context, input ScaleEvaluationInput) ([]ScaleFactorScore, float64) {
	factorScores := make([]ScaleFactorScore, 0, len(input.Scale.Factors))
	for _, factor := range input.Scale.Factors {
		rawScore := e.calculateFactorRawScore(ctx, factor, input.AnswerSheet, input.Questionnaire)
		factorScores = append(factorScores, ScaleFactorScore{
			FactorCode:   factor.Code,
			FactorName:   factor.Title,
			RawScore:     rawScore,
			MaxScore:     cloneEvaluationFloat64Ptr(factor.MaxScore),
			RiskLevel:    RiskLevelNone,
			IsTotalScore: factor.IsTotalScore,
		})
	}
	return factorScores, CalculateTotalScore(factorScores)
}

func (e *Evaluator) ClassifyRisk(model ScaleEvaluationModel, factorScores []ScaleFactorScore) ([]ScaleFactorScore, RiskLevel) {
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		fs.RiskLevel = calculateFactorRiskLevel(model, fs.FactorCode, fs.RawScore)
		updatedScores = append(updatedScores, fs)
	}
	return updatedScores, calculateOverallRiskLevel(model, updatedScores)
}

func (e *Evaluator) Interpret(model ScaleEvaluationModel, factorScores []ScaleFactorScore, totalScore float64, riskLevel RiskLevel) ([]ScaleFactorScore, string, string) {
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		fs.Conclusion, fs.Suggestion = interpretFactor(model, fs)
		updatedScores = append(updatedScores, fs)
	}
	conclusion, suggestion := interpretOverall(model, updatedScores, totalScore, riskLevel)
	return updatedScores, conclusion, suggestion
}

func (e *Evaluator) calculateFactorRawScore(ctx context.Context, factor FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) float64 {
	if sheet == nil {
		return simulateFactorScore(factor)
	}
	if e == nil || e.scoringRegistry == nil {
		return 0
	}
	values, err := collectFactorValues(factor, sheet, qnr)
	if err != nil {
		return 0
	}
	score, err := e.scoringRegistry.ScoreFactor(ctx, factor, values)
	if err != nil {
		return 0
	}
	return score
}

func CalculateTotalScore(factorScores []ScaleFactorScore) float64 {
	var totalScore float64
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
	}
	return totalScore
}

func collectFactorValues(factor FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) ([]float64, error) {
	switch factor.ScoringStrategy {
	case ScoringStrategySum, ScoringStrategyAvg:
		return collectQuestionScores(factor, sheet), nil
	case ScoringStrategyCnt:
		if qnr == nil {
			return nil, fmt.Errorf("questionnaire is required")
		}
		return collectCntMatches(factor, sheet, qnr), nil
	default:
		return nil, nil
	}
}

func collectQuestionScores(factor FactorSnapshot, sheet *ScaleAnswerSheetSnapshot) []float64 {
	answerMap := factorScoreAnswerMap(sheet)
	scores := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		if answer, found := answerMap[qCode.String()]; found {
			scores = append(scores, answer.Score)
		}
	}
	return scores
}

func collectCntMatches(factor FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) []float64 {
	targetContents := factor.ScoringParams.GetCntOptionContents()
	if len(targetContents) == 0 {
		return nil
	}
	optionContentMap := factorScoreOptionContentMap(qnr)
	answerMap := factorScoreAnswerMap(sheet)
	matchValues := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		answer, found := answerMap[qCode.String()]
		if !found {
			continue
		}
		optionID := factorScoreOptionID(answer)
		if optionID == "" {
			continue
		}
		optionContent, found := optionContentMap[optionID]
		if !found {
			continue
		}
		if slices.Contains(targetContents, optionContent) {
			matchValues = append(matchValues, 1.0)
		}
	}
	return matchValues
}

func simulateFactorScore(factor FactorSnapshot) float64 {
	questionCount := len(factor.QuestionCodes)
	if questionCount == 0 {
		return 50.0
	}
	return float64(questionCount) * 2.5
}

func factorScoreOptionContentMap(qnr *ScaleQuestionnaireSnapshot) map[string]string {
	contentMap := make(map[string]string)
	if qnr == nil {
		return contentMap
	}
	for _, q := range qnr.Questions {
		for _, opt := range q.Options {
			contentMap[opt.Code] = opt.Content
		}
	}
	return contentMap
}

func factorScoreAnswerMap(sheet *ScaleAnswerSheetSnapshot) map[string]ScaleAnswerSnapshot {
	answerMap := make(map[string]ScaleAnswerSnapshot)
	if sheet == nil {
		return answerMap
	}
	for _, ans := range sheet.Answers {
		answerMap[ans.QuestionCode.String()] = ans
	}
	return answerMap
}

func factorScoreOptionID(answer ScaleAnswerSnapshot) string {
	raw := answer.Value
	if raw == nil {
		return ""
	}
	if str, ok := raw.(string); ok {
		return str
	}
	if arr, ok := raw.([]string); ok && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

func calculateFactorRiskLevel(model ScaleEvaluationModel, factorCode FactorCode, score float64) RiskLevel {
	if factor, found := findFactor(model, factorCode); found {
		if rule := findInterpretRule(factor, score); rule != nil {
			return rule.GetRiskLevel()
		}
	}
	return defaultRiskLevelByScore(score)
}

func calculateOverallRiskLevel(model ScaleEvaluationModel, factorScores []ScaleFactorScore) RiskLevel {
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			if factor, found := findFactor(model, fs.FactorCode); found {
				if rule := findInterpretRule(factor, fs.RawScore); rule != nil {
					return rule.GetRiskLevel()
				}
			}
		}
	}

	maxRisk := RiskLevelNone
	for _, fs := range factorScores {
		if riskLevelOrder(fs.RiskLevel) > riskLevelOrder(maxRisk) {
			maxRisk = fs.RiskLevel
		}
	}
	return maxRisk
}

func interpretFactor(model ScaleEvaluationModel, fs ScaleFactorScore) (string, string) {
	if factor, found := findFactor(model, fs.FactorCode); found {
		if rule := findInterpretRuleWithRangeFallback(factor, fs.RawScore); rule != nil && rule.GetConclusion() != "" {
			return rule.GetConclusion(), rule.GetSuggestion()
		}
	}
	return defaultFactorInterpretation(fs.FactorName, fs.RiskLevel, fs.RawScore)
}

func interpretOverall(model ScaleEvaluationModel, factorScores []ScaleFactorScore, totalScore float64, riskLevel RiskLevel) (string, string) {
	for _, fs := range factorScores {
		if !fs.IsTotalScore {
			continue
		}
		if factor, found := findFactor(model, fs.FactorCode); found {
			if rule := findInterpretRule(factor, fs.RawScore); rule != nil && rule.GetConclusion() != "" {
				return rule.GetConclusion(), rule.GetSuggestion()
			}
		}
	}
	return defaultOverallInterpretation(totalScore, riskLevel)
}

func findFactor(model ScaleEvaluationModel, factorCode FactorCode) (FactorSnapshot, bool) {
	for _, factor := range model.Factors {
		if factor.Code == factorCode {
			return factor, true
		}
	}
	return FactorSnapshot{}, false
}

func findInterpretRule(factor FactorSnapshot, score float64) *InterpretationRule {
	for i := range factor.InterpretRules {
		if factor.InterpretRules[i].Matches(score) {
			return &factor.InterpretRules[i]
		}
	}
	return nil
}

func findInterpretRuleWithRangeFallback(factor FactorSnapshot, score float64) *InterpretationRule {
	if rule := findInterpretRule(factor, score); rule != nil {
		return rule
	}
	if len(factor.InterpretRules) == 0 {
		return nil
	}
	return &factor.InterpretRules[len(factor.InterpretRules)-1]
}

func defaultRiskLevelByScore(score float64) RiskLevel {
	switch {
	case score >= 80:
		return RiskLevelSevere
	case score >= 60:
		return RiskLevelHigh
	case score >= 40:
		return RiskLevelMedium
	case score >= 20:
		return RiskLevelLow
	default:
		return RiskLevelNone
	}
}

func riskLevelOrder(level RiskLevel) int {
	switch level {
	case RiskLevelNone:
		return 0
	case RiskLevelLow:
		return 1
	case RiskLevelMedium:
		return 2
	case RiskLevelHigh:
		return 3
	case RiskLevelSevere:
		return 4
	default:
		return 0
	}
}

func defaultFactorInterpretation(factorName string, riskLevel RiskLevel, score float64) (string, string) {
	switch riskLevel {
	case RiskLevelSevere:
		return fmt.Sprintf("%s得分%.1f分，处于严重异常水平", factorName, score), "建议立即寻求专业帮助，进行进一步评估"
	case RiskLevelHigh:
		return fmt.Sprintf("%s得分%.1f分，处于较高风险水平", factorName, score), "建议尽快咨询专业人员，了解更多信息"
	case RiskLevelMedium:
		return fmt.Sprintf("%s得分%.1f分，处于中等水平", factorName, score), "建议关注相关方面，适当调整生活方式"
	case RiskLevelLow:
		return fmt.Sprintf("%s得分%.1f分，处于正常偏低水平", factorName, score), "整体情况良好，保持当前状态"
	default:
		return fmt.Sprintf("%s得分%.1f分，处于正常水平", factorName, score), "状态良好，继续保持"
	}
}

func defaultOverallInterpretation(totalScore float64, riskLevel RiskLevel) (string, string) {
	switch riskLevel {
	case RiskLevelSevere:
		return "测评结果显示存在严重问题，需要立即关注", "强烈建议尽快寻求专业帮助，进行全面评估和干预"
	case RiskLevelHigh:
		return "测评结果显示存在较高风险，需要重点关注", "建议尽快咨询专业人员，获取更详细的评估和指导"
	case RiskLevelMedium:
		return "测评结果显示存在一定风险，需要适度关注", "建议关注相关方面的变化，必要时寻求专业帮助"
	case RiskLevelLow:
		return "测评结果显示整体情况良好，少数方面需要注意", "保持健康的生活方式，定期进行自我检查"
	default:
		return "测评已完成，整体情况良好", "保持健康的生活方式"
	}
}

func cloneEvaluationFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

type DefaultScoringStrategyRegistry struct{}

func (DefaultScoringStrategyRegistry) ScoreFactor(_ context.Context, factor FactorSnapshot, values []float64) (float64, error) {
	switch factor.ScoringStrategy {
	case ScoringStrategySum:
		return sumValues(values), nil
	case ScoringStrategyAvg:
		if len(values) == 0 {
			return 0, nil
		}
		return sumValues(values) / float64(len(values)), nil
	case ScoringStrategyCnt:
		return float64(len(values)), nil
	default:
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
}

func sumValues(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}
