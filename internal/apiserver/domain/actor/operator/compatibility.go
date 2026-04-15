package operator

// Staff 是 Operator 的兼容别名，保留给边界层旧调用方。
type Staff = Operator

// NewStaff 兼容旧构造函数，内部委托到 NewOperator。
func NewStaff(orgID int64, userID int64, name string) *Operator {
	return NewOperator(orgID, userID, name)
}

// RoleStaff 是 RoleOperator 的兼容别名，保留旧命名给边界层。
const RoleStaff Role = RoleOperator
