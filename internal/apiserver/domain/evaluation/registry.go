package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// ModelKind distinguishes scale vs personality typology descriptors.
type ModelKind string

const (
	ModelKindScale            ModelKind = "scale"
	ModelKindTypology         ModelKind = "typology"
	ModelKindBehavioralRating ModelKind = "behavioral_rating"
	ModelKindCognitive        ModelKind = "cognitive"
)

// ModelDescriptor is the canonical registration entry for an evaluation model.
type ModelDescriptor struct {
	Kind      ModelKind
	Algorithm modelcatalog.Algorithm
}

// ExecutionIdentity derives the routing identity for a runtime descriptor.
func (d ModelDescriptor) ExecutionIdentity() ExecutionIdentity {
	switch d.Kind {
	case ModelKindScale:
		return ExecutionIdentityScaleDefault
	case ModelKindBehavioralRating:
		return ExecutionIdentityBehavioralRatingDefault
	case ModelKindCognitive:
		return ExecutionIdentityCognitiveDefault
	case ModelKindTypology:
		if d.Algorithm != "" {
			return PersonalityTypologyIdentity(d.Algorithm)
		}
		return ExecutionIdentityPersonalityTypology
	default:
		return ExecutionIdentity{}
	}
}

// CognitiveModelDescriptor returns the built-in cognitive runtime descriptor.
func CognitiveModelDescriptor() ModelDescriptor {
	return ModelDescriptor{
		Kind:      ModelKindCognitive,
		Algorithm: modelcatalog.AlgorithmSPM,
	}
}

// BehavioralRatingModelDescriptor returns the built-in behavioral_rating runtime descriptor.
func BehavioralRatingModelDescriptor() ModelDescriptor {
	return ModelDescriptor{
		Kind:      ModelKindBehavioralRating,
		Algorithm: modelcatalog.AlgorithmBehavioralRatingDefault,
	}
}

// ScaleModelDescriptor returns the built-in scale evaluation descriptor.
func ScaleModelDescriptor() ModelDescriptor {
	return ModelDescriptor{Kind: ModelKindScale}
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
