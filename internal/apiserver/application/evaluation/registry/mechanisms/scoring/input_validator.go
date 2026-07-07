package scoring

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ExecutionInput is the validated input for a factor-scoring evaluation run.
type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

// InputValidator validates factor-scoring execution input.
type InputValidator interface {
	Validate(input ExecutionInput) error
}

// DefaultInputValidator is the production input validator for factor-scoring runs.
type DefaultInputValidator struct{}

func (DefaultInputValidator) Validate(input ExecutionInput) error {
	if input.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if !input.Assessment.Status().IsSubmitted() {
		return fmt.Errorf("assessment is not submitted")
	}
	if input.Input == nil {
		return fmt.Errorf("evaluation input snapshot is required")
	}
	scale, ok := evaluationinput.ScalePayload(input.Input)
	if !ok || scale == nil {
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
	if modelRef := input.Assessment.EvaluationModelRef(); modelRef != nil && modelRef.IsScale() {
		if modelRef.Version() != "" && scale.ScaleVersion != "" && modelRef.Version() != scale.ScaleVersion {
			return fmt.Errorf("medical scale version does not match the evaluation model")
		}
	}
	if input.Input.AnswerSheet == nil {
		return fmt.Errorf("answer sheet not found")
	}
	if input.Input.AnswerSheet.QuestionnaireCode != input.Assessment.QuestionnaireRef().Code().String() {
		return fmt.Errorf("answer sheet does not match the questionnaire")
	}
	if err := requireSameQuestionnaireVersion("answer sheet", input.Input.AnswerSheet.QuestionnaireVersion, input.Assessment.QuestionnaireRef().Version()); err != nil {
		return err
	}
	if input.Input.Questionnaire == nil {
		return fmt.Errorf("questionnaire snapshot not found")
	}
	if input.Input.Questionnaire.Code != input.Assessment.QuestionnaireRef().Code().String() {
		return fmt.Errorf("questionnaire snapshot does not match the assessment questionnaire")
	}
	if err := requireSameQuestionnaireVersion("questionnaire snapshot", input.Input.Questionnaire.Version, input.Assessment.QuestionnaireRef().Version()); err != nil {
		return err
	}
	if err := requireSameQuestionnaireVersion("medical scale", scale.QuestionnaireVersion, input.Input.AnswerSheet.QuestionnaireVersion); err != nil {
		return err
	}
	if err := requireSameQuestionnaireVersion("medical scale", scale.QuestionnaireVersion, input.Input.Questionnaire.Version); err != nil {
		return err
	}
	return nil
}

func requireSameQuestionnaireVersion(label, got, want string) error {
	if got == "" || want == "" {
		return fmt.Errorf("%s questionnaire version is required", label)
	}
	if got != want {
		return fmt.Errorf("%s questionnaire version does not match", label)
	}
	return nil
}
