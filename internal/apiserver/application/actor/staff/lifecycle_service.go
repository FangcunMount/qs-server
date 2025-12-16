package staff

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	identityv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/identity/v1"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
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
	identitySvc   *iam.IdentityService
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
	identitySvc *iam.IdentityService,
) StaffLifecycleService {
	return &lifecycleService{
		repo:          repo,
		factory:       factory,
		validator:     validator,
		editor:        editor,
		roleAllocator: roleAllocator,
		binder:        binder,
		uow:           uow,
		identitySvc:   identitySvc,
	}
}

// Register 注册新员工
func (s *lifecycleService) Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error) {
	var result *staff.Staff

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 验证参数
		if err := s.validateRegisterDTO(dto); err != nil {
			return err
		}

		// 2. 解析或创建用户（先按手机号查，查不到再创建）
		userID, err := s.resolveOrCreateUser(ctx, dto)
		if err != nil {
			return err
		}

		// 3~5. 创建员工、分配角色并持久化
		st, err := s.createAndSaveStaff(txCtx, dto, userID)
		if err != nil {
			return err
		}
		result = st
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

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
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
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {

		if err := s.repo.Delete(txCtx, staff.ID(staffID)); err != nil {
			return errors.Wrap(err, "failed to delete staff")
		}
		return nil
	})
}

// UpdateContactInfo 更新联系方式
func (s *lifecycleService) UpdateContactInfo(ctx context.Context, dto UpdateStaffContactDTO) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {

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
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {

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

// validateRegisterDTO 校验 Register 所需的 DTO 字段
func (s *lifecycleService) validateRegisterDTO(dto RegisterStaffDTO) error {
	if err := s.validator.ValidateOrgID(dto.OrgID); err != nil {
		return err
	}
	if err := s.validator.ValidateName(dto.Name, true); err != nil {
		return err
	}
	return nil
}

// resolveOrCreateUser: 若 DTO 中已有 userID 则直接返回；否则先按 phone 搜索 IAM 用户，找到返回其 ID，未找到则创建新用户并返回
func (s *lifecycleService) resolveOrCreateUser(ctx context.Context, dto RegisterStaffDTO) (int64, error) {
	userID := dto.UserID
	if userID != 0 {
		return userID, nil
	}
	if s.identitySvc == nil || !s.identitySvc.IsEnabled() {
		return 0, errors.WithCode(code.ErrValidation, "user_id is required or IAM must be enabled to create user")
	}

	// 按手机号搜索
	searchReq := &identityv1.SearchUsersRequest{Phones: []string{dto.Phone}}
	searchResp, err := s.identitySvc.SearchUsers(ctx, searchReq)
	if err != nil {
		return 0, err
	}
	if searchResp != nil && len(searchResp.Users) > 0 {
		uidStr := searchResp.Users[0].Id
		if uidStr != "" {
			if uid, err := strconv.ParseInt(uidStr, 10, 64); err == nil {
				return uid, nil
			}
		}
	}

	// 未找到则创建
	return s.identitySvc.CreateUser(ctx, dto.Name, dto.Email, dto.Phone)
}

// createAndSaveStaff 在事务内检查是否已存在、创建 Staff、分配角色并保存
func (s *lifecycleService) createAndSaveStaff(txCtx context.Context, dto RegisterStaffDTO, userID int64) (*staff.Staff, error) {
	// 检查是否已存在
	_, err := s.repo.FindByUser(txCtx, dto.OrgID, userID)
	if err == nil {
		return nil, errors.WithCode(code.ErrUserAlreadyExists, "staff with this user_id already exists")
	}
	if !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, err
	}

	// 创建员工
	st := staff.NewStaff(dto.OrgID, userID, dto.Name)

	// 分配角色
	for _, roleName := range dto.Roles {
		role := staff.Role(roleName)
		if err := s.validator.ValidateRole(role); err != nil {
			return nil, err
		}
		if err := s.roleAllocator.AssignRole(st, role); err != nil {
			return nil, err
		}
	}

	// 持久化
	if err := s.repo.Save(txCtx, st); err != nil {
		return nil, errors.Wrap(err, "failed to save staff")
	}

	return st, nil
}
