package ruleengine

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// AnswerValidator adapts the current validation batch engine to the application port.
type AnswerValidator struct {
	batch *validation.BatchValidator
}

// NewAnswerValidator creates a validation engine adapter.
func NewAnswerValidator(batch *validation.BatchValidator) *AnswerValidator {
	if batch == nil {
		batch = validation.NewBatchValidator()
	}
	return &AnswerValidator{batch: batch}
}

// ValidateAnswers executes answer validation with the current domain rule engine.
func (v *AnswerValidator) ValidateAnswers(_ context.Context, tasks []ruleengineport.AnswerValidationTask) ([]ruleengineport.AnswerValidationResult, error) {
	validationTasks := make([]validation.ValidationTask, 0, len(tasks))
	for _, task := range tasks {
		validationTasks = append(validationTasks, validation.ValidationTask{
			ID:    task.ID,
			Value: task.Value,
			Rules: task.Rules,
		})
	}

	results := v.batch.ValidateAll(validationTasks)
	output := make([]ruleengineport.AnswerValidationResult, 0, len(results))
	for _, result := range results {
		item := ruleengineport.AnswerValidationResult{
			ID:    result.ID,
			Valid: result.Result == nil || result.Result.IsValid(),
		}
		if result.Result != nil {
			for _, err := range result.Result.GetErrors() {
				item.Errors = append(item.Errors, ruleengineport.ValidationError{
					RuleType: err.GetRuleType(),
					Message:  err.GetMessage(),
				})
			}
		}
		output = append(output, item)
	}
	return output, nil
}
