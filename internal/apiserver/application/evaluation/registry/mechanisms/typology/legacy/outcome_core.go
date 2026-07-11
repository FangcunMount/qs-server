package legacy

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// AssemblePersonalityTypeOutcome builds assessment outcome from mechanism-neutral detail.
func AssemblePersonalityTypeOutcome(modelRef assessment.EvaluationModelRef, detail outcometypology.PersonalityTypeDetail) *domainoutcome.Execution {
	score := detail.MatchPercent
	if score == 0 && detail.Similarity > 0 {
		score = detail.Similarity * 100
	}
	outcome := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{
		PrimaryLabel: detail.TypeCode,
		Score:        scorePtr(score),
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}, domainoutcome.Detail{
		Kind:    modelRef.Kind(),
		Payload: detail,
	})
	outcome.Primary = &domainoutcome.ScoreValue{
		Kind:  domainoutcome.ScoreKindMatchPercent,
		Value: score,
		Label: detail.TypeCode,
	}
	outcome.Level = &domainoutcome.ResultLevel{
		Code:     detail.TypeCode,
		Label:    detail.TypeName,
		Severity: "none",
	}
	outcome.Profile = &domainoutcome.ProfileResult{
		Kind: domainoutcome.ProfileKindPersonalityType,
		Code: detail.TypeCode,
		Name: detail.TypeName,
	}
	return outcome
}

// AssembleTraitProfileOutcome builds assessment outcome from mechanism-neutral trait profile.
func AssembleTraitProfileOutcome(modelRef assessment.EvaluationModelRef, detail outcometypology.TraitProfileDetail) *domainoutcome.Execution {
	primaryLabel := "trait_profile"
	if len(detail.Traits) > 0 {
		primaryLabel = detail.Traits[0].Code
	}
	outcome := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{
		PrimaryLabel: primaryLabel,
	}, domainoutcome.Detail{
		Kind:    modelRef.Kind(),
		Payload: detail,
	})
	if len(detail.Traits) > 0 {
		outcome.Profile = &domainoutcome.ProfileResult{
			Kind: domainoutcome.ProfileKindPersonalityTrait,
			Code: detail.Traits[0].Code,
			Name: detail.Traits[0].Name,
		}
	}
	return outcome
}

func scorePtr(score float64) *float64 {
	if score == 0 {
		return nil
	}
	return &score
}
