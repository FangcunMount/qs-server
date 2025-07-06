package medicalscale

import (
	"fmt"
)

// FactorType 因子类型
type FactorType string

const (
	// PrimaryFactor 一级因子
	PrimaryFactor FactorType = "primary"
	// MultilevelFactor 多级因子
	MultilevelFactor FactorType = "multilevel"
)

// String 返回因子类型的字符串表示
func (ft FactorType) String() string {
	return string(ft)
}

// IsValid 检查因子类型是否有效
func (ft FactorType) IsValid() bool {
	return ft == PrimaryFactor || ft == MultilevelFactor
}

// Factor 因子实体
type Factor struct {
	code            string
	title           string
	isTotalScore    bool
	factorType      FactorType
	calculationRule CalculationRule
	interpretRules  []InterpretRule
}

// NewFactor 创建新的因子
func NewFactor(
	code, title string,
	isTotalScore bool,
	factorType FactorType,
	calculationRule CalculationRule,
	interpretRules []InterpretRule,
) Factor {
	return Factor{
		code:            code,
		title:           title,
		isTotalScore:    isTotalScore,
		factorType:      factorType,
		calculationRule: calculationRule,
		interpretRules:  interpretRules,
	}
}

// Code 获取因子代码
func (f Factor) Code() string {
	return f.code
}

// Title 获取因子标题
func (f Factor) Title() string {
	return f.title
}

// IsTotalScore 是否为总分因子
func (f Factor) IsTotalScore() bool {
	return f.isTotalScore
}

// Type 获取因子类型
func (f Factor) Type() FactorType {
	return f.factorType
}

// CalculationRule 获取计算规则
func (f Factor) CalculationRule() CalculationRule {
	return f.calculationRule
}

// InterpretRules 获取解读规则列表
func (f Factor) InterpretRules() []InterpretRule {
	// 返回副本以保护内部状态
	result := make([]InterpretRule, len(f.interpretRules))
	copy(result, f.interpretRules)
	return result
}

// UpdateTitle 更新因子标题
func (f *Factor) UpdateTitle(title string) error {
	if title == "" {
		return fmt.Errorf("factor title cannot be empty")
	}
	f.title = title
	return nil
}

// UpdateCalculationRule 更新计算规则
func (f *Factor) UpdateCalculationRule(rule CalculationRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("invalid calculation rule: %w", err)
	}
	f.calculationRule = rule
	return nil
}

// AddInterpretRule 添加解读规则
func (f *Factor) AddInterpretRule(rule InterpretRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("invalid interpret rule: %w", err)
	}

	// 检查分数范围是否冲突
	for _, existingRule := range f.interpretRules {
		if rule.ScoreRange().IsOverlapping(existingRule.ScoreRange()) {
			return fmt.Errorf("score range conflicts with existing rule")
		}
	}

	f.interpretRules = append(f.interpretRules, rule)
	return nil
}

// RemoveInterpretRule 移除解读规则
func (f *Factor) RemoveInterpretRule(index int) error {
	if index < 0 || index >= len(f.interpretRules) {
		return fmt.Errorf("invalid interpret rule index: %d", index)
	}

	f.interpretRules = append(f.interpretRules[:index], f.interpretRules[index+1:]...)
	return nil
}

// GetInterpretation 根据分数获取解读
func (f Factor) GetInterpretation(score float64) (string, error) {
	for _, rule := range f.interpretRules {
		if rule.ScoreRange().Contains(score) {
			return rule.Content(), nil
		}
	}
	return "", fmt.Errorf("no interpretation found for score: %.2f", score)
}

// Validate 验证因子的完整性
func (f Factor) Validate() error {
	if f.code == "" {
		return fmt.Errorf("factor code cannot be empty")
	}
	if f.title == "" {
		return fmt.Errorf("factor title cannot be empty")
	}
	if !f.factorType.IsValid() {
		return fmt.Errorf("invalid factor type: %s", f.factorType)
	}

	// 验证计算规则
	if err := f.calculationRule.Validate(); err != nil {
		return fmt.Errorf("calculation rule validation failed: %w", err)
	}

	// 验证解读规则
	if len(f.interpretRules) == 0 {
		return fmt.Errorf("factor must have at least one interpret rule")
	}

	for i, rule := range f.interpretRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("interpret rule %d validation failed: %w", i, err)
		}
	}

	// 检查解读规则的分数范围是否有重叠
	for i, rule1 := range f.interpretRules {
		for j, rule2 := range f.interpretRules {
			if i != j && rule1.ScoreRange().IsOverlapping(rule2.ScoreRange()) {
				return fmt.Errorf("interpret rules have overlapping score ranges")
			}
		}
	}

	return nil
}

