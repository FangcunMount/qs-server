package reporting

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKeyFromRuntimeDescriptorKey maps runtime descriptor routing to report builders.
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

// MechanismReportBuilderKeyFromEvaluatorKey derives the mechanism routing key from an evaluator key.
func MechanismReportBuilderKeyFromEvaluatorKey(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (MechanismReportBuilderKey, bool) {
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

// MechanismReportBuilderKeyFromOutcome derives the mechanism routing key from a scored outcome.
func MechanismReportBuilderKeyFromOutcome(outcome evaloutcome.Outcome) (MechanismReportBuilderKey, bool) {
	reportType := resolveReportType(outcome)
	if !outcome.RuntimeDescriptorKey.IsZero() {
		return MechanismReportBuilderKeyFromRuntimeDescriptorKey(outcome.RuntimeDescriptorKey, reportType)
	}
	if snapshot, ok := evaloutcome.PublishedSnapshotFromInput(outcome.Input); ok {
		routingKey, err := evalpipeline.ExecutionRoutingFromSnapshot(snapshot)
		if err == nil {
			return MechanismReportBuilderKeyFromRuntimeDescriptorKey(routingKey, reportType)
		}
	}
	return MechanismReportBuilderKey{}, false
}

func (r *mutableReportBuilderRegistry) resolveReportBuilder(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (ReportBuilder, error) {
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	if mechanismKey, ok := MechanismReportBuilderKeyFromEvaluatorKey(key, reportType); ok {
		if builder, err := r.ResolveByMechanism(mechanismKey); err == nil {
			return builder, nil
		}
	}
	return r.resolveByEvaluatorKey(key, reportType)
}
