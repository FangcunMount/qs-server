package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
)

// Module aggregates one personality typology algorithm's adapter and report path.
type Module struct {
	Algorithm     assessmentmodel.Algorithm
	CategoryLabel string
	Adapter       personalityadapter.ModelAdapter
	reportBuilder reportBuilderFunc
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
		if module.Algorithm == "" || module.Adapter == nil {
			continue
		}
		out = append(out, module.Descriptor())
	}
	return out
}
