package answersheet

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidateReadiness checks the submission ports after composition completes
// the catalog and answering-start bindings. It does not perform external I/O.
func (s *submissionService) ValidateReadiness() error {
	if s == nil || s.repo == nil || s.reader == nil || s.questionnaireRepo == nil || s.durableStore == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "survey submission persistence dependencies are not configured")
	}
	if s.binding == nil || s.attribution == nil || s.startResolver == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "survey submission admission dependencies are not configured")
	}
	return nil
}
