package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// ModelKind distinguishes scale vs personality typology descriptors.
type ModelKind string

const (
	ModelKindScale    ModelKind = "scale"
	ModelKindTypology ModelKind = "typology"
)

// ModelDescriptor is the canonical registration entry for an evaluation model.
type ModelDescriptor struct {
	Key       EvaluatorKey
	Kind      ModelKind
	Algorithm modelcatalog.Algorithm
}

// ScaleModelDescriptor returns the built-in scale evaluation descriptor.
func ScaleModelDescriptor() ModelDescriptor {
	return ModelDescriptor{Key: EvaluatorKeyScaleDefault, Kind: ModelKindScale}
}

// DefaultModelDescriptors returns built-in scale descriptors only.
// Typology descriptors are owned by application typology.DefaultModules() at composition root.
func DefaultModelDescriptors() []ModelDescriptor {
	return []ModelDescriptor{ScaleModelDescriptor()}
}

// TypologyAlgorithms returns typology algorithms from descriptors.
func TypologyAlgorithms(descs []ModelDescriptor) []modelcatalog.Algorithm {
	out := make([]modelcatalog.Algorithm, 0, len(descs))
	for _, desc := range descs {
		if desc.Kind != ModelKindTypology || desc.Algorithm == "" {
			continue
		}
		out = append(out, desc.Algorithm)
	}
	return out
}
