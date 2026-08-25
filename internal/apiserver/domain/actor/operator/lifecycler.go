package operator

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Lifecycler Operator生命周期管理领域服务
// 负责管理 Operator 的生命周期（激活、停用）
type Lifecycler interface {
	// Activate 激活员工
	Activate(staff *Operator) error

	// Deactivate 停用员工
	Deactivate(staff *Operator) error
}

// lifecycler 生命周期管理器实现
type lifecycler struct{}

// NewLifecycler 创建生命周期管理器
func NewLifecycler() Lifecycler {
	return &lifecycler{}
}

// Activate 激活员工
func (lc *lifecycler) Activate(staff *Operator) error {
	// 1. 检查是否已激活（幂等）
	if staff.IsActive() {
		return nil
	}

	// 2. 业务规则：激活前必须已绑定用户
	if staff.UserID() <= 0 {
		return errors.WithCode(code.ErrValidation, "cannot activate staff without user binding")
	}

	// 3. 执行激活
	staff.activate()

	return nil
}

// Deactivate 停用员工
func (lc *lifecycler) Deactivate(staff *Operator) error {
	// 1. 检查是否已停用（幂等）
	if !staff.IsActive() {
		return nil
	}

	// 2. 业务规则：停用时应清空所有角色
	if len(staff.Roles()) > 0 {
		staff.ReplaceRolesProjection(nil)
	}

	// 3. 执行停用
	staff.deactivate()

	return nil
}
