package staff

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// authorizationService 员工权限管理服务实现
// 行为者：IT管理员/权限管理员
type authorizationService struct {
	repo          staff.Repository
	validator     staff.Validator
	roleAllocator staff.RoleAllocator
	lifecycler    staff.Lifecycler
	uow           *mysql.UnitOfWork
}

// NewAuthorizationService 创建员工权限管理服务
func NewAuthorizationService(
	repo staff.Repository,
	validator staff.Validator,
	roleAllocator staff.RoleAllocator,
	lifecycler staff.Lifecycler,
	uow *mysql.UnitOfWork,
) StaffAuthorizationService {
	return &authorizationService{
		repo:          repo,
		validator:     validator,
		roleAllocator: roleAllocator,
		lifecycler:    lifecycler,
		uow:           uow,
	}
}

// AssignRole 分配角色
func (s *authorizationService) AssignRole(ctx context.Context, staffID uint64, roleName string) error {
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
		if err := s.roleAllocator.AssignRole(st, role); err != nil {
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
func (s *authorizationService) RemoveRole(ctx context.Context, staffID uint64, roleName string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务移除角色
		role := staff.Role(roleName)
		if err := s.roleAllocator.RemoveRole(st, role); err != nil {
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
func (s *authorizationService) Activate(ctx context.Context, staffID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务激活
		if err := s.lifecycler.Activate(st); err != nil {
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
func (s *authorizationService) Deactivate(ctx context.Context, staffID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务停用（需要提供原因）
		if err := s.lifecycler.Deactivate(st, "deactivated by admin"); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}
