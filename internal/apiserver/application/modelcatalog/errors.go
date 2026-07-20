package modelcatalog

import (
	stderrors "errors"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

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

// MapDraftWriteError maps draft persistence failures to stable application errors.
func MapDraftWriteError(err error) error {
	if domain.IsRevisionConflict(err) {
		return errors.WithCode(code.ErrConflict, "assessment model revision conflict; refresh and retry")
	}
	return err
}