// CalculationRule 计算规则值对象
type CalculationRule struct {
	formulaType FormulaType
	sourceCodes []string
}

// FormulaType 公式类型
type FormulaType string

const (
	// SumFormula 求和公式
	SumFormula FormulaType = "sum"
	// AverageFormula 平均值公式
	AverageFormula FormulaType = "average"
	// WeightedSumFormula 加权求和公式
	WeightedSumFormula FormulaType = "weighted_sum"
	// CustomFormula 自定义公式
	CustomFormula FormulaType = "custom"
)

// String 返回公式类型的字符串表示
func (ft FormulaType) String() string {
	return string(ft)
}

// IsValid 检查公式类型是否有效
func (ft FormulaType) IsValid() bool {
	return ft == SumFormula || ft == AverageFormula || ft == WeightedSumFormula || ft == CustomFormula
}

// NewCalculationRule 创建计算规则
func NewCalculationRule(formulaType FormulaType, sourceCodes []string) CalculationRule {
	return CalculationRule{
		formulaType: formulaType,
		sourceCodes: sourceCodes,
	}
}

// FormulaType 获取公式类型
func (cr CalculationRule) FormulaType() FormulaType {
	return cr.formulaType
}

// SourceCodes 获取源代码列表
func (cr CalculationRule) SourceCodes() []string {
	// 返回副本以保护内部状态
	result := make([]string, len(cr.sourceCodes))
	copy(result, cr.sourceCodes)
	return result
}

// Validate 验证计算规则
func (cr CalculationRule) Validate() error {
	if !cr.formulaType.IsValid() {
		return fmt.Errorf("invalid formula type: %s", cr.formulaType)
	}
	if len(cr.sourceCodes) == 0 {
		return fmt.Errorf("source codes cannot be empty")
	}
	for i, code := range cr.sourceCodes {
		if code == "" {
			return fmt.Errorf("source code at index %d cannot be empty", i)
		}
	}
	return nil
}

// InterpretRule 解读规则值对象
type InterpretRule struct {
	scoreRange ScoreRange
	content    string
}

// NewInterpretRule 创建解读规则
func NewInterpretRule(scoreRange ScoreRange, content string) InterpretRule {
	return InterpretRule{
		scoreRange: scoreRange,
		content:    content,
	}
}

// ScoreRange 获取分数范围
func (ir InterpretRule) ScoreRange() ScoreRange {
	return ir.scoreRange
}

// Content 获取解读内容
func (ir InterpretRule) Content() string {
	return ir.content
}

// Validate 验证解读规则
func (ir InterpretRule) Validate() error {
	if err := ir.scoreRange.Validate(); err != nil {
		return fmt.Errorf("invalid score range: %w", err)
	}
	if ir.content == "" {
		return fmt.Errorf("interpret content cannot be empty")
	}
	return nil
}

// ScoreRange 分数范围值对象
type ScoreRange struct {
	minScore float64
	maxScore float64
}

// NewScoreRange 创建分数范围
func NewScoreRange(minScore, maxScore float64) ScoreRange {
	return ScoreRange{
		minScore: minScore,
		maxScore: maxScore,
	}
}

// MinScore 获取最低分
func (sr ScoreRange) MinScore() float64 {
	return sr.minScore
}

// MaxScore 获取最高分
func (sr ScoreRange) MaxScore() float64 {
	return sr.maxScore
}

// Contains 检查分数是否在范围内
func (sr ScoreRange) Contains(score float64) bool {
	return score >= sr.minScore && score <= sr.maxScore
}

// IsOverlapping 检查是否与另一个分数范围重叠
func (sr ScoreRange) IsOverlapping(other ScoreRange) bool {
	return sr.minScore <= other.maxScore && sr.maxScore >= other.minScore
}

// Validate 验证分数范围
func (sr ScoreRange) Validate() error {
	if sr.minScore > sr.maxScore {
		return fmt.Errorf("min score (%.2f) cannot be greater than max score (%.2f)", sr.minScore, sr.maxScore)
	}
	return nil
}

// String 返回分数范围的字符串表示
func (sr ScoreRange) String() string {
	return fmt.Sprintf("[%.2f, %.2f]", sr.minScore, sr.maxScore)
}
