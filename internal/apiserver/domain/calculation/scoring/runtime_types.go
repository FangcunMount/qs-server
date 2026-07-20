package scoring

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"

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
	StrategySum         Strategy = "sum"
	StrategyAvg         Strategy = "avg"
	StrategyCnt         Strategy = "cnt"
	StrategyWeightedSum Strategy = "weighted_sum"
)

func (s Strategy) String() string {
	return string(s)
}

func (s Strategy) IsValid() bool {
	return capability.Supports(capability.PathScaleDescriptor, capability.UsageQuestionAggregation, string(s)) ||
		capability.Supports(capability.PathScaleDescriptor, capability.UsageCompositeProjection, string(s))
}
