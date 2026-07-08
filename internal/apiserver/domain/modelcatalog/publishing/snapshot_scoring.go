package publishing

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

func buildScoringPublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
	if model.Kind != binding.KindScale {
		return nil, fmt.Errorf("model kind %s is not scale", model.Kind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("scale model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("scale model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = binding.AlgorithmScaleDefault
	}
	return scoringPublishedEnvelope(model, encoded, algorithm), nil
}

// BuildScoringPublishedSnapshotFromScale materializes a v2 published snapshot from a scale ruleset snapshot.
func BuildScoringPublishedSnapshotFromScale(model *scalesnapshot.ScaleSnapshot) (*PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("scale snapshot is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("marshal scale payload: %w", err)
	}
	status := model.Status
	if status == "" {
		status = string(ModelStatusPublished)
	}
	version := model.ScaleVersion
	if version == "" {
		version = model.QuestionnaireVersion
	}
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		PayloadFormat: PayloadFormatAssessmentScaleV1,
		Model: ModelDefinition{
			ProductChannel: binding.ProductChannelMedicalScale,
			Kind:           binding.KindScale,
			SubKind:        binding.SubKindEmpty,
			Algorithm:      binding.AlgorithmScaleDefault,
			Code:           model.Code,
			Version:        version,
			Title:          model.Title,
			Status:         status,
		},
		Binding: binding.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		Decision: DecisionSpec{Kind: binding.DecisionKindScoreRange},
		Source:   SourceRef{},
		Payload:  payload,
	}, nil
}

func scoringPublishedEnvelope(model *AssessmentModel, encoded []byte, algorithm binding.Algorithm) *PublishedModelSnapshot {
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		PayloadFormat: PayloadFormatAssessmentScaleV1,
		Model: ModelDefinition{
			ProductChannel: binding.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           binding.KindScale,
			SubKind:        binding.SubKindEmpty,
			Algorithm:      algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: DecisionSpec{Kind: binding.DecisionKindScoreRange},
		Source:   SourceRef{},
		Payload:  encoded,
	}
}
