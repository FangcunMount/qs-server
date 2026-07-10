package definition

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// ScaleDefinitionHandler owns the scale-specific wire projection. Its domain
// input and publish validation are DefinitionV2-first; JSON parsing remains
// only for importing a legacy definition envelope.
type ScaleDefinitionHandler struct{}

func (ScaleDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindScale
}

func (h ScaleDefinitionHandler) PrepareForSave(ctx context.Context, model *domain.AssessmentModel, input SaveInput) (SaveResult, []domain.DomainValidationIssue, error) {
	if input.DefinitionV2 != nil {
		if issues := ValidateDefinitionV2(input.DefinitionV2); len(issues) > 0 {
			return SaveResult{}, issues, nil
		}
		if model == nil {
			return SaveResult{}, []domain.DomainValidationIssue{{
				Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError,
			}}, nil
		}
		candidate := *model
		candidate.DefinitionV2 = input.DefinitionV2
		built, err := h.BuildSnapshotPayload(ctx, &candidate)
		if err != nil {
			return SaveResult{}, nil, err
		}
		return SaveResult{
			Payload:      domain.DefinitionPayload{Format: built.PayloadFormat, Data: append([]byte(nil), built.Payload...)},
			DefinitionV2: input.DefinitionV2,
			Algorithm:    built.Algorithm,
			SubKind:      built.SubKind,
		}, nil, nil
	}

	format := input.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatAssessmentScaleV1
	}
	if format != domain.PayloadFormatAssessmentScaleV1 {
		return SaveResult{}, []domain.DomainValidationIssue{{
			Field: "definition.format", Message: "unsupported scale payload format", Code: "definition.format.unsupported", Level: domain.ValidationLevelError,
		}}, nil
	}
	if len(input.Payload) == 0 {
		return SaveResult{}, []domain.DomainValidationIssue{{
			Field: "definition", Message: "量表定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError,
		}}, nil
	}
	if !json.Valid(input.Payload) {
		return SaveResult{}, []domain.DomainValidationIssue{{
			Field: "definition.payload", Message: "量表定义不是有效 JSON", Code: "definition.payload.invalid", Level: domain.ValidationLevelError,
		}}, nil
	}
	snapshot, err := scalepayload.ParsePublishedPayload(input.Payload)
	if err != nil {
		return SaveResult{}, nil, err
	}
	return SaveResult{
		Payload:      domain.DefinitionPayload{Format: format, Data: append([]byte(nil), input.Payload...)},
		DefinitionV2: scalepayload.DefinitionFromScaleSnapshot(snapshot),
	}, nil, nil
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
