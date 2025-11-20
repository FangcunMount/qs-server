package staff_management

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// staffService 员工服务实现
type staffService struct {
	repo        staff.Repository
	factory     staff.Factory
	validator   staff.Validator
	roleManager staff.RoleManager
	uow         *mysql.UnitOfWork
}

// NewStaffService 创建员工服务
func NewStaffService(
	repo staff.Repository,
	factory staff.Factory,
	validator staff.Validator,
	roleManager staff.RoleManager,
	uow *mysql.UnitOfWork,
) StaffApplicationService {
	return &staffService{
		repo:        repo,
		factory:     factory,
		validator:   validator,
		roleManager: roleManager,
		uow:         uow,
	}
}

// Register 注册新员工
func (s *staffService) Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error) {
	var result *staff.Staff
	var err error

	err = s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 验证参数
		if err := s.validator.ValidateOrgID(dto.OrgID); err != nil {
			return err
		}
		if err := s.validator.ValidateName(dto.Name, true); err != nil {
			return err
		}

		// 2. 检查是否已存在
		_, err := s.repo.FindByIAMUser(txCtx, dto.OrgID, dto.IAMUserID)
		if err == nil {
			return errors.WithCode(code.ErrUserAlreadyExists, "staff with this iam_user_id already exists")
		}
		if !errors.IsCode(err, code.ErrUserNotFound) && err != nil {
			return err
		}

		// 3. 创建员工
		result = staff.NewStaff(dto.OrgID, dto.IAMUserID, dto.Name)

		// 4. 分配角色
		for _, roleName := range dto.Roles {
			role := staff.Role(roleName)
			if err := s.validator.ValidateRole(role); err != nil {
				return err
			}
			if err := s.roleManager.AssignRole(result, role); err != nil {
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

// EnsureByIAMUser 确保员工存在（幂等）
func (s *staffService) EnsureByIAMUser(ctx context.Context, orgID int64, iamUserID int64, name string) (*StaffResult, error) {
	var result *staff.Staff
	var err error

	err = s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)
		// 使用工厂的幂等创建方法
		result, err = s.factory.GetOrCreateByIAMUser(txCtx, orgID, iamUserID, name)
		return err
	})

	if err != nil {
		return nil, err
	}

	return toStaffResult(result), nil
}

// Delete 删除员工
func (s *staffService) Delete(ctx context.Context, staffID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)
		if err := s.repo.Delete(txCtx, staff.ID(staffID)); err != nil {
			return errors.Wrap(err, "failed to delete staff")
		}
		return nil
	})
}

// toStaffResult 将领域对象转换为 DTO
func toStaffResult(s *staff.Staff) *StaffResult {
	if s == nil {
		return nil
	}

	// 转换角色列表
	roles := make([]string, len(s.Roles()))
	for i, role := range s.Roles() {
		roles[i] = string(role)
	}

	return &StaffResult{
		ID:        uint64(s.ID()),
		OrgID:     s.OrgID(),
		IAMUserID: s.IAMUserID(),
		Roles:     roles,
		Name:      s.Name(),
		Email:     s.Email(),
		Phone:     s.Phone(),
		IsActive:  s.IsActive(),
	}
}
