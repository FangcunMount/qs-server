package registry

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
	switch key.Kind {
	case modelcatalog.KindBehavioralRating:
		return MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
			DecisionKind:    modelcatalog.DecisionKindNormLookup,
			ReportType:      reportType,
		}, true
	case modelcatalog.KindCognitive:
		return MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
			DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
			ReportType:      reportType,
		}, true
	}
	family, ok := modelcatalog.AlgorithmFamilyFromIdentity(key.Kind, key.SubKind, key.Algorithm)
	if !ok {
		return MechanismReportBuilderKey{}, false
	}
	decision := defaultDecisionKindForFamily(family)
	if decision == "" {
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

// MechanismKeyFallbackCandidates returns progressively broader lookup keys for registry resolution.
func MechanismKeyFallbackCandidates(key MechanismReportBuilderKey) []MechanismReportBuilderKey {
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	base := []MechanismReportBuilderKey{
		key,
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ProductChannel: key.ProductChannel, Audience: key.Audience, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ProductChannel: key.ProductChannel, Audience: key.Audience},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ProductChannel: key.ProductChannel, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ProductChannel: key.ProductChannel},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, Audience: key.Audience, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, Audience: key.Audience},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel, Audience: key.Audience, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel, Audience: key.Audience},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Audience: key.Audience, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Audience: key.Audience},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType},
		{AlgorithmFamily: key.AlgorithmFamily, ReportType: key.ReportType},
	}
	return dedupeMechanismKeys(base)
}

func dedupeMechanismKeys(keys []MechanismReportBuilderKey) []MechanismReportBuilderKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]MechanismReportBuilderKey, 0, len(keys))
	seen := make(map[MechanismReportBuilderKey]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func (r *mutableReportBuilderRegistry) resolveReportBuilder(key evaluation.ExecutionIdentity, reportType domainReport.ReportType) (ReportBuilder, error) {
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	mechanismKey, ok := MechanismReportBuilderKeyFromExecutionIdentity(key, reportType)
	if !ok {
		return nil, fmt.Errorf("unsupported interpretation report builder key: %s report type: %s", key, reportType)
	}
	return r.ResolveByMechanism(mechanismKey)
}
