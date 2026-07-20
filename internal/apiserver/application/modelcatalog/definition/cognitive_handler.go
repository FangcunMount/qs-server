package definition

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// CognitiveDefinitionHandler composes shared validators with cognitive payload projection.
type CognitiveDefinitionHandler struct {
	NormRepo           port.NormRepository
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

// Supports 支持特定评估模型身份
func (CognitiveDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindCognitive
}

// ValidateForPublish 验证发布
func (h CognitiveDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{modelRequiredIssue()}
	}
	if model.Definition.IsEmpty() {
		return []domain.DomainValidationIssue{{
			Field: "definition", Message: "认知模型定义不能为空",
			Code: "definition.required", Level: domain.ValidationLevelError,
		}}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, ValidateDefinitionForPublish(ctx, model, h.NormRepo)...)
	issues = append(issues, ValidateAlgorithmBinding(model)...)
	issues = AppendDecisionKindIssues(model, issues)
	issues = append(issues, ValidateQuestionnaireMeasure(ctx, h.QuestionnaireQuery, model)...)
	return issues
}

// BuildSnapshotPayload 构建评估模型快照负载
func (CognitiveDefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	return (CompatibilityPayloadProjector{}).ProjectCognitive(model)
}
