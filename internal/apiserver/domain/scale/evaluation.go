package scale

import (
	"context"
	"fmt"
	"slices"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
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

// Evaluator 执行量表解释模型评估。
type Evaluator struct {
	scoringRegistry ScoringStrategyRegistry
	calculator      *calculation.Engine
}

// NewEvaluator 创建量表解释模型评估器。
func NewEvaluator(scoringRegistry ScoringStrategyRegistry) *Evaluator {
	if scoringRegistry == nil {
		scoringRegistry = DefaultScoringStrategyRegistry{}
	}
	return &Evaluator{
		scoringRegistry: scoringRegistry,
		calculator:      calculation.NewEngine(scaleCalculationRegistry{registry: scoringRegistry}),
	}
}

// NewDefaultEvaluator 创建默认量表解释模型评估器。
func NewDefaultEvaluator() *Evaluator {
	return NewEvaluator(DefaultScoringStrategyRegistry{})
}

// Evaluate 执行量表解释模型评估。
func (e *Evaluator) Evaluate(ctx context.Context, input ScaleEvaluationInput) (*ScaleEvaluationResult, error) {
	// 计算量表因子得分。
	factorScores, totalScore := e.calculateScores(ctx, input)
	// 分类量表因子风险等级。
	factorScores, riskLevel := e.classifyRisk(input.Scale, factorScores)
	// 解读量表因子。
	factorScores, conclusion, suggestion := e.interpret(input.Scale, factorScores, totalScore, riskLevel)

	// 返回量表解释模型评估结果。
	return &ScaleEvaluationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	}, nil
}

// calculateScores 计算量表因子得分。
func (e *Evaluator) calculateScores(ctx context.Context, input ScaleEvaluationInput) ([]ScaleFactorScore, float64) {
	// 创建量表因子得分列表。
	factorScores := make([]ScaleFactorScore, 0, len(input.Scale.Factors))
	// 计算量表因子得分。
	for _, factor := range input.Scale.Factors {
		// 计算因子得分。
		rawScore := e.calculateFactorRawScore(ctx, factor, input.AnswerSheet, input.Questionnaire)
		// 创建量表因子得分。
		factorScores = append(factorScores, ScaleFactorScore{
			FactorCode:   factor.Code,
			FactorName:   factor.Title,
			RawScore:     rawScore,
			MaxScore:     cloneEvaluationFloat64Ptr(factor.MaxScore),
			RiskLevel:    RiskLevelNone,
			IsTotalScore: factor.IsTotalScore,
		})
	}

	// 计算总分。
	return factorScores, calculateTotalScore(factorScores)
}

// classifyRisk 分类量表因子风险等级。
func (e *Evaluator) classifyRisk(model ScaleEvaluationModel, factorScores []ScaleFactorScore) ([]ScaleFactorScore, RiskLevel) {
	// 创建量表因子得分列表。
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		// 计算因子风险等级。
		fs.RiskLevel = calculateFactorRiskLevel(model, fs.FactorCode, fs.RawScore)
		// 添加到量表因子得分列表。
		updatedScores = append(updatedScores, fs)
	}
	// 计算总体风险等级。
	return updatedScores, calculateOverallRiskLevel(model, updatedScores)
}

// interpret 解读量表因子。
func (e *Evaluator) interpret(model ScaleEvaluationModel, factorScores []ScaleFactorScore, totalScore float64, riskLevel RiskLevel) ([]ScaleFactorScore, string, string) {
	// 创建量表因子得分列表。
	updatedScores := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		// 解读因子。
		fs.Conclusion, fs.Suggestion = interpretFactor(model, fs)
		// 添加到量表因子得分列表。
		updatedScores = append(updatedScores, fs)
	}
	// 解读总体。
	conclusion, suggestion := interpretOverall(model, updatedScores, totalScore, riskLevel)
	// 返回量表因子得分列表、总结论和建议。
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
	score, err := e.calculator.ScoreDimension(ctx, calculation.Dimension{
		Code:            factor.Code.String(),
		ScoringStrategy: string(factor.ScoringStrategy),
	}, values)
	if e.calculator == nil {
		score, err = e.scoringRegistry.ScoreFactor(ctx, factor, values)
	}
	if err != nil {
		return 0
	}
	return score
}

// calculateTotalScore 计算量表总分。
func calculateTotalScore(factorScores []ScaleFactorScore) float64 {
	var totalScore float64
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
	}
	return totalScore
}

// collectFactorValues 收集因子得分。
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

// collectQuestionScores 收集题目得分。
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

// collectCntMatches 收集匹配的题目得分。
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

