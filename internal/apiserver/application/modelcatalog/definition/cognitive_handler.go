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
		IncludeAlgorithmBinding: true,
		StrategyCapabilityPath:  capability.PathCognitiveDescriptor,
	})
}

// MaterializeSnapshot validates the DefinitionV2 cognitive runtime projection.
func (CognitiveDefinitionHandler) MaterializeSnapshot(_ context.Context, model *domain.AssessmentModel) (Materialization, error) {
	return (RuntimeMaterializer{}).MaterializeCognitive(model)
}
