package operator

import (
	"context"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// authorizationService 操作者权限管理服务实现
// 行为者：IT管理员/权限管理员
type authorizationService struct {
	gate       retirement.MutationGate
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
	gates ...retirement.MutationGate,
) OperatorAuthorizationService {
	var gate retirement.MutationGate
	if len(gates) > 0 {
		gate = gates[0]
	}
	return &authorizationService{
		gate:       gate,
		repo:       repo,
		validator:  validator,
		lifecycler: lifecycler,
		uow:        uow,
		authz:      authz,
	}
}

// Activate 激活操作者
func (s *authorizationService) activate(ctx context.Context, operatorID uint64) error {
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
func (s *authorizationService) deactivate(ctx context.Context, operatorID uint64) error {
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

func (s *authorizationService) mutate(ctx context.Context, id uint64, fn func(context.Context) error) error {
	if s.gate == nil {
		return fn(ctx)
	}
	targetID, err := operatorIDFromUint64("operator_id", id)
	if err != nil {
		return err
	}
	op, err := s.repo.FindByID(ctx, targetID)
	if err != nil {
		return err
	}
	return s.gate.WithinMutation(ctx, op.UserID(), fn)
}
func (s *authorizationService) ReplaceRoles(ctx context.Context, id uint64, roles []string) error {
	return errors.WithCode(code.ErrValidation, "role-only updates are retired; use authorization-scope with an explicit range and policy version")
}
func (s *authorizationService) Activate(ctx context.Context, id uint64) error {
	return s.mutate(ctx, id, func(locked context.Context) error { return s.activate(locked, id) })
}
func (s *authorizationService) Deactivate(ctx context.Context, id uint64) error {
	return s.mutate(ctx, id, func(locked context.Context) error { return s.deactivate(locked, id) })
}