// simulateFactorScore 模拟因子得分。
func simulateFactorScore(factor FactorSnapshot) float64 {
	questionCount := len(factor.QuestionCodes)
	if questionCount == 0 {
		return 50.0
	}
	return float64(questionCount) * 2.5
}

// factorScoreOptionContentMap 收集选项内容。
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

// factorScoreAnswerMap 收集答案。
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

// factorScoreOptionID 收集选项ID。
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

// calculateFactorRiskLevel 计算因子风险等级。
func calculateFactorRiskLevel(model ScaleEvaluationModel, factorCode FactorCode, score float64) RiskLevel {
	if factor, found := findFactor(model, factorCode); found {
		if rule := findInterpretRule(factor, score); rule != nil {
			return rule.GetRiskLevel()
		}
	}
	return defaultRiskLevelByScore(score)
}

// calculateOverallRiskLevel 计算总体风险等级。
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

// interpretFactor 解读因子。
func interpretFactor(model ScaleEvaluationModel, fs ScaleFactorScore) (string, string) {
	if factor, found := findFactor(model, fs.FactorCode); found {
		if rule := findInterpretRuleWithRangeFallback(factor, fs.RawScore); rule != nil && rule.GetConclusion() != "" {
			return rule.GetConclusion(), rule.GetSuggestion()
		}
	}
	return defaultFactorInterpretation(fs.FactorName, fs.RiskLevel, fs.RawScore)
}

// interpretOverall 解读总体。
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

// findFactor 查找因子。
func findFactor(model ScaleEvaluationModel, factorCode FactorCode) (FactorSnapshot, bool) {
	for _, factor := range model.Factors {
		if factor.Code == factorCode {
			return factor, true
		}
	}
	return FactorSnapshot{}, false
}

// findInterpretRule 查找解读规则。
func findInterpretRule(factor FactorSnapshot, score float64) *InterpretationRule {
	rules := toScoreRangeRules(factor.InterpretRules)
	matched := interpretation.MatchRule(score, rules)
	if matched == nil {
		return nil
	}
	rule := NewInterpretationRule(NewScoreRange(matched.Min, matched.Max), RiskLevel(matched.Level), matched.Conclusion, matched.Suggestion)
	return &rule
}

// findInterpretRuleWithRangeFallback 查找解读规则（范围降级）。
func findInterpretRuleWithRangeFallback(factor FactorSnapshot, score float64) *InterpretationRule {
	rules := toScoreRangeRules(factor.InterpretRules)
	matched := interpretation.MatchRuleWithRangeFallback(score, rules)
	if matched == nil {
		return nil
	}
	rule := NewInterpretationRule(NewScoreRange(matched.Min, matched.Max), RiskLevel(matched.Level), matched.Conclusion, matched.Suggestion)
	return &rule
}

// defaultRiskLevelByScore 默认风险等级。
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

// riskLevelOrder 风险等级排序。
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

// defaultFactorInterpretation 默认因子解读。
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

// defaultOverallInterpretation 默认总体解读。
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

// cloneEvaluationFloat64Ptr 克隆浮点数指针。
func cloneEvaluationFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// DefaultScoringStrategyRegistry 默认量表因子聚合策略注册表。
type DefaultScoringStrategyRegistry struct{}

// ScoreFactor 执行量表因子聚合策略。
func (DefaultScoringStrategyRegistry) ScoreFactor(_ context.Context, factor FactorSnapshot, values []float64) (float64, error) {
	score, err := calculation.DefaultStrategyRegistry{}.Score(context.Background(), calculation.Dimension{
		Code:            factor.Code.String(),
		ScoringStrategy: string(factor.ScoringStrategy),
	}, values)
	if err != nil {
		return 0, err
	}
	if factor.ScoringStrategy != ScoringStrategySum &&
		factor.ScoringStrategy != ScoringStrategyAvg &&
		factor.ScoringStrategy != ScoringStrategyCnt {
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
	return score, nil
}

type scaleCalculationRegistry struct {
	registry ScoringStrategyRegistry
}

func (r scaleCalculationRegistry) Score(ctx context.Context, dimension calculation.Dimension, values []float64) (float64, error) {
	if r.registry == nil {
		return 0, nil
	}
	return r.registry.ScoreFactor(ctx, FactorSnapshot{
		Code:            NewFactorCode(dimension.Code),
		ScoringStrategy: ScoringStrategyCode(dimension.ScoringStrategy),
	}, values)
}

func toScoreRangeRules(rules []InterpretationRule) []interpretation.ScoreRangeRule {
	converted := make([]interpretation.ScoreRangeRule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, interpretation.ScoreRangeRule{
			Min:        rule.GetScoreRange().Min(),
			Max:        rule.GetScoreRange().Max(),
			Level:      string(rule.GetRiskLevel()),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
	}
	return converted
}
