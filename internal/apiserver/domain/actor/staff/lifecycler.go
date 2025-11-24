package staff

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Lifecycler Staff生命周期管理领域服务
// 负责管理 Staff 的生命周期（激活、停用）
type Lifecycler interface {
	// Activate 激活员工
	Activate(staff *Staff) error

	// Deactivate 停用员工
	// reason: 停用原因（用于审计）
	Deactivate(staff *Staff, reason string) error
}

// lifecycler 生命周期管理器实现
type lifecycler struct {
	roleAllocator RoleAllocator
}

// NewLifecycler 创建生命周期管理器
func NewLifecycler(roleAllocator RoleAllocator) Lifecycler {
	return &lifecycler{
		roleAllocator: roleAllocator,
	}
}

// Activate 激活员工
func (lc *lifecycler) Activate(staff *Staff) error {
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

	// TODO: 发布领域事件
	// events.Publish(NewStaffActivatedEvent(staff.ID()))

	return nil
}

// Deactivate 停用员工
func (lc *lifecycler) Deactivate(staff *Staff, reason string) error {
	// 1. 检查是否已停用（幂等）
	if !staff.IsActive() {
		return nil
	}

	// 2. 业务规则：停用时应清空所有角色
	if len(staff.Roles()) > 0 {
		if err := lc.roleAllocator.ClearRoles(staff); err != nil {
			return errors.Wrap(err, "failed to clear roles during deactivation")
		}
	}

	// 3. 执行停用
	staff.deactivate()

	// TODO: 发布领域事件
	// events.Publish(NewStaffDeactivatedEvent(staff.ID(), reason))

	return nil
}
