package operator

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// authorizationService 操作者权限管理服务实现
// 行为者：IT管理员/权限管理员
type authorizationService struct {
	repo          domain.Repository
	validator     domain.Validator
	roleAllocator domain.RoleAllocator
	lifecycler    domain.Lifecycler
	uow           *mysql.UnitOfWork
}

// NewAuthorizationService 创建操作者权限管理服务
func NewAuthorizationService(
	repo domain.Repository,
	validator domain.Validator,
	roleAllocator domain.RoleAllocator,
	lifecycler domain.Lifecycler,
	uow *mysql.UnitOfWork,
) OperatorAuthorizationService {
	return &authorizationService{
		repo:          repo,
		validator:     validator,
		roleAllocator: roleAllocator,
		lifecycler:    lifecycler,
		uow:           uow,
	}
}

// AssignRole 分配角色
func (s *authorizationService) AssignRole(ctx context.Context, operatorID uint64, roleName string) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找操作者
		st, err := s.repo.FindByID(txCtx, domain.ID(operatorID))
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}

		// 2. 验证角色
		role := domain.Role(roleName)
		if err := s.validator.ValidateRole(role); err != nil {
			return err
		}

		// 3. 使用领域服务分配角色
		if err := s.roleAllocator.AssignRole(st, role); err != nil {
			return err
		}

		// 4. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update operator")
		}

		return nil
	})
}

// RemoveRole 移除角色
func (s *authorizationService) RemoveRole(ctx context.Context, operatorID uint64, roleName string) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找操作者
		st, err := s.repo.FindByID(txCtx, domain.ID(operatorID))
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}

		// 2. 使用领域服务移除角色
		role := domain.Role(roleName)
		if err := s.roleAllocator.RemoveRole(st, role); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update operator")
		}

		return nil
	})
}

// Activate 激活操作者
func (s *authorizationService) Activate(ctx context.Context, operatorID uint64) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找操作者
		st, err := s.repo.FindByID(txCtx, domain.ID(operatorID))
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}

		// 2. 使用领域服务激活
		if err := s.lifecycler.Activate(st); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update operator")
		}

		return nil
	})
}

// Deactivate 停用操作者
func (s *authorizationService) Deactivate(ctx context.Context, operatorID uint64) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找操作者
		st, err := s.repo.FindByID(txCtx, domain.ID(operatorID))
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}

		// 2. 使用领域服务停用（需要提供原因）
		if err := s.lifecycler.Deactivate(st, "deactivated by admin"); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update operator")
		}

		return nil
	})
}
