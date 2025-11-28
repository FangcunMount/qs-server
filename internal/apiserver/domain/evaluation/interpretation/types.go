package interpretation

import (
	"fmt"
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// ==================== 策略类型定义 ====================

// StrategyType 解读策略类型
type StrategyType string

const (
	// StrategyTypeThreshold 阈值解读策略
	// 得分超过阈值则为指定风险等级
	StrategyTypeThreshold StrategyType = "threshold"

	// StrategyTypeRange 区间解读策略
	// 根据得分所在区间确定风险等级
	StrategyTypeRange StrategyType = "range"

	// StrategyTypeComposite 组合解读策略
	// 多个因子组合判断风险等级
	StrategyTypeComposite StrategyType = "composite"
)

// ==================== 风险等级定义 ====================

// RiskLevel 风险等级
// 直接复用 scale 子域的定义
type RiskLevel = scale.RiskLevel

const (
	RiskLevelNone   = scale.RiskLevelNone
	RiskLevelLow    = scale.RiskLevelLow
	RiskLevelMedium = scale.RiskLevelMedium
	RiskLevelHigh   = scale.RiskLevelHigh
	RiskLevelSevere = scale.RiskLevelSevere
)

// ==================== 解读规则定义 ====================

// InterpretRule 解读规则
// 定义得分区间与解读结果的映射
type InterpretRule struct {
	// 区间范围 [Min, Max]
	Min float64
	Max float64

	// 解读结果
	RiskLevel   RiskLevel
	Label       string // 简短标签，如"正常"、"轻度"
	Description string // 详细描述
	Suggestion  string // 建议
}

// Contains 判断得分是否在规则区间内
func (r InterpretRule) Contains(score float64) bool {
	return score >= r.Min && score <= r.Max
}

// ==================== 解读配置 ====================

// InterpretConfig 解读配置
// 包含因子的解读规则集合
type InterpretConfig struct {
	FactorCode string            // 因子编码
	Rules      []InterpretRule   // 解读规则列表（按区间排序）
	Params     map[string]string // 额外参数
}

// ==================== 解读结果 ====================

// InterpretResult 解读结果
// 无状态的值对象，表示单个因子的解读结论
type InterpretResult struct {
	FactorCode  string    // 因子编码
	Score       float64   // 原始得分
	RiskLevel   RiskLevel // 风险等级
	Label       string    // 简短标签
	Description string    // 详细描述
	Suggestion  string    // 建议
}

// IsHighRisk 是否为高风险
func (r *InterpretResult) IsHighRisk() bool {
	return r.RiskLevel == RiskLevelHigh || r.RiskLevel == RiskLevelSevere
}

// ==================== 组合解读输入 ====================

// FactorScore 因子得分
// 用于组合解读策略的输入
type FactorScore struct {
	FactorCode string
	Score      float64
}

// CompositeConfig 组合解读配置
// 定义多因子组合判断规则
type CompositeConfig struct {
	Rules  []CompositeRule   // 组合规则列表
	Params map[string]string // 额外参数
}

// CompositeRule 组合规则
// 定义多个因子的组合条件
type CompositeRule struct {
	Conditions  []FactorCondition // 因子条件列表
	Operator    string            // 条件组合方式: "and" 或 "or"
	RiskLevel   RiskLevel         // 符合条件时的风险等级
	Label       string
	Description string
	Suggestion  string
}

// FactorCondition 因子条件
type FactorCondition struct {
	FactorCode string  // 因子编码
	Operator   string  // 比较运算符: ">", ">=", "<", "<=", "==", "between"
	Value      float64 // 比较值
	MaxValue   float64 // 用于 between 运算符
}

// ==================== 组合解读结果 ====================

// CompositeResult 组合解读结果
type CompositeResult struct {
	RiskLevel   RiskLevel          // 综合风险等级
	Label       string             // 综合标签
	Description string             // 综合描述
	Suggestion  string             // 综合建议
	Details     []*InterpretResult // 各因子详细解读
}

// IsHighRisk 是否为高风险
func (r *CompositeResult) IsHighRisk() bool {
	return r.RiskLevel == RiskLevelHigh || r.RiskLevel == RiskLevelSevere
}

// ==================== 分数范围值对象（用于规则配置）====================

// ScoreRange 分数范围值对象
// 采用左闭右开区间 [min, max)
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

// Contains 检查分数是否在范围内（左闭右开区间）
func (sr ScoreRange) Contains(score float64) bool {
	return score >= sr.minScore && score < sr.maxScore
}

// IsOverlapping 检查是否与另一个分数范围重叠（考虑左闭右开特性）
func (sr ScoreRange) IsOverlapping(other ScoreRange) bool {
	if sr.maxScore == other.minScore || sr.minScore == other.maxScore {
		return false
	}
	return sr.minScore < other.maxScore && sr.maxScore > other.minScore
}

// Validate 验证单个分数范围
func (sr ScoreRange) Validate() error {
	if sr.minScore >= sr.maxScore {
		return fmt.Errorf("invalid range: [%.2f, %.2f), min score must be less than max score", sr.minScore, sr.maxScore)
	}
	return nil
}

// String 返回分数范围的字符串表示（使用左闭右开区间表示法）
func (sr ScoreRange) String() string {
	return fmt.Sprintf("[%.2f, %.2f)", sr.minScore, sr.maxScore)
}

// ValidateRanges 验证多个分数范围是否符合左闭右开且连续的要求
func ValidateRanges(ranges []ScoreRange) error {
	if len(ranges) == 0 {
		return fmt.Errorf("score ranges cannot be empty")
	}

	// 按照 minScore 排序
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].minScore < ranges[j].minScore
	})

	// 检查每个区间是否有效
	for i, r := range ranges {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("invalid range at index %d: %w", i, err)
		}

		// 检查相邻区间是否连续（前一个区间的max等于后一个区间的min）
		if i > 0 {
			prev := ranges[i-1]
			if prev.maxScore != r.minScore {
				return fmt.Errorf("discontinuous ranges at index %d: %.2f != %.2f", i, prev.maxScore, r.minScore)
			}
		}

		// 检查是否与其他区间重叠（除了边界点）
		for j := i + 1; j < len(ranges); j++ {
			if r.IsOverlapping(ranges[j]) {
				return fmt.Errorf("overlapping ranges: %s and %s", r.String(), ranges[j].String())
			}
		}
	}

	return nil
}

// ==================== 简化解读规则（用于规则配置）====================

// SimpleInterpretRule 简化解读规则值对象
// 用于配置分数范围到解读内容的映射
type SimpleInterpretRule struct {
	scoreRange ScoreRange
	content    string
}

// NewSimpleInterpretRule 创建简化解读规则
func NewSimpleInterpretRule(scoreRange ScoreRange, content string) SimpleInterpretRule {
	return SimpleInterpretRule{
		scoreRange: scoreRange,
		content:    content,
	}
}

// GetScoreRange 获取分数范围
func (ir SimpleInterpretRule) GetScoreRange() ScoreRange {
	return ir.scoreRange
}

// GetContent 获取解读内容
func (ir SimpleInterpretRule) GetContent() string {
	return ir.content
}

// Validate 验证解读规则
func (ir SimpleInterpretRule) Validate() error {
	if err := ir.scoreRange.Validate(); err != nil {
		return fmt.Errorf("invalid score range: %w", err)
	}
	if ir.content == "" {
		return fmt.Errorf("interpret content cannot be empty")
	}
	return nil
}
