package operator

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Lifecycler Operator生命周期管理领域服务
// 负责管理 Operator 的生命周期（激活、停用）
type Lifecycler interface {
	// Activate 激活员工
	Activate(operator *Operator) error

	// Deactivate 停用员工
	Deactivate(operator *Operator) error
}

// lifecycler 生命周期管理器实现
type lifecycler struct{}

// NewLifecycler 创建生命周期管理器
func NewLifecycler() Lifecycler {
	return &lifecycler{}
}

// Activate 激活员工
func (lc *lifecycler) Activate(operator *Operator) error {
	// 1. 检查是否已激活（幂等）
	if operator.IsActive() {
		return nil
	}

	// 2. 业务规则：激活前必须已绑定用户
	if operator.UserID() <= 0 {
		return errors.WithCode(code.ErrValidation, "cannot activate operator without user binding")
	}

	// 3. 执行激活
	operator.activate()

	return nil
}

// Deactivate 停用员工
func (lc *lifecycler) Deactivate(operator *Operator) error {
	// 1. 检查是否已停用（幂等）
	if !operator.IsActive() {
		return nil
	}

	// 停用只改变 QS 业务状态；IAM Assignment 与本地角色投影保持不变。
	operator.deactivate()

	return nil
}
