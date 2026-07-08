package publishing

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func buildTaskPerformancePublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
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
