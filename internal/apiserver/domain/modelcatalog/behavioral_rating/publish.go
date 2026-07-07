package behavioral_rating

import (
	"encoding/json"
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// BuildPublishedSnapshot materializes a v2 published snapshot from a draft behavioral_rating model.
func BuildPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	if model.Kind != domain.KindBehavioralRating {
		return nil, fmt.Errorf("model kind %s is not behavioral_rating", model.Kind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("behavioral_rating model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("behavioral_rating model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBrief2
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatForBehavioralRating(algorithm),
		Model: domain.ModelDefinition{
			ProductChannel: domain.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           domain.KindBehavioralRating,
			SubKind:        domain.SubKindEmpty,
			Algorithm:      algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(domain.ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: brief2DecisionSpec(algorithm),
		Source:   domain.SourceRef{},
		Payload:  encoded,
	}, nil
}

func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Version, 10)
}

func brief2DecisionSpec(algorithm domain.Algorithm) domain.DecisionSpec {
	if algorithm == domain.AlgorithmBrief2 {
		return domain.DecisionSpec{Kind: domain.DecisionKindNormLookup}
	}
	return domain.DecisionSpec{Kind: domain.DecisionKindScoreRange}
}
