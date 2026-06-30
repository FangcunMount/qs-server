package personality

import stderrors "errors"

type validationFailedError struct {
	issues []ValidationIssue
}

func (e *validationFailedError) Error() string {
	return firstIssueMessage(e.issues)
}

func AsValidationFailed(err error) ([]ValidationIssue, bool) {
	var target *validationFailedError
	if stderrors.As(err, &target) {
		return append([]ValidationIssue(nil), target.issues...), true
	}
	return nil, false
}
