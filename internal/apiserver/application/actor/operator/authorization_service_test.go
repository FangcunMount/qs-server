package operator

import (
	"context"
	"reflect"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

func TestReplaceRolesUsesDirectRolesAndPersistsEffectiveProjection(t *testing.T) {
	repo := newFakeOperatorRepo()
	op := domain.NewOperator(1, 10001, "operator")
	op.SetID(20001)
	op.ReplaceRolesProjection(
		[]domain.Role{domain.RoleEvaluatorQS},
		[]domain.Role{domain.RoleEvaluatorQS, domain.RoleOperator},
		10, nil, false,
	)
	if err := repo.Save(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	gateway := &operatorAuthzGatewayFake{
		committedVersion: 12,
		projection: iambridge.OperatorRoleProjection{
			DirectRoles:    []string{string(domain.RoleEvaluationPlanManager)},
			EffectiveRoles: []string{string(domain.RoleEvaluationPlanManager), string(domain.RoleOperator)},
			PolicyVersion:  12,
		},
	}
	service := NewAuthorizationService(repo, domain.NewValidator(), domain.NewLifecycler(), nil, gateway)

	if err := service.ReplaceRoles(context.Background(), uint64(op.ID()), []string{string(domain.RoleEvaluationPlanManager)}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gateway.replacedRoles, []string{string(domain.RoleEvaluationPlanManager)}) {
		t.Fatalf("replaced roles = %v", gateway.replacedRoles)
	}
	if !reflect.DeepEqual(op.Roles(), []domain.Role{domain.RoleEvaluationPlanManager}) {
		t.Fatalf("direct roles = %v", op.Roles())
	}
	if !reflect.DeepEqual(op.EffectiveRoles(), []domain.Role{domain.RoleEvaluationPlanManager, domain.RoleOperator}) {
		t.Fatalf("effective roles = %v", op.EffectiveRoles())
	}
	if op.AuthzPolicyVersion() != 12 || op.AuthzProjectionPending() {
		t.Fatalf("projection version/pending = %d/%v", op.AuthzPolicyVersion(), op.AuthzProjectionPending())
	}
}

func TestReplaceRolesReturnsSuccessAndMarksPendingWhenSnapshotLags(t *testing.T) {
	repo := newFakeOperatorRepo()
	op := domain.NewOperator(1, 10001, "operator")
	op.SetID(20001)
	op.ReplaceRolesProjection([]domain.Role{domain.RoleEvaluatorQS}, []domain.Role{domain.RoleEvaluatorQS, domain.RoleOperator}, 10, nil, false)
	if err := repo.Save(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	gateway := &operatorAuthzGatewayFake{
		committedVersion: 12,
		projection:       iambridge.OperatorRoleProjection{PolicyVersion: 11},
	}
	service := NewAuthorizationService(repo, domain.NewValidator(), domain.NewLifecycler(), nil, gateway)

	if err := service.ReplaceRoles(context.Background(), uint64(op.ID()), []string{string(domain.RoleEvaluationPlanManager)}); err != nil {
		t.Fatalf("ReplaceRoles() error = %v, want committed success", err)
	}
	if !op.AuthzProjectionPending() || op.AuthzPolicyVersion() != 10 {
		t.Fatalf("projection version/pending = %d/%v, want 10/true", op.AuthzPolicyVersion(), op.AuthzProjectionPending())
	}
	if !reflect.DeepEqual(op.Roles(), []domain.Role{domain.RoleEvaluatorQS}) {
		t.Fatalf("lagging snapshot overwrote direct role evidence: %v", op.Roles())
	}
}

type operatorAuthzGatewayFake struct {
	replacedRoles    []string
	committedVersion int64
	projection       iambridge.OperatorRoleProjection
}

func (*operatorAuthzGatewayFake) IsEnabled() bool { return true }
func (f *operatorAuthzGatewayFake) ReplaceManagedOperatorRoles(_ context.Context, _, _ int64, roles []string, _, _ string) (int64, error) {
	f.replacedRoles = append([]string(nil), roles...)
	return f.committedVersion, nil
}
func (f *operatorAuthzGatewayFake) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	return f.projection, nil
}
