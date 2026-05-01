package interpretengine

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

type Interpreter struct {
	strategies          map[interpretengine.StrategyType]strategy
	compositeStrategies map[interpretengine.StrategyType]compositeStrategy
}

func NewInterpreter() *Interpreter {
	i := &Interpreter{
		strategies:          make(map[interpretengine.StrategyType]strategy),
		compositeStrategies: make(map[interpretengine.StrategyType]compositeStrategy),
	}
	i.RegisterStrategy(thresholdStrategy{})
	i.RegisterStrategy(rangeStrategy{})
	i.RegisterCompositeStrategy(compositeStrategyImpl{})
	return i
}

func (i *Interpreter) RegisterStrategy(strategy strategy) {
	if strategy == nil {
		return
	}
	i.strategies[strategy.StrategyType()] = strategy
}

func (i *Interpreter) RegisterCompositeStrategy(strategy compositeStrategy) {
	if strategy == nil {
		return
	}
	i.compositeStrategies[strategy.StrategyType()] = strategy
}

func (i *Interpreter) InterpretFactor(score float64, config *interpretengine.Config, strategyType interpretengine.StrategyType) (*interpretengine.Result, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: interpret config is nil", errInvalidConfig)
	}
	strategy := i.strategies[strategyType]
	if strategy == nil {
		return nil, fmt.Errorf("unsupported strategy type: %s", strategyType)
	}
	result, err := strategy.Interpret(score, config)
	if err != nil {
		return nil, fmt.Errorf("interpret failed: %w", err)
	}
	return result, nil
}

func (i *Interpreter) InterpretFactorWithRule(score float64, rule interpretengine.RuleSpec) *interpretengine.Result {
	if !rule.Contains(score) {
		return nil
	}
	return resultFromRule(score, "", rule)
}

func (i *Interpreter) InterpretMultipleFactors(scores []interpretengine.FactorScore, config *interpretengine.CompositeConfig, strategyType interpretengine.StrategyType) (*interpretengine.CompositeResult, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: composite config is nil", errInvalidConfig)
	}
	strategy := i.compositeStrategies[strategyType]
	if strategy == nil {
		return nil, fmt.Errorf("unsupported composite strategy type: %s", strategyType)
	}
	result, err := strategy.InterpretMultiple(scores, config)
	if err != nil {
		return nil, fmt.Errorf("composite interpret failed: %w", err)
	}
	return result, nil
}
