package operator

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// RoleAllocator 角色分配器领域服务
// 负责 Operator 的角色分配、移除和清空
type RoleAllocator interface {
	// AssignRole 分配单个角色
	AssignRole(staff *Operator, role Role) error

	// RemoveRole 移除单个角色
	RemoveRole(staff *Operator, role Role) error

	// ClearRoles 清空所有角色
	ClearRoles(staff *Operator) error

	// AssignRoles 批量分配角色
	AssignRoles(staff *Operator, roles []Role) error

	// ReplaceRoles 替换所有角色
	ReplaceRoles(staff *Operator, roles []Role) error
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

// IsSupportedRole 判断角色是否为当前 QS 支持的角色。
func IsSupportedRole(role Role) bool {
	switch role {
	case RoleQSAdmin, RoleContentManager, RoleEvaluatorQS, RoleOperator,
		RoleEvaluationPlanManager:
		return true
	default:
		return false
	}
}

// AssignRole 分配角色
func (ra *roleAllocator) AssignRole(staff *Operator, role Role) error {
	// 1. 验证角色合法性
	if err := ra.validator.ValidateRole(role); err != nil {
		return err
	}

	return staff.AssignRole(role)
}

// RemoveRole 移除角色
func (ra *roleAllocator) RemoveRole(staff *Operator, role Role) error {
	// 1. 验证角色合法性
	if err := ra.validator.ValidateRole(role); err != nil {
		return err
	}

	return staff.RemoveRole(role)
}

// ClearRoles 清空所有角色
func (ra *roleAllocator) ClearRoles(staff *Operator) error {
	staff.ClearRoles()
	return nil
}

// AssignRoles 批量分配角色
func (ra *roleAllocator) AssignRoles(staff *Operator, roles []Role) error {
	// 1. 验证角色列表
	if err := ra.validator.ValidateRoles(roles); err != nil {
		return err
	}

	for _, role := range roles {
		if err := staff.AssignRole(role); err != nil {
			if !staff.IsActive() {
				return inactiveRoleAssignmentError("cannot assign roles to inactive staff")
			}
			return err
		}
	}

	return nil
}

// ReplaceRoles 替换所有角色
func (ra *roleAllocator) ReplaceRoles(staff *Operator, roles []Role) error {
	// 1. 验证角色列表
	if err := ra.validator.ValidateRoles(roles); err != nil {
		return err
	}

	return staff.ReplaceRoles(roles)
}

func invalidRoleError() error {
	return errors.WithCode(code.ErrValidation, "invalid role")
}

func inactiveRoleAssignmentError(message string) error {
	return errors.WithCode(code.ErrValidation, "%s", message)
}
