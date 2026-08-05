package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// 设计说明：
// 本文件定义的值对象用于跨聚合根引用，遵循 DDD 最佳实践。
// 当前由 AnswerSheet.SubmissionContext 持有，并通过归因快照连接 Assessment。

// TesteeRef 受试者引用（值对象）
// 用于在其他聚合根（如 AnswerSheet、Assessment）中引用受试者
// 避免跨聚合根直接依赖实体，保持松耦合
type TesteeRef struct {
	testeeID  testee.ID // 受试者ID
	profileID *uint64   // 可选：IAM Profile ID
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
