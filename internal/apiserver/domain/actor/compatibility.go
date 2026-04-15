package actor

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"

// StaffRef 是 OperatorRef 的兼容别名，保留给旧调用方。
type StaffRef = OperatorRef

// NewStaffRef 创建员工引用。
func NewStaffRef(operatorID operator.ID, userID int64, name string) *StaffRef {
	return NewOperatorRef(operatorID, userID, name)
}

// StaffID 获取员工ID（兼容旧命名）。
func (r *OperatorRef) StaffID() operator.ID {
	return r.operatorID
}

// FillerTypeStaff 是 FillerTypeOperator 的兼容别名。
const FillerTypeStaff FillerType = FillerTypeOperator

// IsStaff 是否员工代填（兼容旧命名）。
func (f *FillerRef) IsStaff() bool {
	return f.fillerType == FillerTypeOperator
}
