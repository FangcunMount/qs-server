package definition

import "fmt"

// ScoringSpec 描述因子的计分策略、参数与最大分约束。
type ScoringSpec struct {
	strategy ScoringStrategyCode
	params   *ScoringParams
	maxScore *float64
}

func NewScoringSpec(strategy ScoringStrategyCode, params *ScoringParams, maxScore *float64) (ScoringSpec, error) {
	spec := ScoringSpec{
		strategy: strategy,
		params:   cloneScoringParams(params),
		maxScore: cloneFloat64Ptr(maxScore),
	}
	if spec.strategy == "" {
		spec.strategy = ScoringStrategySum
	}
	if spec.params == nil {
		spec.params = NewScoringParams()
	}
	if err := spec.Validate(); err != nil {
		return ScoringSpec{}, err
	}
	return spec, nil
}

func defaultScoringSpec() ScoringSpec {
	spec, err := NewScoringSpec(ScoringStrategySum, NewScoringParams(), nil)
	if err != nil {
		panic(err)
	}
	return spec
}

func (s ScoringSpec) Strategy() ScoringStrategyCode {
	return s.strategy
}

func (s ScoringSpec) Params() *ScoringParams {
	return cloneScoringParams(s.params)
}

func (s ScoringSpec) MaxScore() *float64 {
	return cloneFloat64Ptr(s.maxScore)
}

func (s ScoringSpec) Validate() error {
	if !s.strategy.IsValid() {
		return newError(ErrorKindInvalidArgument, "invalid scoring strategy: %s", s.strategy)
	}
	if s.maxScore != nil && *s.maxScore <= 0 {
		return newError(ErrorKindInvalidArgument, "max score must be greater than 0")
	}
	switch s.strategy {
	case ScoringStrategyCnt:
		if s.params == nil || len(s.params.GetCntOptionContents()) == 0 {
			return newError(ErrorKindInvalidArgument, "cnt scoring strategy requires cnt_option_contents")
		}
	case ScoringStrategySum, ScoringStrategyAvg:
		return nil
	default:
		return fmt.Errorf("unsupported scoring strategy: %s", s.strategy)
	}
	return nil
}

func (s ScoringSpec) withStrategy(strategy ScoringStrategyCode) ScoringSpec {
	s.strategy = strategy
	return s
}

func (s ScoringSpec) withParams(params *ScoringParams) ScoringSpec {
	s.params = cloneScoringParams(params)
	if s.params == nil {
		s.params = NewScoringParams()
	}
	return s
}

func (s ScoringSpec) withMaxScore(maxScore *float64) ScoringSpec {
	s.maxScore = cloneFloat64Ptr(maxScore)
	return s
}

func cloneScoringParams(params *ScoringParams) *ScoringParams {
	if params == nil {
		return nil
	}
	return params.Clone()
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
