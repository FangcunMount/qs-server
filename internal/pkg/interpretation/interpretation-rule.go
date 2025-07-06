package interpretation

import "fmt"

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
