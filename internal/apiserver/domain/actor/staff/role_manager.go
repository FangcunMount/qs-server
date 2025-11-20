package staff

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// RoleManager 角色管理器领域服务
// 负责 Staff 的角色分配、移除、权限检查
type RoleManager interface {
	// AssignRole 分配角色
	AssignRole(staff *Staff, role Role) error

	// RemoveRole 移除角色
	RemoveRole(staff *Staff, role Role) error

	// AssignRoles 批量分配角色
	AssignRoles(staff *Staff, roles []Role) error

	// ReplaceRoles 替换所有角色
	ReplaceRoles(staff *Staff, roles []Role) error

	// ClearRoles 清空所有角色
	ClearRoles(staff *Staff) error

	// ValidatePermission 验证是否有某个权限
	// 返回 nil 表示有权限，返回 error 表示无权限
	ValidatePermission(staff *Staff, requiredRoles ...Role) error
}

// roleManager 角色管理器实现
type roleManager struct {
	validator Validator
}

// NewRoleManager 创建角色管理器
func NewRoleManager(validator Validator) RoleManager {
	return &roleManager{
		validator: validator,
	}
}

// AssignRole 分配角色
func (rm *roleManager) AssignRole(staff *Staff, role Role) error {
	// 验证角色合法性
	if err := rm.validator.ValidateRole(role); err != nil {
		return err
	}

	// 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot assign role to inactive staff")
	}

	// 执行分配
	staff.assignRole(role)

	return nil
}

// RemoveRole 移除角色
func (rm *roleManager) RemoveRole(staff *Staff, role Role) error {
	// 验证角色合法性
	if err := rm.validator.ValidateRole(role); err != nil {
		return err
	}

	// 检查是否有该角色
	if !staff.HasRole(role) {
		return errors.WithCode(code.ErrValidation, "staff does not have this role")
	}

	// 执行移除
	staff.removeRole(role)

	return nil
}

// AssignRoles 批量分配角色
func (rm *roleManager) AssignRoles(staff *Staff, roles []Role) error {
	// 验证角色列表
	if err := rm.validator.ValidateRoles(roles); err != nil {
		return err
	}

	// 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot assign roles to inactive staff")
	}

	// 批量分配
	for _, role := range roles {
		staff.assignRole(role)
	}

	return nil
}

// ReplaceRoles 替换所有角色
func (rm *roleManager) ReplaceRoles(staff *Staff, roles []Role) error {
	// 验证角色列表
	if err := rm.validator.ValidateRoles(roles); err != nil {
		return err
	}

	// 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrValidation, "cannot replace roles for inactive staff")
	}

	// 清空现有角色
	staff.roles = make([]Role, 0)

	// 添加新角色（去重）
	roleSet := make(map[Role]bool)
	for _, role := range roles {
		if !roleSet[role] {
			staff.assignRole(role)
			roleSet[role] = true
		}
	}

	return nil
}

// ClearRoles 清空所有角色
func (rm *roleManager) ClearRoles(staff *Staff) error {
	staff.roles = make([]Role, 0)
	return nil
}

// ValidatePermission 验证权限
func (rm *roleManager) ValidatePermission(staff *Staff, requiredRoles ...Role) error {
	// 检查员工是否激活
	if !staff.IsActive() {
		return errors.WithCode(code.ErrPermissionDenied, "staff is not active")
	}

	// 检查是否有任意一个所需角色
	if !staff.HasAnyRole(requiredRoles...) {
		return errors.WithCode(code.ErrPermissionDenied, "insufficient permissions")
	}

	return nil
}
