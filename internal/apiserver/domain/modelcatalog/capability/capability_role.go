package capability

// CapabilityRole separates product-facing taxonomy from executable model families.
type CapabilityRole string

const (
	CapabilityRoleProductChannel CapabilityRole = "product_channel"
	CapabilityRoleModelFamily    CapabilityRole = "model_family"
)

func (r CapabilityRole) String() string { return string(r) }
