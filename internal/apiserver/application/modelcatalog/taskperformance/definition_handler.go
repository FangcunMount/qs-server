package taskperformance

import (
	"context"
	"encoding/json"
	"fmt"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

// DefinitionHandler owns cognitive task-performance definition validation and publish shaping.
type DefinitionHandler struct {
	NormRepo port.NormRepository
}

func (DefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindCognitive
}

func (DefinitionHandler) PrepareForSave(_ context.Context, _ *domain.AssessmentModel, input appdefinition.SaveInput) (appdefinition.SaveResult, []domain.DomainValidationIssue, error) {
	materialized, err := cognitivepayload.MaterializeDefinition(input.Payload)
	if err != nil {
		return appdefinition.SaveResult{}, nil, err
	}
	if issues := appdefinition.ValidateDefinitionV2(materialized.Definition); len(issues) > 0 {
		return appdefinition.SaveResult{}, issues, nil
	}
	return appdefinition.SaveResult{
		Payload:      domain.DefinitionPayload{Data: append([]byte(nil), input.Payload...)},
		DefinitionV2: materialized.Definition,
		Norms:        materialized.Norms,
	}, nil, nil
}

func (h DefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
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
	issues := model.ValidateForPublish().Issues
	issues = append(issues, appdefinition.ValidateDefinitionV2ForPublish(ctx, model.DefinitionV2, h.NormRepo)...)
	if _, err := model.DecisionKindForDefinition(); err != nil {
		issues = append(issues, domain.DomainValidationIssue{Field: "definition_v2.conclusions", Code: "definition_v2.decision.invalid", Message: err.Error(), Level: domain.ValidationLevelError})
	}
	return issues
}

func (DefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (appdefinition.SnapshotBuildResult, error) {
	if model.Definition.IsEmpty() {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("cognitive model definition is empty")
	}
	if model.DefinitionV2 == nil {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("cognitive definition_v2 is required")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("cognitive model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	return appdefinition.SnapshotBuildResult{
		Kind:          domain.KindCognitive,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatForCognitive(algorithm),
		DecisionKind:  decisionKind,
		Payload:       encoded,
	}, nil
}
