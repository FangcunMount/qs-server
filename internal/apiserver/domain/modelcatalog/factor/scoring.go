package factor

// ScoringStrategy names how source scores aggregate into a factor raw score.
// The declared set must match capability.DeclaredAuthoringStrategyCodes()
// (OpenAPI enums / ops options / publish catalog). Path-specific subsets are
// enforced by ValidateScoringStrategyCapability — presence here is not global support.
type ScoringStrategy string

const (
	ScoringStrategySum         ScoringStrategy = "sum"
	ScoringStrategyAvg         ScoringStrategy = "avg"
	ScoringStrategyWeightedSum ScoringStrategy = "weighted_sum"
	ScoringStrategyWeightedAvg ScoringStrategy = "weighted_avg"
	ScoringStrategyCnt         ScoringStrategy = "cnt"
	ScoringStrategyNone        ScoringStrategy = "none"
	ScoringStrategyLookup      ScoringStrategy = "lookup"
	ScoringStrategyCustom      ScoringStrategy = "custom"
)

func (s ScoringStrategy) String() string { return string(s) }

// ScoringParams 携带strategy-特定 parameters。
type ScoringParams struct {
	CntOptionContents []string
}
