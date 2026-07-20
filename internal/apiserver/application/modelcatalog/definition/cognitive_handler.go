package definition

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
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
	return supportsBinding(domain.KindCognitive, identity)
}

// ValidateForPublish 验证发布
func (h CognitiveDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	return ComposePublishValidation(ctx, model, PublicationComposerOptions{
		NormRepo:                h.NormRepo,
		QuestionnaireQuery:      h.QuestionnaireQuery,
		RequireLegacyDefinition: true,
		LegacyDefinitionMessage: "认知模型定义不能为空",
		IncludeAlgorithmBinding: true,
		StrategyCapabilityPath:  capability.PathCognitiveDescriptor,
	})
}

// BuildSnapshotPayload 构建评估模型快照负载
func (CognitiveDefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	return (CompatibilityPayloadProjector{}).ProjectCognitive(model)
}
