package factor

// FactorRole classifies the business semantics of a model dimension.
type FactorRole string

const (
	FactorRoleDimension FactorRole = "dimension"
	FactorRoleTotal     FactorRole = "total"
	FactorRoleIndex     FactorRole = "index"
	FactorRoleValidity  FactorRole = "validity"
	FactorRoleSubtest   FactorRole = "subtest"
	FactorRoleTaskSet   FactorRole = "task_set"
)

func (r FactorRole) String() string { return string(r) }

func (r FactorRole) IsValid() bool {
	switch r {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleIndex,
		FactorRoleValidity, FactorRoleSubtest, FactorRoleTaskSet:
		return true
	default:
		return false
	}
}
