package reporting

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionPathForMechanismFamily 映射算法家族 到 its 物化路径。
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

// ExecutionPathForReportBuilder 解析执行路径 用于 报告构建器。
func ExecutionPathForReportBuilder(builder ReportBuilder) (modelcatalog.ExecutionPath, error) {
	if builder == nil {
		return "", fmt.Errorf("interpretation report builder is nil")
	}
	if keyed, ok := builder.(MechanismKeyedReportBuilder); ok {
		if path, ok := ExecutionPathForMechanismFamily(keyed.MechanismKey().AlgorithmFamily); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("unsupported report builder execution path")
}

// ExecutionPathForScoreProjector 解析执行路径 用于 score 投影器。
func ExecutionPathForScoreProjector(projector ScoreProjector) (modelcatalog.ExecutionPath, error) {
	if projector == nil {
		return "", fmt.Errorf("interpretation score projector is nil")
	}
	if keyed, ok := projector.(MechanismKeyedScoreProjector); ok {
		if path, ok := ExecutionPathForMechanismFamily(keyed.MechanismKey().AlgorithmFamily); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("unsupported score projector execution path")
}
