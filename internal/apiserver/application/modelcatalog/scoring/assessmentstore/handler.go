package assessmentstore

import (
	"context"
	"encoding/json"
	"fmt"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// DefinitionHandler owns medical-scale definition validation and publish shaping.
type DefinitionHandler struct{}

func (DefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindScale
}

func (DefinitionHandler) PrepareForSave(_ context.Context, _ *domain.AssessmentModel, input appdefinition.SaveInput) (appdefinition.SaveResult, []domain.DomainValidationIssue, error) {
	format := input.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatAssessmentScaleV1
	}
	if format != domain.PayloadFormatAssessmentScaleV1 {
		return appdefinition.SaveResult{}, []domain.DomainValidationIssue{{
			Field: "definition.format", Message: "unsupported scale payload format", Code: "definition.format.unsupported", Level: domain.ValidationLevelError,
		}}, nil
	}
	if len(input.Payload) == 0 {
		return appdefinition.SaveResult{}, []domain.DomainValidationIssue{{
			Field: "definition", Message: "量表定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError,
		}}, nil
	}
	if !json.Valid(input.Payload) {
		return appdefinition.SaveResult{}, []domain.DomainValidationIssue{{
			Field: "definition.payload", Message: "量表定义不是有效 JSON", Code: "definition.payload.invalid", Level: domain.ValidationLevelError,
		}}, nil
	}
	snapshot, err := scalesnapshot.ParsePublishedPayload(input.Payload)
	if err != nil {
		return appdefinition.SaveResult{}, nil, err
	}
	return appdefinition.SaveResult{
		Payload: domain.DefinitionPayload{
			Format: format,
			Data:   append([]byte(nil), input.Payload...),
		},
		DefinitionV2: scalesnapshot.DefinitionFromScaleSnapshot(snapshot),
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
			Field: "definition", Message: "量表定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError,
		}}
	}
	return model.ValidateForPublish().Issues
}

func (DefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (appdefinition.SnapshotBuildResult, error) {
	if model.Definition.IsEmpty() {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("scale model definition is empty")
	}
	snapshot, err := scalesnapshot.ParsePublishedPayload(model.Definition.Data)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	snapshot.Status = "published"
	snapshot.Title = model.Title
	snapshot.Code = model.Code
	snapshot.QuestionnaireCode = model.Binding.QuestionnaireCode
	snapshot.QuestionnaireVersion = model.Binding.QuestionnaireVersion
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("marshal scale snapshot: %w", err)
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmScaleDefault
	}
	return appdefinition.SnapshotBuildResult{
		Kind:          domain.KindScale,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatAssessmentScaleV1,
		DecisionKind:  domain.DecisionKindScoreRange,
		Payload:       encoded,
		Version:       snapshot.ScaleVersion,
	}, nil
}
