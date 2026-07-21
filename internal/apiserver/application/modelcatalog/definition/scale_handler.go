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
	issues := ComposePublishValidation(ctx, model, PublicationComposerOptions{
		QuestionnaireQuery:     h.QuestionnaireQuery,
		StrategyCapabilityPath: capability.PathScaleDescriptor,
	})
	if model == nil || model.DefinitionV2 == nil {
		return issues
	}
	for _, rule := range model.DefinitionV2.Measure.Scoring {
		if rule.Constant == 0 {
			continue
		}
		issues = append(issues, domain.DomainValidationIssue{
			Field:   "measure.scoring[" + rule.FactorCode + "].constant",
			Code:    "factor.scoring.constant.unsupported",
			Message: "scale factor_scoring 不支持 scoring.constant；请移除该字段或使用具备 constant 语义的模型类型",
			Level:   domain.ValidationLevelError,
		})
	}
	return issues
}

// MaterializeSnapshot validates the DefinitionV2 scale runtime projection.
func (ScaleDefinitionHandler) MaterializeSnapshot(_ context.Context, model *domain.AssessmentModel) (Materialization, error) {
	return (RuntimeMaterializer{}).MaterializeScale(model)
}
