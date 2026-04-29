package operator

import (
	"context"
	"strconv"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	identityv1 "github.com/FangcunMount/iam/api/grpc/iam/identity/v1"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// lifecycleService 操作者生命周期服务实现
// 行为者：人事/行政部门
type lifecycleService struct {
	repo          domain.Repository
	factory       domain.Factory
	validator     domain.Validator
	editor        domain.Editor
	lifecycler    domain.Lifecycler
	roleAllocator domain.RoleAllocator
	binder        domain.Binder
	uow           apptransaction.Runner
	identitySvc   *iam.IdentityService
	accountSvc    *iam.OperationAccountService
	assignment    *iam.AuthzAssignmentClient
	snapshot      *iam.AuthzSnapshotLoader
}

// NewLifecycleService 创建操作者生命周期服务
func NewLifecycleService(
	repo domain.Repository,
	factory domain.Factory,
	validator domain.Validator,
	editor domain.Editor,
	lifecycler domain.Lifecycler,
	roleAllocator domain.RoleAllocator,
	binder domain.Binder,
	uow apptransaction.Runner,
	identitySvc *iam.IdentityService,
	accountSvc *iam.OperationAccountService,
	assignment *iam.AuthzAssignmentClient,
	snapshot *iam.AuthzSnapshotLoader,
) OperatorLifecycleService {
	return &lifecycleService{
		repo:          repo,
		factory:       factory,
		validator:     validator,
		editor:        editor,
		lifecycler:    lifecycler,
		roleAllocator: roleAllocator,
		binder:        binder,
		uow:           uow,
		identitySvc:   identitySvc,
		accountSvc:    accountSvc,
		assignment:    assignment,
		snapshot:      snapshot,
	}
}

// Register 注册新操作者
func (s *lifecycleService) Register(ctx context.Context, dto RegisterOperatorDTO) (*OperatorResult, error) {
	var result *domain.Operator
	var created bool

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

		// 3~5. 创建操作者、分配角色并持久化
		st, wasCreated, err := s.createAndSaveOperator(txCtx, dto, userID)
		if err != nil {
			return err
		}
		result = st
		created = wasCreated
		return nil
	})

	if err != nil {
		return nil, err
	}

	if dto.IsActive && s.assignment != nil && s.snapshot != nil {
		if err := s.syncIAMRolesAfterRegister(ctx, result, dto.Roles); err != nil {
			if created {
				if rollbackErr := s.rollbackRegisteredOperator(ctx, result.ID()); rollbackErr != nil {
					return nil, errors.Wrapf(rollbackErr, "iam role assignment after register failed and operator rollback failed: %v", err)
				}
				return nil, errors.Wrap(err, "iam role assignment after register; local operator rolled back")
			}
			return nil, errors.Wrap(err, "iam role assignment after ensure operator")
		}
	}

	return toOperatorResult(result), nil
}

// EnsureByUser 确保操作者存在（幂等）
func (s *lifecycleService) EnsureByUser(ctx context.Context, orgID int64, userID int64, name string) (*OperatorResult, error) {
	var result *domain.Operator

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 使用工厂的幂等创建方法
		var err error
		result, err = s.factory.GetOrCreateByUser(txCtx, orgID, userID, name)
		return err
	})

	if err != nil {
		return nil, err
	}

	return toOperatorResult(result), nil
}

// Delete 删除操作者
func (s *lifecycleService) Delete(ctx context.Context, operatorID uint64) error {
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.Delete(txCtx, targetOperatorID); err != nil {
			return errors.Wrap(err, "failed to delete operator")
		}
		return nil
	})
}

// UpdateProfile 更新本地员工投影资料。
func (s *lifecycleService) UpdateProfile(ctx context.Context, dto UpdateOperatorProfileDTO) (*OperatorResult, error) {
	var result *domain.Operator
	targetOperatorID, err := operatorIDFromUint64("operator_id", dto.OperatorID)
	if err != nil {
		return nil, err
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		st, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}

		if err := s.editor.UpdateBasicInfo(st, dto.Name); err != nil {
			return err
		}
		if err := s.editor.UpdateContactInfo(st, dto.Email, dto.Phone); err != nil {
			return err
		}
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update operator")
		}

		result = st
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toOperatorResult(result), nil
}

