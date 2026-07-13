package policy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

const (
	ReportProfileScale           ReportProfile = "scale"
	ReportProfileNorm            ReportProfile = "norm"
	ReportProfileTask            ReportProfile = "task"
	ReportProfilePersonalityType ReportProfile = "personality_type"
	ReportProfileTrait           ReportProfile = "trait_profile"
	ReportProfilePattern         ReportProfile = "pattern_profile"
)

// ReportProfileForDecisionKind maps execution decision kind to report presentation profile.
func ReportProfileForDecisionKind(decision modelcatalog.DecisionKind) ReportProfile {
	switch decision {
	case modelcatalog.DecisionKindScoreRange, modelcatalog.DecisionKindScoreRangeInterpretation:
		return ReportProfileScale
	case modelcatalog.DecisionKindNormLookup:
		return ReportProfileNorm
	case modelcatalog.DecisionKindAbilityLevel:
		return ReportProfileTask
	case modelcatalog.DecisionKindPoleComposition, modelcatalog.DecisionKindDominantFactor:
		return ReportProfilePersonalityType
	case modelcatalog.DecisionKindTraitProfile:
		return ReportProfileTrait
	case modelcatalog.DecisionKindNearestPattern:
		return ReportProfilePattern
	default:
		return ReportProfileDefault
	}
}

// DefaultDecisionKind returns the compatibility decision for an algorithm
// family when an older frozen outcome did not persist one explicitly.
func DefaultDecisionKind(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
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
