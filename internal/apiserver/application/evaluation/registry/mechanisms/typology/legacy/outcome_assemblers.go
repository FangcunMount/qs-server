package legacy

import (
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// AssemblePersonalityTypeFromMBTI converts legacy MBTI detail payload to assessment outcome.
func AssemblePersonalityTypeFromMBTI(
	modelRef assessment.EvaluationModelRef,
	result outcometypology.ScoringResult,
) (*domainoutcome.Execution, error) {
	detail, err := MBTIResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return AssemblePersonalityTypeOutcome(modelRef, PersonalityTypeDetailFromMBTI(detail)), nil
}

// AssemblePersonalityTypeFromSBTI converts legacy SBTI detail payload to assessment outcome.
func AssemblePersonalityTypeFromSBTI(
	modelRef assessment.EvaluationModelRef,
	result outcometypology.ScoringResult,
) (*domainoutcome.Execution, error) {
	detail, err := SBTIResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return AssemblePersonalityTypeOutcome(modelRef, PersonalityTypeDetailFromSBTI(detail)), nil
}

// AssembleTraitProfileFromBigFive converts legacy BigFive detail payload to assessment outcome.
func AssembleTraitProfileFromBigFive(
	modelRef assessment.EvaluationModelRef,
	result outcometypology.ScoringResult,
) (*domainoutcome.Execution, error) {
	detail, err := BigFiveResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return AssembleTraitProfileOutcome(modelRef, TraitProfileDetailFromBigFive(detail)), nil
}
