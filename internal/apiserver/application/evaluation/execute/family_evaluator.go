package execute

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func canonicalEvaluatorKeyForFamily(family modelcatalog.AlgorithmFamily) (evaluation.EvaluatorKey, bool) {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return evaluation.EvaluatorKeyScaleDefault, true
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return evaluation.EvaluatorKeyPersonalityTypology, true
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return evaluation.EvaluatorKeyBehavioralRatingDefault, true
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return evaluation.EvaluatorKeyCognitiveDefault, true
	default:
		return evaluation.EvaluatorKey{}, false
	}
}
