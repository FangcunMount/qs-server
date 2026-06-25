package scale

// InterpretationRule 解读规则值对象
// 定义分数区间与风险等级、结论文案的映射关系
type InterpretationRule struct {
	scoreRange ScoreRange // 分数区间
	riskLevel  RiskLevel  // 风险等级
	conclusion string     // 结论文案
	suggestion string     // 建议文案
}

// NewInterpretationRule 创建解读规则
func NewInterpretationRule(
	scoreRange ScoreRange,
	riskLevel RiskLevel,
	conclusion string,
	suggestion string,
) InterpretationRule {
	return InterpretationRule{
		scoreRange: scoreRange,
		riskLevel:  riskLevel,
		conclusion: conclusion,
		suggestion: suggestion,
	}
}

// GetScoreRange 获取分数区间
func (r InterpretationRule) GetScoreRange() ScoreRange {
	return r.scoreRange
}

// GetRiskLevel 获取风险等级
func (r InterpretationRule) GetRiskLevel() RiskLevel {
	return r.riskLevel
}

// GetConclusion 获取结论文案
func (r InterpretationRule) GetConclusion() string {
	return r.conclusion
}

// GetSuggestion 获取建议文案
func (r InterpretationRule) GetSuggestion() string {
	return r.suggestion
}

// Matches 判断给定分数是否匹配此规则
func (r InterpretationRule) Matches(score float64) bool {
	return r.scoreRange.Contains(score)
}

// IsValid 检查解读规则是否有效
func (r InterpretationRule) IsValid() bool {
	return r.scoreRange.IsValid() && r.riskLevel.IsValid()
}
