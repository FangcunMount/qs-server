package modelcatalog

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageModelCatalog

// RegisterNames lists registerModule keys: aggregate plus legacy capability aliases.
var RegisterNames = []string{string(Name), "scale", "personalitymodel"}

// LegacyRegisterNames are legacy registerModule keys kept for GetLoadedModules compatibility
// after scale and personality catalog merged under modelcatalog.
var LegacyRegisterNames = []string{"scale", "personalitymodel"}

// Descriptor identifies the model-catalog module in container composition.
type Descriptor struct {
	Name                modules.PackageName
	RegisterNames       []string
	LegacyRegisterNames []string
}

// Describe returns the model-catalog module descriptor.
func Describe() Descriptor {
	return Descriptor{
		Name:                Name,
		RegisterNames:       append([]string(nil), RegisterNames...),
		LegacyRegisterNames: append([]string(nil), LegacyRegisterNames...),
	}
}
