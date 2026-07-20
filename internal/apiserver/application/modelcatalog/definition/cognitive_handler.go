package definition

import (
	"context"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
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
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("cognitive definition_v2 is required")
	}
	encoded, err := cognitivepayload.PayloadFromDefinition(model.DefinitionV2)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("project cognitive payload: %w", err)
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{Kind: domain.KindCognitive, Algorithm: algorithm, PayloadFormat: domain.PayloadFormatForCognitive(algorithm), DecisionKind: decisionKind, Payload: encoded}, nil
}
