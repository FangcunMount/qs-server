package modelcatalog

import stderrors "errors"

type ValidationFailedError struct {
	Result *ValidationResult
}

func (e *ValidationFailedError) Error() string {
	if e.Result != nil && len(e.Result.Issues) > 0 {
		return e.Result.Issues[0].Message
	}
	return "模型校验失败"
}

func NewValidationFailedError(issues []ValidationIssue) error {
	return &ValidationFailedError{Result: NewValidationResult(issues)}
}

func ValidationFailedFrom(err error) (*ValidationFailedError, bool) {
	var target *ValidationFailedError
	if stderrors.As(err, &target) {
		return target, true
	}
	return nil, false
}
