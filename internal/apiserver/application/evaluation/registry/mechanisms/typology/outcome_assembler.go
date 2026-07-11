package typology

import (
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func executionFromPersonalityType(modelRef assessment.EvaluationModelRef, detail outcometypology.PersonalityTypeDetail) *domainoutcome.Execution {
	return typologylegacy.AssemblePersonalityTypeOutcome(modelRef, detail)
}

func executionFromTraitProfile(modelRef assessment.EvaluationModelRef, detail outcometypology.TraitProfileDetail) *domainoutcome.Execution {
	return typologylegacy.AssembleTraitProfileOutcome(modelRef, detail)
}
