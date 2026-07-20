package binding

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// PublishedQuestionnairePolicy is the shared bind/publish baseline for families
// that require a published questionnaire with at least one question (MC-R009).
// It does not invent questionnaire type or exclusivity rules.
type PublishedQuestionnairePolicy struct {
	Kind           domain.Kind
	Questionnaires questionnaireapp.QuestionnaireQueryService
}

// Supports reports whether this policy owns the given identity kind.
func (p PublishedQuestionnairePolicy) Supports(identity domain.Identity) bool {
	return identity.Kind == p.Kind
}

// Validate requires a published questionnaire version that contains questions.
func (p PublishedQuestionnairePolicy) Validate(ctx context.Context, _ *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	if binding.QuestionnaireCode == "" {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "questionnaire code is required")
	}
	if p.Questionnaires == nil {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInternalServerError, "questionnaire query service is not configured")
	}
	var (
		result *questionnaireapp.QuestionnaireResult
		err    error
	)
	if binding.QuestionnaireVersion != "" {
		result, err = p.Questionnaires.GetPublishedByCodeVersion(ctx, binding.QuestionnaireCode, binding.QuestionnaireVersion)
	} else {
		result, err = p.Questionnaires.GetPublishedByCode(ctx, binding.QuestionnaireCode)
	}
	if err != nil || result == nil || result.Version == "" {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "binding questionnaire is invalid")
	}
	if len(result.Questions) == 0 {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "binding questionnaire must contain questions")
	}
	return domain.QuestionnaireBinding{QuestionnaireCode: result.Code, QuestionnaireVersion: result.Version}, nil
}

// BeforePublish re-validates the bound published questionnaire at release time.
func (p PublishedQuestionnairePolicy) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	if model == nil || model.Binding.QuestionnaireCode == "" {
		return nil
	}
	_, err := p.Validate(ctx, model, model.Binding)
	return err
}
