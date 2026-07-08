package modelcatalog

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageModelCatalog

// RegisterNames lists registerModule keys for the model-catalog aggregate module.
var RegisterNames = []string{string(Name)}

// Descriptor identifies the model-catalog module in container composition.
type Descriptor struct {
	Name          modules.PackageName
	RegisterNames []string
}

// Describe returns the model-catalog module descriptor.
func Describe() Descriptor {
	return Descriptor{
		Name:          Name,
		RegisterNames: append([]string(nil), RegisterNames...),
	}
}
