package execute

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func familyExecutorsFromRegistry(registry *mutableEvaluatorRegistry) map[modelcatalog.AlgorithmFamily]Evaluator {
	if registry == nil {
		return nil
	}
	out := make(map[modelcatalog.AlgorithmFamily]Evaluator)
	for key, evaluator := range registry.items {
		family, ok := algorithmFamilyForEvaluatorKey(key)
		if !ok {
			continue
		}
		if _, exists := out[family]; !exists {
			out[family] = evaluator
		}
	}
	return out
}

func algorithmFamilyForEvaluatorKey(key evaluation.EvaluatorKey) (modelcatalog.AlgorithmFamily, bool) {
	if key.IsPersonalityTypologyLegacyKey() || key == evaluation.EvaluatorKeyPersonalityTypology {
		return modelcatalog.AlgorithmFamilyFactorClassification, true
	}
	switch key.Kind {
	case modelcatalog.KindScale:
		return modelcatalog.AlgorithmFamilyFactorScoring, true
	case modelcatalog.KindBehavioralRating:
		return modelcatalog.AlgorithmFamilyFactorNorm, true
	case modelcatalog.KindCognitive:
		return modelcatalog.AlgorithmFamilyTaskPerformance, true
	case modelcatalog.KindPersonality:
		if key.SubKind == modelcatalog.SubKindTypology || key.SubKind == "" {
			return modelcatalog.AlgorithmFamilyFactorClassification, true
		}
	}
	return modelcatalog.AlgorithmFamilyFromIdentity(key.Kind, key.SubKind, key.Algorithm)
}

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
