package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// RegisterLegacyOutcomeAdapters returns a registry copy with characterization-only legacy outcome adapters.
func RegisterLegacyOutcomeAdapters(registry OutcomeAdapterRegistry) OutcomeAdapterRegistry {
	return registry.
		Register(modeltypology.DetailAdapterMBTI, assemblePersonalityTypeFromMBTI).
		Register(modeltypology.DetailAdapterSBTI, assemblePersonalityTypeFromSBTI).
		Register(modeltypology.DetailAdapterBigFive, assembleTraitProfileOutcome)
}

func assembleTraitProfileOutcome(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.BigFiveResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromTraitProfile(modelRef, evaluationtypology.TraitProfileDetailFromBigFive(detail)), nil
}

func assemblePersonalityTypeFromMBTI(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromPersonalityType(modelRef, evaluationtypology.PersonalityTypeDetailFromMBTI(detail)), nil
}

func assemblePersonalityTypeFromSBTI(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	detail, err := evaluationtypology.SBTIResultDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromPersonalityType(modelRef, evaluationtypology.PersonalityTypeDetailFromSBTI(detail)), nil
}
