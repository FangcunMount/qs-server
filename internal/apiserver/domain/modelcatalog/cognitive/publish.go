package cognitive

import (
	"encoding/json"
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// BuildPublishedSnapshot materializes a v2 published snapshot from a draft cognitive model.
func BuildPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	if model.Kind != domain.KindCognitive {
		return nil, fmt.Errorf("model kind %s is not cognitive", model.Kind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("cognitive model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("cognitive model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatForCognitive(algorithm),
		Model: domain.ModelDefinition{
			ProductChannel: domain.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           domain.KindCognitive,
			SubKind:        domain.SubKindEmpty,
			Algorithm:      algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(domain.ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindScoreRange},
		Source:   domain.SourceRef{},
		Payload:  encoded,
	}, nil
}

func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Version, 10)
}
