package taskperformance

import (
	"context"
	"encoding/json"
	"fmt"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltaskperformance "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance"
)

// DefinitionHandler owns cognitive task-performance definition validation and publish shaping.
type DefinitionHandler struct{}

func (DefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindCognitive
}

func (DefinitionHandler) PrepareForSave(_ context.Context, _ *domain.AssessmentModel, input appdefinition.SaveInput) (appdefinition.SaveResult, []domain.DomainValidationIssue, error) {
	result := appdefinition.SaveResult{
		Payload: domain.DefinitionPayload{Data: append([]byte(nil), input.Payload...)},
	}
	if definitionV2, err := modeltaskperformance.DefinitionFromPayload(input.Payload); err == nil {
		result.DefinitionV2 = definitionV2
	}
	return result, nil, nil
}

func (DefinitionHandler) ValidateForPublish(_ context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{{
			Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError,
		}}
	}
	if model.Definition.IsEmpty() {
		return []domain.DomainValidationIssue{{
			Field: "definition", Message: "认知模型定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError,
		}}
	}
	return model.ValidateForPublish().Issues
}

func (DefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (appdefinition.SnapshotBuildResult, error) {
	if model.Definition.IsEmpty() {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("cognitive model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("cognitive model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	return appdefinition.SnapshotBuildResult{
		Kind:          domain.KindCognitive,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatForCognitive(algorithm),
		DecisionKind:  domain.DecisionKindAbilityLevel,
		Payload:       encoded,
	}, nil
}
