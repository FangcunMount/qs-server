package calculation

import (
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation/rules"
)

// CalculationRuleBuilder 计算规则构建器
type CalculationRuleBuilder struct {
	rule *rules.CalculationRule
}

// NewCalculationRule 创建新的计算规则构建器
func NewCalculationRule(name string) *CalculationRuleBuilder {
	return &CalculationRuleBuilder{
		rule: rules.NewCalculationRule(name),
	}
}

// WithPrecision 设置精度
func (b *CalculationRuleBuilder) WithPrecision(precision int) *CalculationRuleBuilder {
	b.rule.SetPrecision(precision)
	return b
}

// WithRoundingMode 设置舍入模式
func (b *CalculationRuleBuilder) WithRoundingMode(mode string) *CalculationRuleBuilder {
	b.rule.SetRoundingMode(mode)
	return b
}

// WithOperandLimits 设置操作数限制
func (b *CalculationRuleBuilder) WithOperandLimits(min, max int) *CalculationRuleBuilder {
	b.rule.SetOperandLimits(min, max)
	return b
}

// WithWeights 设置权重
func (b *CalculationRuleBuilder) WithWeights(weights []float64) *CalculationRuleBuilder {
	b.rule.SetWeights(weights)
	return b
}

// WithParam 添加参数
func (b *CalculationRuleBuilder) WithParam(key string, value interface{}) *CalculationRuleBuilder {
	b.rule.AddParam(key, value)
	return b
}

// WithMessage 设置消息
func (b *CalculationRuleBuilder) WithMessage(message string) *CalculationRuleBuilder {
	b.rule.SetMessage(message)
	return b
}

// Build 构建计算规则
func (b *CalculationRuleBuilder) Build() *rules.CalculationRule {
	return b.rule
}

// 便捷的计算规则创建函数

// Sum 创建求和计算规则
func Sum() *CalculationRuleBuilder {
	return NewCalculationRule("sum")
}

// SumWithPrecision 创建带精度的求和规则
func SumWithPrecision(precision int) *CalculationRuleBuilder {
	return Sum().WithPrecision(precision)
}

// Average 创建平均值计算规则
func Average() *CalculationRuleBuilder {
	return NewCalculationRule("average")
}

// AverageWithRounding 创建带舍入模式的平均值规则
func AverageWithRounding(precision int, mode string) *CalculationRuleBuilder {
	return Average().WithPrecision(precision).WithRoundingMode(mode)
}

// Max 创建最大值计算规则
func Max() *CalculationRuleBuilder {
	return NewCalculationRule("max")
}

// Min 创建最小值计算规则
func Min() *CalculationRuleBuilder {
	return NewCalculationRule("min")
}

// Option 创建选项计算规则
func Option() *CalculationRuleBuilder {
	return NewCalculationRule("option").WithOperandLimits(1, 1)
}

// OptionWithMax 创建允许多个操作数的选项规则
func OptionWithMax(maxOperands int) *CalculationRuleBuilder {
	return NewCalculationRule("option").WithOperandLimits(1, maxOperands)
}

// Weighted 创建加权计算规则
func Weighted(weights []float64) *CalculationRuleBuilder {
	return NewCalculationRule("weighted").WithWeights(weights)
}

// WeightedAverage 创建加权平均规则
func WeightedAverage(weights []float64) *CalculationRuleBuilder {
	return Weighted(weights).WithParam("calculation_type", "weighted_average")
}

// WeightedSum 创建加权求和规则
func WeightedSum(weights []float64) *CalculationRuleBuilder {
	return Weighted(weights).WithParam("calculation_type", "weighted_sum")
}

// 复合规则构建器

// NumberCalculationRules 数值计算规则组合
type NumberCalculationRules struct {
	calculationType string
	precision       int
	roundingMode    string
	minOperands     int
	maxOperands     int
	weights         []float64
}

