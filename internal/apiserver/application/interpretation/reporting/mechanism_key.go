package reporting

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

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
	return MechanismReportBuilderKeyFromEvaluatorKey(ResolveOutcomeKey(outcome), resolveReportType(outcome))
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
