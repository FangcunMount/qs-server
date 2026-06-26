package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"

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
	Algorithm assessmentmodel.Algorithm
}

// DefaultModelDescriptors returns the built-in evaluation model registry.
func DefaultModelDescriptors() []ModelDescriptor {
	return []ModelDescriptor{
		{Key: EvaluatorKeyScaleDefault, Kind: ModelKindScale},
		{
			Key:       PersonalityTypologyKey(assessmentmodel.AlgorithmMBTI),
			Kind:      ModelKindTypology,
			Algorithm: assessmentmodel.AlgorithmMBTI,
		},
		{
			Key:       PersonalityTypologyKey(assessmentmodel.AlgorithmSBTI),
			Kind:      ModelKindTypology,
			Algorithm: assessmentmodel.AlgorithmSBTI,
		},
	}
}

// TypologyAlgorithms returns typology algorithms from descriptors.
func TypologyAlgorithms(descs []ModelDescriptor) []assessmentmodel.Algorithm {
	out := make([]assessmentmodel.Algorithm, 0, len(descs))
	for _, desc := range descs {
		if desc.Kind != ModelKindTypology || desc.Algorithm == "" {
			continue
		}
		out = append(out, desc.Algorithm)
	}
	return out
}
