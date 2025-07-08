package interpretation

import (
	"fmt"
	"sort"
)

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
func (ir InterpretRule) GetScoreRange() ScoreRange {
	return ir.scoreRange
}

// Content 获取解读内容
func (ir InterpretRule) GetContent() string {
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
	// 由于是左闭右开区间，相邻区间的边界点（前一个区间的max等于后一个区间的min）不算重叠
	if sr.maxScore == other.minScore || sr.minScore == other.maxScore {
		return false
	}
	return sr.minScore < other.maxScore && sr.maxScore > other.minScore
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

// Validate 验证单个分数范围
func (sr ScoreRange) Validate() error {
	if sr.minScore >= sr.maxScore { // 左闭右开区间不允许 min >= max
		return fmt.Errorf("invalid range: [%.2f, %.2f), min score must be less than max score", sr.minScore, sr.maxScore)
	}
	return nil
}

// String 返回分数范围的字符串表示（使用左闭右开区间表示法）
func (sr ScoreRange) String() string {
	return fmt.Sprintf("[%.2f, %.2f)", sr.minScore, sr.maxScore)
}
