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
	case modelcatalog.DecisionKindScoreRange:
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
