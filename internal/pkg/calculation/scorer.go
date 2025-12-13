package calculation

// ==================== 单值计分器 ====================

// OptionScorer 选项计分器
// 根据选项映射计算单个值的得分
// 设计原则：无状态，纯函数式计算
type OptionScorer struct{}

// NewOptionScorer 创建选项计分器
func NewOptionScorer() *OptionScorer {
	return &OptionScorer{}
}

// Score 计算单个值的得分
// value: 可计分的值（抽象接口）
// optionScores: 选项编码 -> 分数的映射
// 返回: 计算得分
func (s *OptionScorer) Score(value ScorableValue, optionScores map[string]float64) float64 {
	if value == nil || value.IsEmpty() || len(optionScores) == 0 {
		return 0
	}

	// 尝试作为单选计分
	if selected, ok := value.AsSingleSelection(); ok {
		if score, found := optionScores[selected]; found {
			return score
		}
	}

	// 尝试作为多选计分（累加所有选中选项的分数）
	if selections, ok := value.AsMultipleSelections(); ok {
		var totalScore float64
		for _, sel := range selections {
			if score, found := optionScores[sel]; found {
				totalScore += score
			}
		}
		return totalScore
	}

	// 尝试作为数值计分（如李克特量表，直接使用数值）
	if num, ok := value.AsNumber(); ok {
		return num
	}

	return 0
}

// ScoreWithMax 计算单个值的得分，同时返回满分
// 满分定义为选项中的最高分
func (s *OptionScorer) ScoreWithMax(value ScorableValue, optionScores map[string]float64) (score float64, maxScore float64) {
	score = s.Score(value, optionScores)
	maxScore = s.getMaxScore(optionScores)
	return
}

// getMaxScore 获取选项中的最高分
func (s *OptionScorer) getMaxScore(optionScores map[string]float64) float64 {
	var maxScore float64
	for _, score := range optionScores {
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// ==================== 默认计分器实例 ====================

// 默认计分器（单例）
var defaultScorer = NewOptionScorer()

// DefaultScorer 获取默认计分器
func DefaultScorer() *OptionScorer {
	return defaultScorer
}

// Score 使用默认计分器计算得分（便捷函数）
func Score(value ScorableValue, optionScores map[string]float64) float64 {
	return defaultScorer.Score(value, optionScores)
}

// ScoreWithMax 使用默认计分器计算得分和满分（便捷函数）
func ScoreWithMax(value ScorableValue, optionScores map[string]float64) (score float64, maxScore float64) {
	return defaultScorer.ScoreWithMax(value, optionScores)
}
