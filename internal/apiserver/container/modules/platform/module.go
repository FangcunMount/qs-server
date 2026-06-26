package platform

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackagePlatform

// Capability names integration surfaces composed at container root today.
var Capabilities = []string{
	"iam",
	"eventing",
	"cachegovernance",
	"qrcode",
	"notification",
	"codes",
}

// Descriptor identifies the platform/integration layer in container composition.
type Descriptor struct {
	Name         modules.PackageName
	Capabilities []string
}

// Describe returns the platform module descriptor.
func Describe() Descriptor {
	return Descriptor{
		Name:         Name,
		Capabilities: append([]string(nil), Capabilities...),
	}
}
