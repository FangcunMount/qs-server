// Package factor_scoring is the mechanism-oriented domain entry for factor-scoring execution.
// Implementation lives in domain/evaluation/scale during migration.
package factor_scoring

import (
	evalscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Handler scores scale-like models using the factor-scoring engine.
type Handler = evalscale.Handler

// EvaluateInput is the domain input for factor-scoring execution.
type EvaluateInput = evalscale.EvaluateInput

// ScaleInterpretationResult is the domain scoring outcome for factor-scoring execution.
type ScaleInterpretationResult = evalscale.ScaleInterpretationResult

// ScoringStrategyRegistry scores individual factors during factor-scoring execution.
type ScoringStrategyRegistry = evalscale.ScoringStrategyRegistry

// DefaultScoringStrategyRegistry is the built-in factor scoring registry.
type DefaultScoringStrategyRegistry = evalscale.DefaultScoringStrategyRegistry

// NewHandler creates a factor-scoring handler.
var NewHandler = evalscale.NewHandler

// NewDefaultHandler creates a factor-scoring handler with default dependencies.
var NewDefaultHandler = evalscale.NewDefaultHandler

// AlgorithmFamily is the mechanism family for this package.
const AlgorithmFamily = modelcatalog.AlgorithmFamilyFactorScoring
