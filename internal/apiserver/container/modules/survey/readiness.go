package survey

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidateReadiness runs after ModelCatalog completes Survey's late bindings.
// Optional name, hotset, cache-signal and QR-code services are outside this gate.
func (m *Module) ValidateReadiness() error {
	if m == nil || m.Questionnaire == nil || m.AnswerSheet == nil || m.AnsweringStart == nil ||
		m.Questionnaire.LifecycleService == nil || m.Questionnaire.ContentService == nil || m.Questionnaire.QueryService == nil ||
		m.AnswerSheet.SubmissionService == nil || m.AnswerSheet.ManagementService == nil || m.AnswerSheet.ScoringService == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "survey application services are not configured")
	}
	if err := m.bindingSyncer.validateReadiness(); err != nil {
		return err
	}
	ready, ok := m.AnswerSheet.SubmissionService.(interface{ ValidateReadiness() error })
	if !ok {
		return errors.WithCode(code.ErrModuleInitializationFailed, "survey submission readiness validator is not configured")
	}
	return ready.ValidateReadiness()
}
