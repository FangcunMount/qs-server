package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// Module describes a typology algorithm alias exposed to evaluation wiring.
type Module struct {
	Algorithm     assessmentmodel.Algorithm
	CategoryLabel string
}

// Descriptor returns the evaluation registry entry for this module.
func (m Module) Descriptor() evaldomain.ModelDescriptor {
	return evaldomain.ModelDescriptor{
		Key:       evaldomain.PersonalityTypologyKey(m.Algorithm),
		Kind:      evaldomain.ModelKindTypology,
		Algorithm: m.Algorithm,
	}
}

// ModuleDescriptors projects registered typology modules into evaluation descriptors.
func ModuleDescriptors(modules []Module) []evaldomain.ModelDescriptor {
	out := make([]evaldomain.ModelDescriptor, 0, len(modules))
	for _, module := range modules {
		if module.Algorithm == "" {
			continue
		}
		out = append(out, module.Descriptor())
	}
	return out
}

// ConfiguredTypologyDescriptor returns the generic configured typology routing descriptor.
func ConfiguredTypologyDescriptor() evaldomain.ModelDescriptor {
	return evaldomain.ModelDescriptor{
		Key:       evaldomain.EvaluatorKeyPersonalityTypology,
		Kind:      evaldomain.ModelKindTypology,
		Algorithm: assessmentmodel.AlgorithmPersonalityTypology,
	}
}

// DefaultTypologyDescriptors returns the single configured typology routing descriptor.
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return []evaldomain.ModelDescriptor{ConfiguredTypologyDescriptor()}
}
