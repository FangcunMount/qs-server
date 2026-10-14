package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidateReadiness checks the submission ports after composition completes
// the catalog and answering-start bindings. It does not perform external I/O.
func (s *submissionService) ValidateReadiness() error {
	switch {
	case s == nil:
		return missingSubmissionDependency("submission service")
	case s.repo == nil:
		return missingSubmissionDependency("answer sheet repository")
	case s.reader == nil:
		return missingSubmissionDependency("answer sheet reader")
	case s.questionnaireRepo == nil:
		return missingSubmissionDependency("questionnaire repository")
	case s.durableStore == nil:
		return missingSubmissionDependency("durable writer")
	case s.binding == nil:
		return missingSubmissionDependency("assessment binding resolver")
	case s.attribution == nil:
		return missingSubmissionDependency("attribution resolver")
	case s.startResolver == nil:
		return missingSubmissionDependency("answering start resolver")
	}
	return nil
}

func missingSubmissionDependency(dependency string) error {
	logger.L(context.Background()).Errorw("required Survey submission dependency is missing", "dependency", dependency)
	return errors.WithCode(code.ErrModuleInitializationFailed, "survey submission: %s is required", dependency)
}
