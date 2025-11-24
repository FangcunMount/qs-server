package staff

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// PermissionValidator 权限验证器领域服务
// 负责检查 Staff 的权限
type PermissionValidator interface {
	// Validate 验证是否有指定的权限
	// 返回 nil 表示有权限，返回 error 表示无权限
	Validate(staff *Staff, requiredRoles ...Role) error

	// ValidateAny 验证是否有任意一个权限
	// 返回 nil 表示至少有一个权限，返回 error 表示都没有
	ValidateAny(staff *Staff, requiredRoles ...Role) error

	// ValidateAll 验证是否拥有所有权限
	// 返回 nil 表示拥有所有权限，返回 error 表示缺少某些权限
	ValidateAll(staff *Staff, requiredRoles ...Role) error

	// ValidateActive 验证是否激活
	ValidateActive(staff *Staff) error
}

// permissionValidator 权限验证器实现
type permissionValidator struct{}

// NewPermissionValidator 创建权限验证器
func NewPermissionValidator() PermissionValidator {
	return &permissionValidator{}
}

// Validate 验证权限（默认使用 ValidateAny 逻辑）
func (pv *permissionValidator) Validate(staff *Staff, requiredRoles ...Role) error {
	return pv.ValidateAny(staff, requiredRoles...)
}

// ValidateAny 验证是否有任意一个权限
func (pv *permissionValidator) ValidateAny(staff *Staff, requiredRoles ...Role) error {
	// 1. 检查员工是否激活
	if err := pv.ValidateActive(staff); err != nil {
		return err
	}

	// 2. 如果没有要求任何角色，直接通过
	if len(requiredRoles) == 0 {
		return nil
	}

	// 3. 检查是否至少有一个角色
	for _, role := range requiredRoles {
		if staff.HasRole(role) {
			return nil
		}
	}

	return errors.WithCode(code.ErrPermissionDenied, "staff does not have required permission")
}

// ValidateAll 验证是否拥有所有权限
func (pv *permissionValidator) ValidateAll(staff *Staff, requiredRoles ...Role) error {
	// 1. 检查员工是否激活
	if err := pv.ValidateActive(staff); err != nil {
		return err
	}

	// 2. 如果没有要求任何角色，直接通过
	if len(requiredRoles) == 0 {
		return nil
	}

	// 3. 检查是否拥有所有角色
	missRoles := make([]string, 0)
	for _, role := range requiredRoles {
		if !staff.HasRole(role) {
			missRoles = append(missRoles, role.String())
		}
	}
	if len(missRoles) > 0 {
		msg := fmt.Sprintf("staff missing required roles: %s", strings.Join(missRoles, ", "))
		return errors.WithCode(code.ErrPermissionDenied, msg)
	}

	return nil
}

// ValidateActive 验证是否激活
func (pv *permissionValidator) ValidateActive(staff *Staff) error {
	if !staff.IsActive() {
		return errors.WithCode(code.ErrPermissionDenied, "staff is not active")
	}
	return nil
}
