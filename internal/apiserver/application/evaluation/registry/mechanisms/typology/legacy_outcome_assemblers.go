package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// RegisterLegacyOutcomeAdapters 返回注册表副本 使用 仅用于表征 旧版 结果 adapters。
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
