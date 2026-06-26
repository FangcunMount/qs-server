package statistics

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageStatistics

// Descriptor identifies the statistics module in container composition.
type Descriptor struct {
	Name modules.PackageName
}

// Describe returns the statistics module descriptor.
func Describe() Descriptor {
	return Descriptor{Name: Name}
}
