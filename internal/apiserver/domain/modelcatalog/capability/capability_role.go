package capability

// CapabilityRole separates product-facing 分类体系 从 可执行 模型家族。
// Product channels 不得 drive 运行时 执行路径 用于 new models。
type CapabilityRole string

const (
	// CapabilityRoleProductChannel 标记API 类型 用于 product aggregation 仅。
	// They 不得 receive 新的草稿模型; 使用 模型家族 类型 instead。
	CapabilityRoleProductChannel CapabilityRole = "product_channel"
	// CapabilityRoleModelFamily 标记可执行 assessment 模型家族。
	CapabilityRoleModelFamily CapabilityRole = "model_family"
)

func (r CapabilityRole) String() string { return string(r) }

// IsProductChannel 报告是否 能力 是 产品聚合槽位。
func (c KindCapability) IsProductChannel() bool {
	return c.Role == CapabilityRoleProductChannel
}

// AllowsNewDraft 报告是否 目录 APIs may create 新的草稿模型 用于 这个家族。
func (c KindCapability) AllowsNewDraft() bool {
	return c.CreateSupported && !c.IsProductChannel()
}
