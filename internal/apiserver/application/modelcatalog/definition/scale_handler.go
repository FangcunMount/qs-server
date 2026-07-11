package definition

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// ScaleDefinitionHandler owns the scale-specific wire projection and publish
// validation. DefinitionV2 is its only authoring input.
type ScaleDefinitionHandler struct{}

func (ScaleDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindScale
}

func (ScaleDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{{
			Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError,
		}}
	}
	issues := model.ValidateForPublish().Issues
	return append(issues, ValidateDefinitionV2ForPublish(ctx, model.DefinitionV2, nil)...)
}

func (ScaleDefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil {
		return SnapshotBuildResult{}, fmt.Errorf("scale assessment model is nil")
	}
	if model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("scale definition_v2 is required")
	}
	snapshot := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{
		Code:                 model.Code,
		ScaleVersion:         "v" + fmt.Sprint(model.Revision()),
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               "published",
	}, model.DefinitionV2)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("marshal scale snapshot: %w", err)
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmScaleDefault
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{
		Kind:          domain.KindScale,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatAssessmentScaleV1,
		DecisionKind:  decisionKind,
		Payload:       encoded,
		Version:       snapshot.ScaleVersion,
	}, nil
}
