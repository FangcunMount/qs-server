package legacy

import (
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// AssemblePersonalityTypeOutcome builds assessment outcome from mechanism-neutral detail.
func AssemblePersonalityTypeOutcome(modelRef assessment.EvaluationModelRef, detail outcometypology.PersonalityTypeDetail) *assessment.AssessmentOutcome {
	score := detail.MatchPercent
	if score == 0 && detail.Similarity > 0 {
		score = detail.Similarity * 100
	}
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Score:        scorePtr(score),
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindPersonality,
		Payload: detail,
	})
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindMatchPercent,
		Value: score,
		Label: detail.TypeCode,
	}
	outcome.Level = &assessment.OutcomeResultLevel{
		Code:     detail.TypeCode,
		Label:    detail.TypeName,
		Severity: "none",
	}
	outcome.Profile = &assessment.ProfileResult{
		Kind:        assessment.ProfileKindPersonalityType,
		Code:        detail.TypeCode,
		Name:        detail.TypeName,
		Summary:     detail.OneLiner,
		Strengths:   append([]string(nil), detail.Strengths...),
		Weaknesses:  append([]string(nil), detail.Weaknesses...),
		Suggestions: append([]string(nil), detail.Suggestions...),
	}
	return outcome
}

// AssembleTraitProfileOutcome builds assessment outcome from mechanism-neutral trait profile.
func AssembleTraitProfileOutcome(modelRef assessment.EvaluationModelRef, detail outcometypology.TraitProfileDetail) *assessment.AssessmentOutcome {
	primaryLabel := "trait_profile"
	if len(detail.Traits) > 0 {
		primaryLabel = detail.Traits[0].Code
	}
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{
		PrimaryLabel: primaryLabel,
	}, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindPersonality,
		Payload: detail,
	})
	if len(detail.Traits) > 0 {
		outcome.Profile = &assessment.ProfileResult{
			Kind:    assessment.ProfileKindPersonalityTrait,
			Code:    detail.Traits[0].Code,
			Name:    detail.Traits[0].Name,
			Summary: detail.Traits[0].Name,
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
