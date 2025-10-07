package role

import (
	"time"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
)

// Auditor 审核员/员工（用户体系中的角色）
// 值对象：用于表示负责审核、管理问卷和答卷的员工角色
type Auditor struct {
	UserID     user.UserID // 用户ID
	Name       string      // 姓名
	EmployeeID string      // 员工编号
	Department string      // 所属部门
	Position   string      // 职位
	Status     Status      // 员工状态
	HiredAt    time.Time   // 入职时间
}

// Status 员工状态
type Status uint8

const (
	StatusOnDuty    Status = 1 // 在职
	StatusOnLeave   Status = 2 // 休假
	StatusSuspended Status = 3 // 停职
	StatusResigned  Status = 4 // 离职
)

// NewAuditor 创建审核员
func NewAuditor(userID user.UserID, name string, employeeID string) *Auditor {
	return &Auditor{
		UserID:     userID,
		Name:       name,
		EmployeeID: employeeID,
		Status:     StatusOnDuty,
		HiredAt:    time.Now(),
	}
}

// GetUserID 获取用户ID
func (a *Auditor) GetUserID() user.UserID {
	return a.UserID
}

// GetName 获取姓名
func (a *Auditor) GetName() string {
	return a.Name
}

// GetEmployeeID 获取员工编号
func (a *Auditor) GetEmployeeID() string {
	return a.EmployeeID
}

// GetDepartment 获取部门
func (a *Auditor) GetDepartment() string {
	return a.Department
}

// GetPosition 获取职位
func (a *Auditor) GetPosition() string {
	return a.Position
}

// GetStatus 获取状态
func (a *Auditor) GetStatus() Status {
	return a.Status
}

// GetHiredAt 获取入职时间
func (a *Auditor) GetHiredAt() time.Time {
	return a.HiredAt
}

// WithDepartment 设置部门
func (a *Auditor) WithDepartment(department string) *Auditor {
	a.Department = department
	return a
}

// WithPosition 设置职位
func (a *Auditor) WithPosition(position string) *Auditor {
	a.Position = position
	return a
}

// WithStatus 设置状态
func (a *Auditor) WithStatus(status Status) *Auditor {
	a.Status = status
	return a
}

// WithHiredAt 设置入职时间
func (a *Auditor) WithHiredAt(hiredAt time.Time) *Auditor {
	a.HiredAt = hiredAt
	return a
}

// IsActive 判断是否在职且可工作
func (a *Auditor) IsActive() bool {
	return a.Status == StatusOnDuty
}

// CanAudit 判断是否可以审核
func (a *Auditor) CanAudit() bool {
	return a.Status == StatusOnDuty || a.Status == StatusOnLeave
}

// Value 获取状态值
func (s Status) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s Status) String() string {
	switch s {
	case StatusOnDuty:
		return "on_duty"
	case StatusOnLeave:
		return "on_leave"
	case StatusSuspended:
		return "suspended"
	case StatusResigned:
		return "resigned"
	default:
		return "unknown"
	}
}
