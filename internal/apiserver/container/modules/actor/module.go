package actor

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageActor

// Descriptor identifies the actor module in container composition.
type Descriptor struct {
	Name modules.PackageName
}

// Describe returns the actor module descriptor.
func Describe() Descriptor {
	return Descriptor{Name: Name}
}
