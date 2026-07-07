package factor

// FactorRole classifies the business semantics of a model dimension.
type FactorRole string

const (
	FactorRoleDimension     FactorRole = "dimension" // 叶子计分因子（文档亦称 factor）
	FactorRoleTotal         FactorRole = "total"
	FactorRoleIndex         FactorRole = "index" // 综合指数（文档亦称 composite_index）
	FactorRoleValidity      FactorRole = "validity"
	FactorRoleSubtest       FactorRole = "subtest"
	FactorRoleTaskSet       FactorRole = "task_set"
	FactorRoleReportGroup   FactorRole = "report_group"   // 报告分组，不参与计分
	FactorRoleAbilityDomain FactorRole = "ability_domain" // 能力域，task_performance 可选
)

func (r FactorRole) String() string { return string(r) }

func (r FactorRole) IsValid() bool {
	switch r {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleIndex,
		FactorRoleValidity, FactorRoleSubtest, FactorRoleTaskSet,
		FactorRoleReportGroup, FactorRoleAbilityDomain:
		return true
	default:
		return false
	}
}
