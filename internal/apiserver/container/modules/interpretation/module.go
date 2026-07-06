package interpretation

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageInterpretation

// Report assembly for read/write model, builder registry, and durable saver lives in assemble.go.
type Descriptor struct {
	Name modules.PackageName
}

// Describe returns the report module descriptor.
func Describe() Descriptor {
	return Descriptor{Name: Name}
}
