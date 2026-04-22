package operator

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
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
	assignment    *iaminfra.AuthzAssignmentClient
	snapshot      *iaminfra.AuthzSnapshotLoader
}

// NewAuthorizationService 创建操作者权限管理服务
func NewAuthorizationService(
	repo domain.Repository,
	validator domain.Validator,
	roleAllocator domain.RoleAllocator,
	lifecycler domain.Lifecycler,
	uow *mysql.UnitOfWork,
	assignment *iaminfra.AuthzAssignmentClient,
	snapshot *iaminfra.AuthzSnapshotLoader,
) OperatorAuthorizationService {
	return &authorizationService{
		repo:          repo,
		validator:     validator,
		roleAllocator: roleAllocator,
		lifecycler:    lifecycler,
		uow:           uow,
		assignment:    assignment,
		snapshot:      snapshot,
	}
}

// AssignRole 分配角色（IAM 启用时先 GrantAssignment，再以快照刷新本地投影）。
func (s *authorizationService) AssignRole(ctx context.Context, operatorID uint64, roleName string) error {
	role := domain.Role(roleName)
	if err := s.validator.ValidateRole(role); err != nil {
		return err
	}
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}

	st, err := s.repo.FindByID(ctx, targetOperatorID)
	if err != nil {
		return errors.Wrap(err, "failed to find operator")
	}

	if s.assignment != nil && s.snapshot != nil {
		dom := s.snapshot.DomainForOrg(st.OrgID())
		if err := s.assignment.Grant(ctx, dom, strconv.FormatInt(st.UserID(), 10), roleName, actorctx.IAMGrantedBySubject(ctx)); err != nil {
			return errors.Wrap(err, "iam grant assignment")
		}
		if _, err := iaminfra.SyncAndPersistOperatorRolesFromSnapshot(ctx, s.snapshot, s.repo, st.OrgID(), st); err != nil {
			return errors.Wrap(err, "sync roles from iam snapshot")
		}
		return nil
	}

	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		st2, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}
		if err := s.roleAllocator.AssignRole(st2, role); err != nil {
			return err
		}
		return s.repo.Update(txCtx, st2)
	})
}

// RemoveRole 移除角色
func (s *authorizationService) RemoveRole(ctx context.Context, operatorID uint64, roleName string) error {
	role := domain.Role(roleName)
	if err := s.validator.ValidateRole(role); err != nil {
		return err
	}
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}

	st, err := s.repo.FindByID(ctx, targetOperatorID)
	if err != nil {
		return errors.Wrap(err, "failed to find operator")
	}

	if s.assignment != nil && s.snapshot != nil {
		dom := s.snapshot.DomainForOrg(st.OrgID())
		if err := s.assignment.Revoke(ctx, dom, strconv.FormatInt(st.UserID(), 10), roleName); err != nil {
			return errors.Wrap(err, "iam revoke assignment")
		}
		if _, err := iaminfra.SyncAndPersistOperatorRolesFromSnapshot(ctx, s.snapshot, s.repo, st.OrgID(), st); err != nil {
			return errors.Wrap(err, "sync roles from iam snapshot")
		}
		return nil
	}

	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		st2, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}
		if err := s.roleAllocator.RemoveRole(st2, role); err != nil {
			return err
		}
		return s.repo.Update(txCtx, st2)
	})
}

// Activate 激活操作者
func (s *authorizationService) Activate(ctx context.Context, operatorID uint64) error {
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		st, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}
		if err := s.lifecycler.Activate(st); err != nil {
			return err
		}
		return s.repo.Update(txCtx, st)
	})
}

// Deactivate 停用操作者
func (s *authorizationService) Deactivate(ctx context.Context, operatorID uint64) error {
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		st, err := s.repo.FindByID(txCtx, targetOperatorID)
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}
		if err := s.lifecycler.Deactivate(st, "deactivated by admin"); err != nil {
			return err
		}
		return s.repo.Update(txCtx, st)
	})
}
