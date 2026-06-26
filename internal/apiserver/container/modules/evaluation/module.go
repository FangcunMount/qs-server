package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageEvaluation

// Descriptor identifies the evaluation module in container composition.
type Descriptor struct {
	Name modules.PackageName
}

// Describe returns the evaluation module descriptor.
func Describe() Descriptor {
	return Descriptor{Name: Name}
}
