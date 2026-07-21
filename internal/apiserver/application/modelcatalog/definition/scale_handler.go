package definition

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ScaleDefinitionHandler composes shared validators with scale payload projection.
type ScaleDefinitionHandler struct {
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

// Supports 支持特定评估模型身份
func (ScaleDefinitionHandler) Supports(identity domain.Identity) bool {
	return supportsBinding(domain.KindScale, identity)
}

// ValidateForPublish 验证发布
func (h ScaleDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	return ComposePublishValidation(ctx, model, PublicationComposerOptions{
		QuestionnaireQuery:     h.QuestionnaireQuery,
		StrategyCapabilityPath: capability.PathScaleDescriptor,
	})
}

// MaterializeSnapshot validates the DefinitionV2 scale runtime projection.
func (ScaleDefinitionHandler) MaterializeSnapshot(_ context.Context, model *domain.AssessmentModel) (Materialization, error) {
	return (RuntimeMaterializer{}).MaterializeScale(model)
}
