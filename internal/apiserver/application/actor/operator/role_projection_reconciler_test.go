package operator

import (
	"context"
	"errors"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

func TestRoleProjectionReconcilerConvergesPendingOperator(t *testing.T) {
	t.Parallel()

	op := domain.NewOperator(1, 101, "operator")
	op.SetID(201)
	op.ReplaceRolesProjection([]domain.Role{domain.RoleEvaluatorQS}, []domain.Role{domain.RoleEvaluatorQS}, 10, nil, true)
	repo := &pendingProjectionRepoStub{operators: []*domain.Operator{op}}
	gateway := &projectionGatewayStub{projection: iambridge.OperatorRoleProjection{
		DirectRoles:    []string{string(domain.RoleEvaluationPlanManager)},
		EffectiveRoles: []string{string(domain.RoleEvaluationPlanManager), string(domain.RoleOperator)},
		PolicyVersion:  12,
	}}
	reconciler := NewRoleProjectionReconciler(repo, gateway)

	result, err := reconciler.ReconcilePending(context.Background(), 25)

	if err != nil {
		t.Fatalf("ReconcilePending() error = %v", err)
	}
	if result.Scanned != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("ReconcilePending() result = %+v", result)
	}
	if repo.requestedLimit != 25 || repo.updates != 1 {
		t.Fatalf("repository limit/updates = %d/%d", repo.requestedLimit, repo.updates)
	}
	if op.AuthzProjectionPending() || op.AuthzPolicyVersion() != 12 {
		t.Fatalf("projection pending/version = %v/%d", op.AuthzProjectionPending(), op.AuthzPolicyVersion())
	}
}

func TestRoleProjectionReconcilerPreservesPendingOnIAMFailure(t *testing.T) {
	t.Parallel()

	op := domain.NewOperator(1, 101, "operator")
	op.SetID(201)
	op.ReplaceRolesProjection(nil, nil, 10, nil, true)
	repo := &pendingProjectionRepoStub{operators: []*domain.Operator{op}}
	reconciler := NewRoleProjectionReconciler(repo, &projectionGatewayStub{err: errors.New("IAM unavailable")})

	result, err := reconciler.ReconcilePending(context.Background(), 100)

	if err == nil {
		t.Fatal("ReconcilePending() error = nil")
	}
	if result.Scanned != 1 || result.Succeeded != 0 || result.Failed != 1 {
		t.Fatalf("ReconcilePending() result = %+v", result)
	}
	if !op.AuthzProjectionPending() || repo.updates != 0 {
		t.Fatalf("failed projection was mutated: pending=%v updates=%d", op.AuthzProjectionPending(), repo.updates)
	}
}

type pendingProjectionRepoStub struct {
	operators      []*domain.Operator
	requestedLimit int
	updates        int
}

func (s *pendingProjectionRepoStub) ListAuthzProjectionPending(_ context.Context, limit int) ([]*domain.Operator, error) {
	s.requestedLimit = limit
	return append([]*domain.Operator(nil), s.operators...), nil
}
func (*pendingProjectionRepoStub) Save(context.Context, *domain.Operator) error { return nil }
func (s *pendingProjectionRepoStub) Update(context.Context, *domain.Operator) error {
	s.updates++
	return nil
}
func (*pendingProjectionRepoStub) FindByID(context.Context, domain.ID) (*domain.Operator, error) {
	return nil, errors.New("not implemented")
}
func (*pendingProjectionRepoStub) FindByUser(context.Context, int64, int64) (*domain.Operator, error) {
	return nil, errors.New("not implemented")
}
func (*pendingProjectionRepoStub) Delete(context.Context, domain.ID) error { return nil }

type projectionGatewayStub struct {
	projection iambridge.OperatorRoleProjection
	err        error
}

func (*projectionGatewayStub) IsEnabled() bool { return true }
func (*projectionGatewayStub) ReplaceManagedOperatorRoles(context.Context, int64, int64, []string, string, string) (int64, error) {
	return 0, nil
}
func (s *projectionGatewayStub) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	return s.projection, s.err
}
