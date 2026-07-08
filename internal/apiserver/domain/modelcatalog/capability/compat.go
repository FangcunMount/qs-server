package capability

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

type (
	ModelFamilyCapability = binding.ModelFamilyCapability
	CapabilityRole        = binding.CapabilityRole
)

const (
	CapabilityRoleProductChannel = binding.CapabilityRoleProductChannel
	CapabilityRoleModelFamily    = binding.CapabilityRoleModelFamily
)

var (
	FamilyCapabilityByKind = binding.FamilyCapabilityByKind
	RuntimeExecutableKinds = binding.RuntimeExecutableKinds
)
