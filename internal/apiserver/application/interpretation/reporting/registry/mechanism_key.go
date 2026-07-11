package registry

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

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

// MechanismReportBuilderKeyFromInput resolves a mechanism route from
// Interpretation-owned input without falling back to Evaluation aggregates.
func MechanismReportBuilderKeyFromInput(input interpinput.InterpretationInput) (MechanismReportBuilderKey, bool) {
	ctx, ok := ReportRoutingContextFromInput(input)
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
	if key.TemplateVersion == "" {
		key.TemplateVersion = policy.TemplateVersionV1
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
	for i := range base {
		base[i].TemplateVersion = key.TemplateVersion
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
