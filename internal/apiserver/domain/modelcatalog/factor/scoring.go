package factor

// ScoringStrategy 命名如何 question 分数 aggregate 为 因子 原始分。
type ScoringStrategy string

const (
	ScoringStrategySum         ScoringStrategy = "sum"
	ScoringStrategyAvg         ScoringStrategy = "avg"
	ScoringStrategyWeightedSum ScoringStrategy = "weighted_sum"
	ScoringStrategyMax         ScoringStrategy = "max"
	ScoringStrategyMin         ScoringStrategy = "min"
	ScoringStrategyCnt         ScoringStrategy = "cnt"
)

func (s ScoringStrategy) String() string { return string(s) }

// ScoringParams 携带strategy-特定 parameters。
type ScoringParams struct {
	CntOptionContents []string
}
