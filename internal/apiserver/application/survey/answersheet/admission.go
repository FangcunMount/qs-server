package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// resolveAdmission freezes evaluation intent at accept time (EV-R001).
// The catalog must be wired before accepting a new submission. An unbound
// questionnaire is an explicit independent admission, not a missing dependency.
func (s *submissionService) resolveAdmission(ctx context.Context, questionnaireCode, questionnaireVersion string) (domainanswersheet.Admission, error) {
	if s == nil || s.binding == nil {
		return domainanswersheet.Admission{}, errors.WithCode(errorCode.ErrModuleInitializationFailed, "assessment admission binding resolver is not configured")
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
