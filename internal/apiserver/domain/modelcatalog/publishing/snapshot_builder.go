package publishing

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// BuildPublishedSnapshot materializes a v2 published snapshot from a draft assessment model.
func BuildPublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	switch model.Kind {
	case binding.KindBehavioralRating:
		return buildBehavioralRatingPublishedSnapshot(model)
	case binding.KindCognitive:
		return buildCognitivePublishedSnapshot(model)
	default:
		return nil, fmt.Errorf("unsupported model kind %s for publishing snapshot builder", model.Kind)
	}
}
