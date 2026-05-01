package ruleengine

import (
	"context"

	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// AnswerValidator executes answer validation through infrastructure strategies.
type AnswerValidator struct {
	strategies validationStrategies
}

// NewAnswerValidator creates a validation engine adapter.
func NewAnswerValidator() *AnswerValidator {
	return &AnswerValidator{strategies: newDefaultValidationStrategies()}
}

// ValidateAnswers executes answer validation with the current rule engine.
func (v *AnswerValidator) ValidateAnswers(_ context.Context, tasks []ruleengineport.AnswerValidationTask) ([]ruleengineport.AnswerValidationResult, error) {
	strategies := v.strategies
	if strategies == nil {
		strategies = newDefaultValidationStrategies()
	}
	output := make([]ruleengineport.AnswerValidationResult, 0, len(tasks))
	for _, task := range tasks {
		item := ruleengineport.AnswerValidationResult{
			ID:    task.ID,
			Valid: true,
		}
		for _, rule := range task.Rules {
			strategy := strategies.Get(rule.RuleType)
			if strategy == nil {
				continue
			}
			if err := strategy.Validate(task.Value, rule); err != nil {
				item.Valid = false
				item.Errors = append(item.Errors, ruleengineport.ValidationError{
					RuleType: string(rule.RuleType),
					Message:  err.Error(),
				})
			}
		}
		output = append(output, item)
	}
	return output, nil
}
