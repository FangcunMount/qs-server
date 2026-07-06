package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionPathForDescriptor maps a runtime descriptor to its materialization path.
func ExecutionPathForDescriptor(desc ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	switch desc.Kind {
	case ModelKindScale:
		return modelcatalog.ExecutionPathScaleDescriptor, nil
	case ModelKindTypology:
		return modelcatalog.ExecutionPathTypologyDescriptor, nil
	case ModelKindBehavioralRating:
		return modelcatalog.ExecutionPathBehavioralRatingDescriptor, nil
	case ModelKindCognitive:
		return modelcatalog.ExecutionPathCognitiveDescriptor, nil
	default:
		return "", fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
	}
}
