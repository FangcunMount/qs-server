package assessmentmodel

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageAssessmentModel

// LegacyRegisterNames are registerModule keys kept for GetLoadedModules compatibility
// after scale and personality catalog merged under assessmentmodel.
var LegacyRegisterNames = []string{"scale", "personalitymodel"}

// Descriptor identifies the assessment-model module in container composition.
type Descriptor struct {
	Name                modules.PackageName
	LegacyRegisterNames []string
}

// Describe returns the assessment-model module descriptor.
func Describe() Descriptor {
	return Descriptor{
		Name:                Name,
		LegacyRegisterNames: append([]string(nil), LegacyRegisterNames...),
	}
}
