package publishing

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
)

func buildCognitivePublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("cognitive model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("cognitive model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = binding.AlgorithmSPM
	}
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		PayloadFormat: PayloadFormatForCognitive(algorithm),
		Model: ModelDefinition{
			ProductChannel: binding.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           binding.KindCognitive,
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
	}, nil
}

func buildBehavioralRatingPublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("behavioral_rating model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("behavioral_rating model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = binding.AlgorithmBehavioralRatingDefault
	}
	var err error
	encoded, err = norming.RequirePrimaryDimensionCodeForPublish(encoded)
	if err != nil {
		return nil, err
	}
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		PayloadFormat: PayloadFormatForBehavioralRating(algorithm),
		Model: ModelDefinition{
			ProductChannel: binding.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           binding.KindBehavioralRating,
			SubKind:        binding.SubKindEmpty,
			Algorithm:      algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: DecisionSpec{Kind: norming.DecisionKindFromDefinitionPayload(encoded)},
		Source:   SourceRef{},
		Payload:  encoded,
	}, nil
}

func modelVersionString(model *AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Version, 10)
}
