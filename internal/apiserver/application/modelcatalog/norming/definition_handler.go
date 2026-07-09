package norming

import (
	"context"
	"encoding/json"
	"fmt"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	domainnorming "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// DefinitionHandler owns behavioral-rating definition validation and publish shaping.
type DefinitionHandler struct{}

func (DefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindBehavioralRating
}

func (DefinitionHandler) PrepareForSave(_ context.Context, _ *domain.AssessmentModel, input appdefinition.SaveInput) (appdefinition.SaveResult, []domain.DomainValidationIssue, error) {
	return appdefinition.SaveResult{
		Payload: domain.DefinitionPayload{Data: append([]byte(nil), input.Payload...)},
	}, nil, nil
}

func (DefinitionHandler) ValidateForPublish(_ context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
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
	return model.ValidateForPublish().Issues
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
	var err error
	encoded, err = domainnorming.RequirePrimaryDimensionCodeForPublish(encoded)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	result := appdefinition.SnapshotBuildResult{
		Kind:          domain.KindBehavioralRating,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatForBehavioralRating(algorithm),
		DecisionKind:  domainnorming.DecisionKindFromDefinitionPayload(encoded),
		Payload:       encoded,
	}
	if err := validatePublishedScoreNodes(&port.PublishedModel{
		PayloadFormat: result.PayloadFormat,
		Code:          model.Code,
		Version:       fmt.Sprintf("v%d", model.Revision()),
		Title:         model.Title,
		Status:        string(domain.ModelStatusPublished),
		Payload:       result.Payload,
	}); err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	return result, nil
}