// UpdateContactInfo 更新联系方式
func (s *lifecycleService) UpdateContactInfo(ctx context.Context, dto UpdateOperatorContactDTO) error {
	targetOperatorID, err := operatorIDFromUint64("operator_id", dto.OperatorID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {

		// 1. 查找操作者
		st, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}

		// 2. 使用领域服务更新
		email := dto.Email
		phone := dto.Phone
		if err := s.editor.UpdateContactInfo(st, &email, &phone); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update operator")
		}

		return nil
	})
}

// UpdateFromExternalSource 从外部源更新操作者信息
func (s *lifecycleService) UpdateFromExternalSource(ctx context.Context, operatorID uint64, name, email, phone string) error {
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找操作者
		st, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
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
			return errors.Wrap(err, "failed to update operator")
		}

		return nil
	})
}

// validateRegisterDTO 校验 Register 所需的 DTO 字段
func (s *lifecycleService) validateRegisterDTO(dto RegisterOperatorDTO) error {
	if err := s.validator.ValidateOrgID(dto.OrgID); err != nil {
		return err
	}
	if err := s.validator.ValidateName(dto.Name, true); err != nil {
		return err
	}
	for _, rn := range dto.Roles {
		if err := s.validator.ValidateRole(domain.Role(rn)); err != nil {
			return err
		}
	}
	if s.assignment == nil || s.snapshot == nil {
		if len(dto.Roles) == 0 {
			return errors.WithCode(code.ErrValidation, "roles are required when IAM authorization is not enabled")
		}
	}
	if dto.UserID == 0 {
		if dto.Phone == "" {
			return errors.WithCode(code.ErrValidation, "phone is required when user_id is not provided")
		}
		if strings.TrimSpace(dto.Password) == "" {
			return errors.WithCode(code.ErrValidation, "password is required when user_id is not provided")
		}
	}
	return nil
}

// resolveOrCreateUser: 优先通过 IAM 注册运营账号（可同时创建 user/account/credential），否则回退到 legacy user-only 创建。
func (s *lifecycleService) resolveOrCreateUser(ctx context.Context, dto RegisterOperatorDTO) (int64, error) {
	if strings.TrimSpace(dto.Password) != "" {
		if s.accountSvc == nil || !s.accountSvc.IsEnabled() {
			return 0, errors.WithCode(code.ErrValidation, "IAM operation account service is not enabled")
		}
		result, err := s.accountSvc.RegisterOperationAccount(ctx, iam.RegisterOperationAccountInput{
			ExistingUserID: formatOptionalUserID(dto.UserID),
			Name:           dto.Name,
			Phone:          dto.Phone,
			Email:          dto.Email,
			ScopedTenantID: strconv.FormatInt(dto.OrgID, 10),
			Password:       dto.Password,
		})
		if err != nil {
			if dto.UserID == 0 && isUserAlreadyExistsErr(err) {
				userID, found, lookupErr := s.findExistingUserByPhone(ctx, dto.Phone)
				if lookupErr != nil {
					return 0, lookupErr
				}
				if found {
					return userID, nil
				}
			}
			return 0, err
		}
		return result.UserID, nil
	}

	userID := dto.UserID
	if userID != 0 {
		return userID, nil
	}
	return s.findOrCreateUserByPhone(ctx, dto)
}

func (s *lifecycleService) findOrCreateUserByPhone(ctx context.Context, dto RegisterOperatorDTO) (int64, error) {
	if s.identitySvc == nil || !s.identitySvc.IsEnabled() {
		return 0, errors.WithCode(code.ErrValidation, "user_id is required or IAM must be enabled to create user")
	}

	if userID, found, err := s.findExistingUserByPhone(ctx, dto.Phone); err != nil {
		return 0, err
	} else if found {
		return userID, nil
	}

	// 未找到则创建
	return s.identitySvc.CreateUser(ctx, dto.Name, dto.Email, dto.Phone)
}

