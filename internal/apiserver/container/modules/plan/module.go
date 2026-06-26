package plan

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackagePlan

// Descriptor identifies the plan module in container composition.
type Descriptor struct {
	Name modules.PackageName
}

// Describe returns the plan module descriptor.
func Describe() Descriptor {
	return Descriptor{Name: Name}
}
