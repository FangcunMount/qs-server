package typology

import (
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func assessmentOutcomeFromPersonalityType(modelRef assessment.EvaluationModelRef, detail outcometypology.PersonalityTypeDetail) *assessment.AssessmentOutcome {
	return typologylegacy.AssemblePersonalityTypeOutcome(modelRef, detail)
}

func assessmentOutcomeFromTraitProfile(modelRef assessment.EvaluationModelRef, detail outcometypology.TraitProfileDetail) *assessment.AssessmentOutcome {
	return typologylegacy.AssembleTraitProfileOutcome(modelRef, detail)
}
