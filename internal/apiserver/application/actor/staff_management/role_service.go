package staff_management

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// roleService 员工角色服务实现
type roleService struct {
	repo        staff.Repository
	validator   staff.Validator
	roleManager staff.RoleManager
	editor      staff.Editor
	uow         *mysql.UnitOfWork
}

// NewRoleService 创建员工角色服务
func NewRoleService(
	repo staff.Repository,
	validator staff.Validator,
	roleManager staff.RoleManager,
	editor staff.Editor,
	uow *mysql.UnitOfWork,
) StaffRoleApplicationService {
	return &roleService{
		repo:        repo,
		validator:   validator,
		roleManager: roleManager,
		editor:      editor,
		uow:         uow,
	}
}

// AssignRole 分配角色
func (s *roleService) AssignRole(ctx context.Context, staffID uint64, roleName string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 验证角色
		role := staff.Role(roleName)
		if err := s.validator.ValidateRole(role); err != nil {
			return err
		}

		// 3. 使用领域服务分配角色
		if err := s.roleManager.AssignRole(st, role); err != nil {
			return err
		}

		// 4. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}

// RemoveRole 移除角色
func (s *roleService) RemoveRole(ctx context.Context, staffID uint64, roleName string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务移除角色
		role := staff.Role(roleName)
		if err := s.roleManager.RemoveRole(st, role); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}

// Activate 激活员工
func (s *roleService) Activate(ctx context.Context, staffID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务激活
		if err := s.editor.Activate(st); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}

// Deactivate 停用员工
func (s *roleService) Deactivate(ctx context.Context, staffID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务停用（需要提供原因）
		if err := s.editor.Deactivate(st, "deactivated by admin"); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}
