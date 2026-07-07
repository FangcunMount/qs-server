// Package factor_classification is the mechanism-oriented domain entry for typology execution.
// Implementation lives in domain/evaluation/personality/configured during migration.
package factor_classification

import (
	evalconfigured "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/configured"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Evaluator runs configured typology scoring.
type Evaluator = evalconfigured.Evaluator

// NewEvaluator creates a configured typology evaluator.
var NewEvaluator = evalconfigured.NewEvaluator

// AlgorithmFamily is the mechanism family for this package.
const AlgorithmFamily = modelcatalog.AlgorithmFamilyFactorClassification
