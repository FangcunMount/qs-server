package staff

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// lifecycleService 员工生命周期服务实现
// 行为者：人事/行政部门
type lifecycleService struct {
	repo          staff.Repository
	factory       staff.Factory
	validator     staff.Validator
	editor        staff.Editor
	roleAllocator staff.RoleAllocator
	binder        staff.Binder
	uow           *mysql.UnitOfWork
}

// NewLifecycleService 创建员工生命周期服务
func NewLifecycleService(
	repo staff.Repository,
	factory staff.Factory,
	validator staff.Validator,
	editor staff.Editor,
	roleAllocator staff.RoleAllocator,
	binder staff.Binder,
	uow *mysql.UnitOfWork,
) StaffLifecycleService {
	return &lifecycleService{
		repo:          repo,
		factory:       factory,
		validator:     validator,
		editor:        editor,
		roleAllocator: roleAllocator,
		binder:        binder,
		uow:           uow,
	}
}

// Register 注册新员工
func (s *lifecycleService) Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error) {
	var result *staff.Staff

	err := s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 验证参数
		if err := s.validator.ValidateOrgID(dto.OrgID); err != nil {
			return err
		}
		if err := s.validator.ValidateName(dto.Name, true); err != nil {
			return err
		}

		// 2. 检查是否已存在
		_, err := s.repo.FindByUser(txCtx, dto.OrgID, dto.UserID)
		if err == nil {
			return errors.WithCode(code.ErrUserAlreadyExists, "staff with this user_id already exists")
		}
		if !errors.IsCode(err, code.ErrUserNotFound) {
			return err
		}

		// 3. 创建员工
		result = staff.NewStaff(dto.OrgID, dto.UserID, dto.Name)

		// 4. 分配角色
		for _, roleName := range dto.Roles {
			role := staff.Role(roleName)
			if err := s.validator.ValidateRole(role); err != nil {
				return err
			}
			if err := s.roleAllocator.AssignRole(result, role); err != nil {
				return err
			}
		}

		// 5. 持久化
		if err := s.repo.Save(txCtx, result); err != nil {
			return errors.Wrap(err, "failed to save staff")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return toStaffResult(result), nil
}

// EnsureByUser 确保员工存在（幂等）
func (s *lifecycleService) EnsureByUser(ctx context.Context, orgID int64, userID int64, name string) (*StaffResult, error) {
	var result *staff.Staff

	err := s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)
		// 使用工厂的幂等创建方法
		var err error
		result, err = s.factory.GetOrCreateByUser(txCtx, orgID, userID, name)
		return err
	})

	if err != nil {
		return nil, err
	}

	return toStaffResult(result), nil
}

// Delete 删除员工
func (s *lifecycleService) Delete(ctx context.Context, staffID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)
		if err := s.repo.Delete(txCtx, staff.ID(staffID)); err != nil {
			return errors.Wrap(err, "failed to delete staff")
		}
		return nil
	})
}

// UpdateContactInfo 更新联系方式
func (s *lifecycleService) UpdateContactInfo(ctx context.Context, dto UpdateStaffContactDTO) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(dto.StaffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务更新
		email := dto.Email
		phone := dto.Phone
		if err := s.editor.UpdateContactInfo(st, &email, &phone); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}

// UpdateFromExternalSource 从外部源更新员工信息
func (s *lifecycleService) UpdateFromExternalSource(ctx context.Context, staffID uint64, name, email, phone string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务更新
		if err := s.editor.UpdateBasicInfo(st, &name); err != nil {
			return err
		}
		if err := s.editor.UpdateContactInfo(st, &email, &phone); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}
