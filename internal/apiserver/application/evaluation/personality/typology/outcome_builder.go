package typology

import (
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func buildMBTIOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	model, err := modeltypology.ToMBTI(payload)
	if err != nil {
		return nil, err
	}
	detail, err := evaluationtypology.ScoreMBTI(model, answerSheetFromPort(sheet))
	if err != nil {
		return nil, err
	}
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
	return outcome, nil
}

func buildSBTIOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	model, err := modeltypology.ToSBTI(payload)
	if err != nil {
		return nil, err
	}
	detail, err := evaluationtypology.ScoreSBTI(model, answerSheetFromPort(sheet))
	if err != nil {
		return nil, err
	}
	score := detail.Similarity * 100
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Score:        &score,
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
		Kind:    assessment.ProfileKindPersonalityType,
		Code:    detail.TypeCode,
		Name:    detail.TypeName,
		Summary: detail.OneLiner,
	}
	return outcome, nil
}

func modelRefFromExecutionInput(input evaluationexecute.ExecutionInput, payload *modeltypology.Payload) assessment.EvaluationModelRef {
	if input.Assessment != nil && input.Assessment.EvaluationModelRef() != nil {
		return *input.Assessment.EvaluationModelRef()
	}
	code := payload.Code
	version := payload.Version
	title := payload.Title
	if code == "" {
		code = string(payload.Algorithm)
	}
	return assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		assessmentmodel.SubKindTypology,
		payload.Algorithm,
		meta.ID(0),
		meta.NewCode(code),
		version,
		title,
	)
}
