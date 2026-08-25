package operator

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
)

func TestAssignRoleFailsClosedWithoutIAMGateway(t *testing.T) {
	repo := newFakeOperatorRepo()
	op := domain.NewOperator(1, 10001, "operator")
	op.SetID(20001)
	if err := repo.Save(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	validator := domain.NewValidator()
	service := NewAuthorizationService(
		repo,
		validator,
		domain.NewLifecycler(),
		nil,
		nil,
	)

	err := service.AssignRole(context.Background(), uint64(op.ID()), string(domain.RoleEvaluatorQS))

	if err == nil {
		t.Fatal("AssignRole() error = nil, want IAM dependency error")
	}
	if repo.updates != 0 {
		t.Fatalf("local repository updates = %d, want 0", repo.updates)
	}
	if len(op.Roles()) != 0 {
		t.Fatalf("local roles = %v, want no local role fact write", op.Roles())
	}
}
