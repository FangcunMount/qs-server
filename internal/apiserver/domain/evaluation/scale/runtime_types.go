package scale

type RiskLevel string

const (
	RiskLevelNone   RiskLevel = "none"
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
	RiskLevelSevere RiskLevel = "severe"
)

func (r RiskLevel) String() string {
	return string(r)
}

type ScoringStrategy string

const (
	ScoringStrategySum ScoringStrategy = "sum"
	ScoringStrategyAvg ScoringStrategy = "avg"
	ScoringStrategyCnt ScoringStrategy = "cnt"
)

func (s ScoringStrategy) String() string {
	return string(s)
}

func (s ScoringStrategy) IsValid() bool {
	return s == ScoringStrategySum || s == ScoringStrategyAvg || s == ScoringStrategyCnt
}
