package reporting

import (
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKeyFromRuntimeDescriptorKey 映射运行时描述符 路由 到 报告构建器。
func MechanismReportBuilderKeyFromRuntimeDescriptorKey(
	key evalpipeline.RuntimeDescriptorKey,
	reportType domainReport.ReportType,
) (MechanismReportBuilderKey, bool) {
	if key.IsZero() {
		return MechanismReportBuilderKey{}, false
	}
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	decision := key.DecisionKind
	if decision == "" {
		decision = defaultDecisionKindForFamily(key.AlgorithmFamily)
	}
	if decision == "" {
		return MechanismReportBuilderKey{}, false
	}
	return MechanismReportBuilderKey{
		AlgorithmFamily: key.AlgorithmFamily,
		DecisionKind:    decision,
		ReportType:      reportType,
	}, true
}

func defaultDecisionKindForFamily(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.DecisionKindScoreRange
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.DecisionKindPoleComposition
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.DecisionKindNormLookup
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.DecisionKindAbilityLevel
	default:
		return ""
	}
}

// MechanismReportBuilderKeyFromExecutionIdentity 推导机制 路由 键 从 评估器键。
func MechanismReportBuilderKeyFromExecutionIdentity(key evaluation.ExecutionIdentity, reportType domainReport.ReportType) (MechanismReportBuilderKey, bool) {
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	family, ok := modelcatalog.AlgorithmFamilyFromIdentity(key.Kind, key.SubKind, key.Algorithm)
	if !ok {
		return MechanismReportBuilderKey{}, false
	}
	decision, ok := modelcatalog.DecisionKindForIdentity(key.Kind, key.SubKind, key.Algorithm)
	if !ok {
		return MechanismReportBuilderKey{}, false
	}
	return MechanismReportBuilderKey{
		AlgorithmFamily: family,
		DecisionKind:    decision,
		ReportType:      reportType,
	}, true
}

// MechanismReportBuilderKeyFromOutcome 推导机制 路由 键 从 scored 结果。
func MechanismReportBuilderKeyFromOutcome(outcome evaloutcome.Outcome) (MechanismReportBuilderKey, bool) {
	ctx, ok := ReportRoutingContextFromOutcome(outcome)
	if !ok {
		return MechanismReportBuilderKey{}, false
	}
	return ctx.MechanismKey()
}

// mechanismKeyFallbackCandidates returns progressively broader lookup keys for registry resolution.
func mechanismKeyFallbackCandidates(key MechanismReportBuilderKey) []MechanismReportBuilderKey {
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	return []MechanismReportBuilderKey{
		key,
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType},
		{AlgorithmFamily: key.AlgorithmFamily, ReportType: key.ReportType},
	}
}

func (r *mutableReportBuilderRegistry) resolveReportBuilder(key evaluation.ExecutionIdentity, reportType domainReport.ReportType) (ReportBuilder, error) {
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	if mechanismKey, ok := MechanismReportBuilderKeyFromExecutionIdentity(key, reportType); ok {
		if builder, err := r.ResolveByMechanism(mechanismKey); err == nil {
			return builder, nil
		}
	}
	return r.resolveByEvaluatorKey(key, reportType)
}

func (r *mutableReportBuilderRegistry) resolveByEvaluatorKey(key evaluation.ExecutionIdentity, reportType domainReport.ReportType) (ReportBuilder, error) {
	registryKey := reportBuilderKey{key: key, reportType: reportType}
	if builder, ok := r.items[registryKey]; ok {
		return builder, nil
	}
	if routed := evaluation.ResolvePersonalityTypologyExecutorIdentity(key); routed != key {
		registryKey.key = routed
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	if routed := evaluation.ResolveBehavioralRatingExecutorIdentity(key); routed != key {
		registryKey.key = routed
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported interpretation report builder key: %s report type: %s", key, reportType)
}
