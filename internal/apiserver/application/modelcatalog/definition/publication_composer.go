package definition

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublicationComposerOptions configures the shared publish-validation pipeline.
// Family handlers compose options instead of owning Kind-switched validation trees.
type PublicationComposerOptions struct {
	NormRepo                    port.NormRepository
	QuestionnaireQuery          questionnaireapp.QuestionnaireQueryService
	RequireLegacyDefinition     bool
	LegacyDefinitionMessage     string
	IncludeBehavioralSemantic   bool
	IncludeAlgorithmBinding     bool
	SkipQuestionnaireOnDefError bool
	// OmitSharedTail skips AppendDecisionKindIssues + ValidateQuestionnaireMeasure.
	// Typology owns questionnaire checks inside AfterDefinition / runtime validator.
	OmitSharedTail bool
	// AfterDefinition runs after Definition/Norm validation and optional early
	// return. Typology uses it for runtime-spec checks.
	AfterDefinition func(ctx context.Context, model *domain.AssessmentModel, issues []domain.DomainValidationIssue) []domain.DomainValidationIssue
}

// ComposePublishValidation runs the shared publication validation pipeline.
func ComposePublishValidation(
	ctx context.Context,
	model *domain.AssessmentModel,
	opts PublicationComposerOptions,
) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{modelRequiredIssue()}
	}
	if opts.RequireLegacyDefinition && model.Definition.IsEmpty() {
		message := opts.LegacyDefinitionMessage
		if message == "" {
			message = "模型定义不能为空"
		}
		return []domain.DomainValidationIssue{{
			Field: "definition", Message: message,
			Code: "definition.required", Level: domain.ValidationLevelError,
		}}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, ValidateDefinitionForPublish(ctx, model, opts.NormRepo)...)
	if opts.IncludeBehavioralSemantic {
		issues = append(issues, ValidateBehavioralSemantic(model)...)
	}
	if opts.IncludeAlgorithmBinding {
		issues = append(issues, ValidateAlgorithmBinding(model)...)
	}
	if opts.SkipQuestionnaireOnDefError && domain.HasValidationErrors(issues) {
		return issues
	}
	if opts.AfterDefinition != nil {
		issues = opts.AfterDefinition(ctx, model, issues)
	}
	if opts.OmitSharedTail {
		return issues
	}
	issues = AppendDecisionKindIssues(model, issues)
	issues = append(issues, ValidateQuestionnaireMeasure(ctx, opts.QuestionnaireQuery, model)...)
	return issues
}
