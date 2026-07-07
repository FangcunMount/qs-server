package modelcatalog

// CapabilityRole separates product-facing taxonomy from executable model families.
// Product channels must not drive runtime execution path for new models.
type CapabilityRole string

const (
	// CapabilityRoleProductChannel marks API kinds used for product aggregation only.
	// They must not receive new draft models; use a ModelFamily kind instead.
	CapabilityRoleProductChannel CapabilityRole = "product_channel"
	// CapabilityRoleModelFamily marks executable assessment model families.
	CapabilityRoleModelFamily CapabilityRole = "model_family"
)

func (r CapabilityRole) String() string { return string(r) }

// IsProductChannel reports whether the capability is a product aggregation slot.
func (c KindCapability) IsProductChannel() bool {
	return c.Role == CapabilityRoleProductChannel
}

// AllowsNewDraft reports whether catalog APIs may create new draft models for this family.
func (c KindCapability) AllowsNewDraft() bool {
	return c.CreateSupported && !c.IsProductChannel()
}
