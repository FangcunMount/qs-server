package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// 设计说明：
// 本文件定义的值对象用于跨聚合根引用，遵循 DDD 最佳实践。
// 当前状态：已设计完成，等待 AnswerSheet/Assessment 等聚合根重构后使用。
// 参考文档：docs/v2/11-01-问卷&量表BC领域模型总览-v2.md

// TesteeRef 受试者引用（值对象）
// 用于在其他聚合根（如 AnswerSheet、Assessment）中引用受试者
// 避免跨聚合根直接依赖实体，保持松耦合
type TesteeRef struct {
	testeeID  testee.ID // 受试者ID
	profileID *uint64   // 可选：用户档案ID（当前对应 IAM.Child.ID）
}

// NewTesteeRef 创建受试者引用
func NewTesteeRef(testeeID testee.ID) *TesteeRef {
	return &TesteeRef{
		testeeID: testeeID,
	}
}

// NewTesteeRefWithProfile 创建带用户档案ID的受试者引用
func NewTesteeRefWithProfile(testeeID testee.ID, profileID uint64) *TesteeRef {
	return &TesteeRef{
		testeeID:  testeeID,
		profileID: &profileID,
	}
}

// TesteeID 获取受试者ID
func (r *TesteeRef) TesteeID() testee.ID {
	return r.testeeID
}

// ProfileID 获取用户档案ID
func (r *TesteeRef) ProfileID() *uint64 {
	return r.profileID
}

// OperatorRef 后台操作者引用（值对象）
// 用于在其他聚合根中引用机构内操作者。
type OperatorRef struct {
	operatorID operator.ID // 操作者ID
	userID     int64       // 用户ID（必须）
	name       string      // 姓名
}

// NewOperatorRef 创建后台操作者引用
func NewOperatorRef(operatorID operator.ID, userID int64, name string) *OperatorRef {
	return &OperatorRef{
		operatorID: operatorID,
		userID:     userID,
		name:       name,
	}
}

// StaffRef 是 OperatorRef 的兼容别名，保留给旧调用方。
type StaffRef = OperatorRef

// NewStaffRef 创建员工引用
func NewStaffRef(operatorID operator.ID, userID int64, name string) *StaffRef {
	return NewOperatorRef(operatorID, userID, name)
}

// OperatorID 获取操作者ID
func (r *OperatorRef) OperatorID() operator.ID {
	return r.operatorID
}

// StaffID 获取员工ID
func (r *OperatorRef) StaffID() operator.ID {
	return r.operatorID
}

// UserID 获取用户ID
func (r *OperatorRef) UserID() int64 {
	return r.userID
}

// Name 获取姓名
func (r *OperatorRef) Name() string {
	return r.name
}

// ClinicianRef 从业者引用（值对象）。
type ClinicianRef struct {
	clinicianID clinician.ID
	name        string
}

// NewClinicianRef 创建从业者引用。
func NewClinicianRef(clinicianID clinician.ID, name string) *ClinicianRef {
	return &ClinicianRef{
		clinicianID: clinicianID,
		name:        name,
	}
}

// ClinicianID 获取从业者ID。
func (r *ClinicianRef) ClinicianID() clinician.ID {
	return r.clinicianID
}

// Name 获取从业者姓名。
func (r *ClinicianRef) Name() string {
	return r.name
}
