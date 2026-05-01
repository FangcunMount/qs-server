package interpretengine

type StrategyType string

const (
	StrategyTypeThreshold StrategyType = "threshold"
	StrategyTypeRange     StrategyType = "range"
	StrategyTypeComposite StrategyType = "composite"
)

type RuleSpec struct {
	Min         float64
	Max         float64
	RiskLevel   string
	Label       string
	Description string
	Suggestion  string
}

func (r RuleSpec) Contains(score float64) bool {
	return score >= r.Min && score < r.Max
}

type Config struct {
	FactorCode string
	Rules      []RuleSpec
	Params     map[string]string
}

type Result struct {
	FactorCode  string
	Score       float64
	RiskLevel   string
	Label       string
	Description string
	Suggestion  string
}

type FactorScore struct {
	FactorCode string
	Score      float64
}

type FactorCondition struct {
	FactorCode string
	Operator   string
	Value      float64
	MaxValue   float64
}

type CompositeRuleSpec struct {
	Conditions  []FactorCondition
	Operator    string
	RiskLevel   string
	Label       string
	Description string
	Suggestion  string
}

type CompositeConfig struct {
	Rules  []CompositeRuleSpec
	Params map[string]string
}

type CompositeResult struct {
	RiskLevel   string
	Label       string
	Description string
	Suggestion  string
	Details     []*Result
}

type Interpreter interface {
	InterpretFactor(score float64, config *Config, strategyType StrategyType) (*Result, error)
	InterpretFactorWithRule(score float64, rule RuleSpec) *Result
	InterpretMultipleFactors(scores []FactorScore, config *CompositeConfig, strategyType StrategyType) (*CompositeResult, error)
}

type DefaultProvider interface {
	ProvideFactor(factorName string, score float64, riskLevel string) *Result
	ProvideOverall(totalScore float64, riskLevel string) *Result
}
