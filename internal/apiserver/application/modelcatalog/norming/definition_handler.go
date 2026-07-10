package norming

import (
	"context"
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
	if issues := appdefinition.ValidateDefinitionV2(input.DefinitionV2); len(issues) > 0 {
		return appdefinition.SaveResult{}, issues, nil
	}
	return appdefinition.SaveResult{
		Payload:      domain.DefinitionPayload{Data: append([]byte(nil), input.Payload...)},
		DefinitionV2: input.DefinitionV2,
		Norms:        append([]*domain.Norm(nil), input.Norms...),
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
			Field: "definition", Message: "行为评定模型定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError,
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
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("behavioral_rating model definition is empty")
	}
	if model.DefinitionV2 == nil {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	encoded, err := behavioralpayload.PayloadFromDefinition(model.DefinitionV2)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("project behavioral_rating payload: %w", err)
	}
	encoded, err = behavioralpayload.PreserveLegacyNormTables(encoded, model.Definition.Data)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBehavioralRatingDefault
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	return appdefinition.SnapshotBuildResult{
		Kind:          domain.KindBehavioralRating,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatForBehavioralRating(algorithm),
		DecisionKind:  decisionKind,
		Payload:       encoded,
	}, nil
}
