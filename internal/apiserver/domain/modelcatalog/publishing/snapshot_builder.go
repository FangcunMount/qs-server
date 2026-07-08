package publishing

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// BuildPublishedSnapshot materializes a v2 published snapshot from a draft assessment model.
func BuildPublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	switch model.Kind {
	case binding.KindPersonality:
		return buildTypologyPublishedSnapshot(model)
	case binding.KindBehavioralRating:
		return buildNormingPublishedSnapshot(model)
	case binding.KindCognitive:
		return buildTaskPerformancePublishedSnapshot(model)
	default:
		return nil, fmt.Errorf("unsupported model kind %s for publishing snapshot builder", model.Kind)
	}
}

func modelVersionString(model *AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Version, 10)
}