// NewNumberCalculationRules 创建数值计算规则组合
func NewNumberCalculationRules(calculationType string) *NumberCalculationRules {
	return &NumberCalculationRules{
		calculationType: calculationType,
		precision:       2,
		roundingMode:    "round",
		minOperands:     0,
		maxOperands:     0,
		weights:         []float64{},
	}
}

// SetPrecision 设置精度
func (r *NumberCalculationRules) SetPrecision(precision int) *NumberCalculationRules {
	r.precision = precision
	return r
}

// SetRoundingMode 设置舍入模式
func (r *NumberCalculationRules) SetRoundingMode(mode string) *NumberCalculationRules {
	r.roundingMode = mode
	return r
}

// SetOperandLimits 设置操作数限制
func (r *NumberCalculationRules) SetOperandLimits(min, max int) *NumberCalculationRules {
	r.minOperands = min
	r.maxOperands = max
	return r
}

// SetWeights 设置权重
func (r *NumberCalculationRules) SetWeights(weights []float64) *NumberCalculationRules {
	r.weights = weights
	return r
}

// Build 构建计算规则
func (r *NumberCalculationRules) Build() *rules.CalculationRule {
	builder := NewCalculationRule(r.calculationType).
		WithPrecision(r.precision).
		WithRoundingMode(r.roundingMode).
		WithOperandLimits(r.minOperands, r.maxOperands)

	if len(r.weights) > 0 {
		builder.WithWeights(r.weights)
	}

	return builder.Build()
}

// ScoreCalculationConfig 分数计算配置
type ScoreCalculationConfig struct {
	Type         string    // sum, average, max, min, weighted
	Precision    int       // 精度
	RoundingMode string    // 舍入模式
	Weights      []float64 // 权重（仅用于加权计算）
}

// BuildScoreCalculationRule 构建分数计算规则
func BuildScoreCalculationRule(config ScoreCalculationConfig) *rules.CalculationRule {
	builder := NewCalculationRule(config.Type).
		WithPrecision(config.Precision).
		WithRoundingMode(config.RoundingMode)

	if config.Type == "weighted" && len(config.Weights) > 0 {
		builder.WithWeights(config.Weights).
			WithParam("calculation_type", "weighted_average")
	}

	return builder.Build()
}

// QuestionnaireCalculationRules 问卷计算规则集合
type QuestionnaireCalculationRules struct {
	rules map[string]*rules.CalculationRule
}

// NewQuestionnaireCalculationRules 创建问卷计算规则集合
func NewQuestionnaireCalculationRules() *QuestionnaireCalculationRules {
	return &QuestionnaireCalculationRules{
		rules: make(map[string]*rules.CalculationRule),
	}
}

// AddRule 添加规则
func (q *QuestionnaireCalculationRules) AddRule(name string, rule *rules.CalculationRule) *QuestionnaireCalculationRules {
	q.rules[name] = rule
	return q
}

// AddSumRule 添加求和规则
func (q *QuestionnaireCalculationRules) AddSumRule(name string, precision int) *QuestionnaireCalculationRules {
	rule := Sum().WithPrecision(precision).Build()
	q.rules[name] = rule
	return q
}

// AddAverageRule 添加平均值规则
func (q *QuestionnaireCalculationRules) AddAverageRule(name string, precision int, roundingMode string) *QuestionnaireCalculationRules {
	rule := Average().WithPrecision(precision).WithRoundingMode(roundingMode).Build()
	q.rules[name] = rule
	return q
}

// AddWeightedRule 添加加权计算规则
func (q *QuestionnaireCalculationRules) AddWeightedRule(name string, weights []float64, calcType string) *QuestionnaireCalculationRules {
	rule := Weighted(weights).WithParam("calculation_type", calcType).Build()
	q.rules[name] = rule
	return q
}

// GetRule 获取规则
func (q *QuestionnaireCalculationRules) GetRule(name string) *rules.CalculationRule {
	return q.rules[name]
}

// GetAllRules 获取所有规则
func (q *QuestionnaireCalculationRules) GetAllRules() map[string]*rules.CalculationRule {
	return q.rules
}
