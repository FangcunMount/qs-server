package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
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

// StaffRef 员工引用（值对象）
// 用于在其他聚合根中引用员工
type StaffRef struct {
	staffID staff.ID // 员工ID
	userID  int64    // 用户ID（必须）
	name    string   // 姓名
}

// NewStaffRef 创建员工引用
func NewStaffRef(staffID staff.ID, userID int64, name string) *StaffRef {
	return &StaffRef{
		staffID: staffID,
		userID:  userID,
		name:    name,
	}
}

// StaffID 获取员工ID
func (r *StaffRef) StaffID() staff.ID {
	return r.staffID
}

// UserID 获取用户ID
func (r *StaffRef) UserID() int64 {
	return r.userID
}

// Name 获取姓名
func (r *StaffRef) Name() string {
	return r.name
}
