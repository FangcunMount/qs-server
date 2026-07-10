package norming

import (
	"context"
	"encoding/json"
	"fmt"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// DefinitionHandler owns behavioral-rating definition validation and publish shaping.
type DefinitionHandler struct {
	NormRepo port.NormRepository
}

func (DefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindBehavioralRating
}

func (DefinitionHandler) PrepareForSave(_ context.Context, _ *domain.AssessmentModel, input appdefinition.SaveInput) (appdefinition.SaveResult, []domain.DomainValidationIssue, error) {
	result := appdefinition.SaveResult{
		Payload: domain.DefinitionPayload{Data: append([]byte(nil), input.Payload...)},
	}
	if materialized, err := behavioralpayload.MaterializeDefinition(input.Payload); err == nil {
		result.DefinitionV2 = materialized.Definition
		result.Norms = materialized.Norms
	}
	return result, nil, nil
}

func (h DefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{{
			Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError,
		}}
	}
	if model.Definition.IsEmpty() {
		return []domain.DomainValidationIssue{{
			Field: "definition", Message: "行为评定模型定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError,
		}}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, appdefinition.ValidateDefinitionV2ForPublish(ctx, model.DefinitionV2, h.NormRepo)...)
	return append(issues, appdefinition.ValidateSharedFactorPayloadForPublish(model.Definition.Data)...)
}

func (DefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (appdefinition.SnapshotBuildResult, error) {
	if model.Definition.IsEmpty() {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("behavioral_rating model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("behavioral_rating model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBehavioralRatingDefault
	}
	return appdefinition.SnapshotBuildResult{
		Kind:          domain.KindBehavioralRating,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatForBehavioralRating(algorithm),
		DecisionKind:  domain.DecisionKindNormLookup,
		Payload:       encoded,
	}, nil
}