func (s *lifecycleService) findExistingUserByPhone(ctx context.Context, phone string) (int64, bool, error) {
	if s.identitySvc == nil || !s.identitySvc.IsEnabled() || strings.TrimSpace(phone) == "" {
		return 0, false, nil
	}

	searchReq := &identityv1.SearchUsersRequest{Phones: []string{phone}}
	searchResp, err := s.identitySvc.SearchUsers(ctx, searchReq)
	if err != nil {
		return 0, false, err
	}
	if searchResp == nil || len(searchResp.Users) == 0 {
		return 0, false, nil
	}

	uidStr := strings.TrimSpace(searchResp.Users[0].Id)
	if uidStr == "" {
		return 0, false, nil
	}
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil {
		return 0, false, errors.Wrap(err, "failed to parse user id from IAM search result")
	}
	return uid, true, nil
}

func isUserAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.IsCode(err, code.ErrUserAlreadyExists) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "user already exists")
}

func formatOptionalUserID(userID int64) string {
	if userID == 0 {
		return ""
	}
	return strconv.FormatInt(userID, 10)
}

// createAndSaveOperator 在事务内检查是否已存在、创建 Operator、分配角色并保存
func (s *lifecycleService) createAndSaveOperator(txCtx context.Context, dto RegisterOperatorDTO, userID int64) (*domain.Operator, bool, error) {
	useIAM := s.assignment != nil && s.snapshot != nil

	st, err := s.repo.FindByUser(txCtx, dto.OrgID, userID)
	if err == nil {
		if err := s.syncOperatorProjection(st, dto, useIAM); err != nil {
			return nil, false, err
		}
		if err := s.repo.Update(txCtx, st); err != nil {
			return nil, false, errors.Wrap(err, "failed to update operator")
		}
		return st, false, nil
	}
	if !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, false, err
	}

	st = domain.NewOperator(dto.OrgID, userID, dto.Name)
	if err := s.syncOperatorProjection(st, dto, useIAM); err != nil {
		return nil, false, err
	}

	if err := s.repo.Save(txCtx, st); err != nil {
		if errors.IsCode(err, code.ErrUserAlreadyExists) {
			return nil, false, err
		}
		return nil, false, errors.Wrap(err, "failed to save operator")
	}

	return st, true, nil
}

func (s *lifecycleService) syncOperatorProjection(st *domain.Operator, dto RegisterOperatorDTO, useIAM bool) error {
	if err := s.editor.UpdateBasicInfo(st, &dto.Name); err != nil {
		return err
	}
	if err := s.editor.UpdateContactInfo(st, &dto.Email, &dto.Phone); err != nil {
		return err
	}

	if dto.IsActive {
		if err := s.lifecycler.Activate(st); err != nil {
			return err
		}
	} else {
		if err := s.lifecycler.Deactivate(st, "synced as inactive"); err != nil {
			return err
		}
	}

	if useIAM {
		return nil
	}

	roles := make([]domain.Role, 0, len(dto.Roles))
	for _, roleName := range dto.Roles {
		role := domain.Role(roleName)
		if err := s.validator.ValidateRole(role); err != nil {
			return err
		}
		roles = append(roles, role)
	}

	if !dto.IsActive {
		return s.roleAllocator.ClearRoles(st)
	}

	return s.roleAllocator.ReplaceRoles(st, roles)
}

func (s *lifecycleService) syncIAMRolesAfterRegister(ctx context.Context, op *domain.Operator, roleNames []string) error {
	dom := s.snapshot.DomainForOrg(op.OrgID())
	uidStr := strconv.FormatInt(op.UserID(), 10)
	for _, rn := range roleNames {
		role := domain.Role(rn)
		if err := s.validator.ValidateRole(role); err != nil {
			return err
		}
		if err := s.assignment.Grant(ctx, dom, uidStr, rn, actorctx.IAMGrantedBySubject(ctx)); err != nil {
			return err
		}
	}
	_, err := iam.SyncAndPersistOperatorRolesFromSnapshot(ctx, s.snapshot, s.repo, op.OrgID(), op)
	return err
}

func (s *lifecycleService) rollbackRegisteredOperator(ctx context.Context, id domain.ID) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.Delete(txCtx, id); err != nil {
			return errors.Wrap(err, "failed to rollback operator")
		}
		return nil
	})
}
