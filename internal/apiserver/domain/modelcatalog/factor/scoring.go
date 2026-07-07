package factor

// ScoringStrategy names how question scores aggregate into a factor raw score.
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

// ScoringParams carries strategy-specific parameters.
type ScoringParams struct {
	CntOptionContents []string
}
