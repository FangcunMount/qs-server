package mbti

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Adapter implements the personality typology model adapter for MBTI.
type Adapter struct{}

func (Adapter) Algorithm() assessmentmodel.Algorithm {
	return assessmentmodel.AlgorithmMBTI
}

func (Adapter) BuildOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *evaluationinput.AnswerSheet,
) (*assessment.AssessmentOutcome, error) {
	model, err := modeltypology.ToMBTI(payload)
	if err != nil {
		return nil, err
	}
	detail, err := Score(model, sheet)
	if err != nil {
		return nil, err
	}
	return assessmentOutcomeFromDetail(modelRef, detail), nil
}

func assessmentOutcomeFromDetail(modelRef assessment.EvaluationModelRef, detail evaluationtypology.MBTIResultDetail) *assessment.AssessmentOutcome {
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindPersonality,
		Payload: detail,
	})
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindMatchPercent,
		Value: detail.MatchPercent,
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
		Strengths:   append([]string(nil), detail.Profile.Strengths...),
		Weaknesses:  append([]string(nil), detail.Profile.Weaknesses...),
		Suggestions: append([]string(nil), detail.Profile.Suggestions...),
	}
	return outcome
}
