package operator

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

// authorizationService 操作者权限管理服务实现
// 行为者：IT管理员/权限管理员
type authorizationService struct {
	repo       domain.Repository
	validator  domain.Validator
	lifecycler domain.Lifecycler
	uow        apptransaction.Runner
	authz      iambridge.OperatorAuthzGateway
}

// NewAuthorizationService 创建操作者权限管理服务
func NewAuthorizationService(
	repo domain.Repository,
	validator domain.Validator,
	lifecycler domain.Lifecycler,
	uow apptransaction.Runner,
	authz iambridge.OperatorAuthzGateway,
) OperatorAuthorizationService {
	return &authorizationService{
		repo:       repo,
		validator:  validator,
		lifecycler: lifecycler,
		uow:        uow,
		authz:      authz,
	}
}

func (s *authorizationService) ReplaceRoles(ctx context.Context, operatorID uint64, roleNames []string) error {
	if err := s.requireOperatorAuthz(); err != nil {
		return err
	}
	for _, roleName := range roleNames {
		if err := s.validator.ValidateRole(domain.Role(roleName)); err != nil {
			return err
		}
	}
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return err
	}
	op, err := s.repo.FindByID(ctx, targetOperatorID)
	if err != nil {
		return errors.Wrap(err, "failed to find operator")
	}
	committedVersion, err := s.authz.ReplaceManagedOperatorRoles(ctx, op.OrgID(), op.UserID(), roleNames,
		actorctx.IAMGrantedBySubject(ctx), "replace staff direct roles")
	if err != nil {
		return errors.Wrap(err, "iam replace managed assignments")
	}
	projection, loadErr := s.authz.LoadOperatorRoleProjection(ctx, op.OrgID(), op.UserID())
	if loadErr != nil || projection.PolicyVersion < committedVersion {
		op.MarkAuthzProjectionPending()
		_ = s.repo.Update(ctx, op)
		return nil
	}
	return persistOperatorRoleProjection(ctx, s.repo, op, projection, false)
}

func (s *authorizationService) requireOperatorAuthz() error {
	if s == nil || s.authz == nil || !s.authz.IsEnabled() {
		return errors.New("IAM operator authorization gateway is required")
	}
	return nil
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
		if err := s.lifecycler.Deactivate(st); err != nil {
			return err
		}
		return s.repo.Update(txCtx, st)
	})
}
