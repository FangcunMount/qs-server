package execute

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func canonicalExecutionIdentityForFamily(family modelcatalog.AlgorithmFamily) (evaluation.ExecutionIdentity, bool) {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return evaluation.ExecutionIdentityScaleDefault, true
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return evaluation.ExecutionIdentityPersonalityTypology, true
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return evaluation.ExecutionIdentityBehavioralRatingDefault, true
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return evaluation.ExecutionIdentityCognitiveDefault, true
	default:
		return evaluation.ExecutionIdentity{}, false
	}
}
