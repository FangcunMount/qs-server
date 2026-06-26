package survey

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageSurvey

// Descriptor identifies the survey module in container composition.
type Descriptor struct {
	Name modules.PackageName
}

// Describe returns the survey module descriptor.
func Describe() Descriptor {
	return Descriptor{Name: Name}
}
