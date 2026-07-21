package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// resolveAdmission freezes evaluation intent at accept time (EV-R001).
// When the binding resolver is not wired yet, returns zero admission so Journey
// can fall back to legacy live binding for that rare window.
func (s *submissionService) resolveAdmission(ctx context.Context, questionnaireCode, questionnaireVersion string) (domainanswersheet.Admission, error) {
	if s.binding == nil {
		return domainanswersheet.Admission{}, nil
	}
	binding, ok, err := s.binding.ResolveAssessmentBinding(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return domainanswersheet.Admission{}, errors.WrapC(err, errorCode.ErrDatabase, "解析测评准入绑定失败")
	}
	if !ok {
		return domainanswersheet.NewIndependentAdmission(questionnaireCode, questionnaireVersion)
	}
	return domainanswersheet.NewAssessmentAdmission(
		questionnaireCode,
		questionnaireVersion,
		binding.Ref.Kind.String(),
		binding.Ref.SubKind.String(),
		binding.Ref.Algorithm.String(),
		binding.Ref.Code,
		binding.Ref.Version,
		binding.Ref.Title,
	)
}

// SetAssessmentBindingResolver injects the catalog binding resolver after
// modelcatalog module initialization (EV-R001).
func (s *submissionService) SetAssessmentBindingResolver(binding rulesetport.AssessmentBindingResolver) {
	if s == nil {
		return
	}
	s.binding = binding
}

// AssessmentBindingInjector is implemented by submission services that accept
// late binding of the assessment admission resolver.
type AssessmentBindingInjector interface {
	SetAssessmentBindingResolver(rulesetport.AssessmentBindingResolver)
}
