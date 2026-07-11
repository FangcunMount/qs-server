package reporting

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func ExecutionPathForMechanismFamily(family modelcatalog.AlgorithmFamily) (modelcatalog.ExecutionPath, bool) {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.ExecutionPathScaleDescriptor, true
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.ExecutionPathTypologyDescriptor, true
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.ExecutionPathBehavioralRatingDescriptor, true
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.ExecutionPathCognitiveDescriptor, true
	default:
		return "", false
	}
}

func ExecutionPathForReportBuilder(builder ReportBuilder) (modelcatalog.ExecutionPath, error) {
	if builder == nil {
		return "", fmt.Errorf("interpretation report builder is nil")
	}
	keyed, ok := builder.(MechanismKeyedReportBuilder)
	if !ok {
		return "", fmt.Errorf("report builder has no mechanism key")
	}
	if path, found := ExecutionPathForMechanismFamily(keyed.MechanismKey().AlgorithmFamily); found {
		return path, nil
	}
	return "", fmt.Errorf("unsupported report builder execution path")
}
