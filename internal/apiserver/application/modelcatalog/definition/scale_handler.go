package definition

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ScaleDefinitionHandler composes shared validators with scale payload projection.
type ScaleDefinitionHandler struct {
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

// Supports 支持特定评估模型身份
func (ScaleDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindScale
}

// ValidateForPublish 验证发布
func (h ScaleDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{modelRequiredIssue()}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, ValidateDefinitionForPublish(ctx, model, nil)...)
	issues = AppendDecisionKindIssues(model, issues)
	issues = append(issues, ValidateQuestionnaireMeasure(ctx, h.QuestionnaireQuery, model)...)
	return issues
}

// BuildSnapshotPayload 构建评估模型快照负载
func (ScaleDefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	return (CompatibilityPayloadProjector{}).ProjectScale(model)
}
