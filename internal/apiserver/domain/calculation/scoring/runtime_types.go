package scoring

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

type Strategy string

const (
	StrategySum Strategy = "sum"
	StrategyAvg Strategy = "avg"
	StrategyCnt Strategy = "cnt"
)

func (s Strategy) String() string {
	return string(s)
}

func (s Strategy) IsValid() bool {
	return s == StrategySum || s == StrategyAvg || s == StrategyCnt
}
