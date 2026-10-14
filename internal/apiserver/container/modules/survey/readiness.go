package survey

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidateReadiness runs after ModelCatalog completes Survey's late bindings.
// Optional name, hotset, cache-signal and QR-code services are outside this gate.
func (m *Module) ValidateReadiness() error {
	switch {
	case m == nil:
		return missingApplicationService("module")
	case m.Questionnaire == nil:
		return missingApplicationService("questionnaire module")
	case m.AnswerSheet == nil:
		return missingApplicationService("answer sheet module")
	case m.AnsweringStart == nil:
		return missingApplicationService("answering start service")
	case m.Questionnaire.LifecycleService == nil:
		return missingApplicationService("questionnaire lifecycle service")
	case m.Questionnaire.ContentService == nil:
		return missingApplicationService("questionnaire content service")
	case m.Questionnaire.QueryService == nil:
		return missingApplicationService("questionnaire query service")
	case m.AnswerSheet.SubmissionService == nil:
		return missingApplicationService("answer sheet submission service")
	case m.AnswerSheet.ManagementService == nil:
		return missingApplicationService("answer sheet management service")
	case m.AnswerSheet.ScoringService == nil:
		return missingApplicationService("answer sheet scoring service")
	}
	if err := m.bindingSyncer.validateReadiness(); err != nil {
		return err
	}
	ready, ok := m.AnswerSheet.SubmissionService.(interface{ ValidateReadiness() error })
	if !ok {
		return missingApplicationService("submission readiness validator")
	}
	return ready.ValidateReadiness()
}

func missingApplicationService(dependency string) error {
	logger.L(context.Background()).Errorw("required Survey application service is missing", "dependency", dependency)
	return errors.WithCode(code.ErrModuleInitializationFailed, "survey %s is not configured", dependency)
}
