package staff

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// RoleAllocator 角色分配器领域服务
// 负责 Staff 的角色分配、移除和清空
type RoleAllocator interface {
	// AssignRole 分配单个角色
	AssignRole(staff *Staff, role Role) error

	// RemoveRole 移除单个角色
	RemoveRole(staff *Staff, role Role) error

	// ClearRoles 清空所有角色
	ClearRoles(staff *Staff) error

	// AssignRoles 批量分配角色
	AssignRoles(staff *Staff, roles []Role) error

	// ReplaceRoles 替换所有角色
	ReplaceRoles(staff *Staff, roles []Role) error
}

// roleAllocator 角色分配器实现
type roleAllocator struct {
	validator Validator
}

// NewRoleAllocator 创建角色分配器
func NewRoleAllocator(validator Validator) RoleAllocator {
	return &roleAllocator{
		validator: validator,
	}
}

// AssignRole 分配角色
func (ra *roleAllocator) AssignRole(staff *Staff, role Role) error {
	// 1. 验证角色合法性
	if err := ra.validator.ValidateRole(role); err != nil {
		return err
	}

	// 2. 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot assign role to inactive staff")
	}

	// 3. 检查是否已有该角色（幂等）
	if staff.HasRole(role) {
		return nil
	}

	// 4. 执行分配
	staff.assignRole(role)

	return nil
}

// RemoveRole 移除角色
func (ra *roleAllocator) RemoveRole(staff *Staff, role Role) error {
	// 1. 验证角色合法性
	if err := ra.validator.ValidateRole(role); err != nil {
		return err
	}

	// 2. 检查是否有该角色
	if !staff.HasRole(role) {
		return nil // 幂等操作
	}

	// 3. 执行移除
	staff.removeRole(role)

	return nil
}

// ClearRoles 清空所有角色
func (ra *roleAllocator) ClearRoles(staff *Staff) error {
	// 清空角色列表
	staff.roles = make([]Role, 0)
	return nil
}

// AssignRoles 批量分配角色
func (ra *roleAllocator) AssignRoles(staff *Staff, roles []Role) error {
	// 1. 验证角色列表
	if err := ra.validator.ValidateRoles(roles); err != nil {
		return err
	}

	// 2. 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot assign roles to inactive staff")
	}

	// 3. 批量分配（去重）
	for _, role := range roles {
		if !staff.HasRole(role) {
			staff.assignRole(role)
		}
	}

	return nil
}

// ReplaceRoles 替换所有角色
func (ra *roleAllocator) ReplaceRoles(staff *Staff, roles []Role) error {
	// 1. 验证角色列表
	if err := ra.validator.ValidateRoles(roles); err != nil {
		return err
	}

	// 2. 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot replace roles for inactive staff")
	}

	// 3. 清空现有角色
	staff.roles = make([]Role, 0)

	// 4. 设置新角色（去重）
	seen := make(map[Role]bool)
	for _, role := range roles {
		if !seen[role] {
			staff.assignRole(role)
			seen[role] = true
		}
	}

	return nil
}
