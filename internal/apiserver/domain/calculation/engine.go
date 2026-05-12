package calculation

import (
	"context"
	"fmt"
)

type Dimension struct {
	Code            string
	ScoringStrategy string
}

type StrategyRegistry interface {
	Score(ctx context.Context, dimension Dimension, values []float64) (float64, error)
}

type Engine struct {
	registry StrategyRegistry
}

func NewEngine(registry StrategyRegistry) *Engine {
	if registry == nil {
		registry = DefaultStrategyRegistry{}
	}
	return &Engine{registry: registry}
}

func (e *Engine) ScoreDimension(ctx context.Context, dimension Dimension, values []float64) (float64, error) {
	if e == nil || e.registry == nil {
		return 0, fmt.Errorf("calculation strategy registry is not configured")
	}
	return e.registry.Score(ctx, dimension, values)
}
