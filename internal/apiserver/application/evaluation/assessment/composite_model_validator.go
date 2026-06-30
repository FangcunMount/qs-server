package assessment

import (
	"context"

	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

type compositeEvaluationModelValidator struct {
	validators []evalassessment.EvaluationModelValidator
}

func NewCompositeEvaluationModelValidator(validators ...evalassessment.EvaluationModelValidator) evalassessment.EvaluationModelValidator {
	filtered := make([]evalassessment.EvaluationModelValidator, 0, len(validators))
	for _, validator := range validators {
		if validator != nil {
			filtered = append(filtered, validator)
		}
	}
	return compositeEvaluationModelValidator{validators: filtered}
}

func (v compositeEvaluationModelValidator) ValidateEvaluationModel(
	ctx context.Context,
	modelRef evalassessment.EvaluationModelRef,
	questionnaireRef evalassessment.QuestionnaireRef,
) error {
	for _, validator := range v.validators {
		if err := validator.ValidateEvaluationModel(ctx, modelRef, questionnaireRef); err != nil {
			return err
		}
	}
	return nil
}
