package interpretation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type ScaleExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

type InputValidator interface {
	Validate(input ScaleExecutionInput) error
}

type DefaultInputValidator struct{}

func (DefaultInputValidator) Validate(input ScaleExecutionInput) error {
	if input.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if !input.Assessment.Status().IsSubmitted() {
		return fmt.Errorf("assessment is not submitted")
	}
	if input.Input == nil {
		return fmt.Errorf("evaluation input snapshot is required")
	}
	scale := input.Input.MedicalScale
	if scale == nil {
		return fmt.Errorf("medical scale is required")
	}
	if len(scale.Factors) == 0 {
		return fmt.Errorf("medical scale has no factors")
	}
	if !scale.IsPublished() {
		return fmt.Errorf("medical scale is not published")
	}
	if scale.QuestionnaireCode != input.Assessment.QuestionnaireRef().Code().String() {
		return fmt.Errorf("medical scale does not match the questionnaire")
	}
	if input.Input.AnswerSheet == nil {
		return fmt.Errorf("answer sheet not found")
	}
	return nil
}
