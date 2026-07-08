package typology

import (
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
)

func assessmentOutcomeFromPersonalityType(modelRef assessment.EvaluationModelRef, detail evaluationtypology.PersonalityTypeDetail) *assessment.AssessmentOutcome {
	return typologylegacy.AssemblePersonalityTypeOutcome(modelRef, detail)
}

func assessmentOutcomeFromTraitProfile(modelRef assessment.EvaluationModelRef, detail evaluationtypology.TraitProfileDetail) *assessment.AssessmentOutcome {
	return typologylegacy.AssembleTraitProfileOutcome(modelRef, detail)
}
