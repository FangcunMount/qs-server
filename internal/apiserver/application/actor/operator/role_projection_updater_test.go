package operator

import (
	"context"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"testing"
)

func TestRequestRoleProjectionUsesCompanyFactsNotFlattenedRoles(t *testing.T) {
	for _, byUser := range []bool{false, true} {
		repo := newFakeOperatorRepo()
		op := domain.NewOperator(1, 101, "test")
		if err := repo.Save(context.Background(), op); err != nil {
			t.Fatal(err)
		}
		gateway := &projectionGatewayStub{projection: iambridge.OperatorRoleProjection{DirectRoles: []string{"qs:assessment_operator"}, PolicyVersion: 12}}
		updater := NewRoleProjectionUpdater(repo, gateway)
		// These names may belong to other companies or unconfigured assignments.
		snapshot := &authzapp.Snapshot{AuthzVersion: 12, DirectRoles: []string{"qs:admin", "qs:result_reviewer"}}
		var err error
		if byUser {
			err = updater.PersistFromSnapshotByUser(context.Background(), 1, 101, snapshot)
		} else {
			err = updater.PersistFromSnapshot(context.Background(), toOperatorResult(op), snapshot)
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(op.Roles()) != 1 || op.Roles()[0] != domain.RoleAssessmentOperator {
			t.Fatalf("unscoped roles projected: %v", op.Roles())
		}
	}
}

func TestRequestProjectionPreservesPendingOnMissingOrStaleFacts(t *testing.T) {
	for _, gatewayVersion := range []int64{0, 9, 11} {
		repo := newFakeOperatorRepo()
		op := domain.NewOperator(1, 101, "test")
		op.ReplaceRolesProjection([]domain.Role{domain.RoleAssessmentOperator}, []domain.Role{domain.RoleAssessmentOperator}, 10, nil, true)
		if err := repo.Save(context.Background(), op); err != nil {
			t.Fatal(err)
		}
		gateway := &projectionGatewayStub{projection: iambridge.OperatorRoleProjection{DirectRoles: []string{"qs:admin"}, PolicyVersion: gatewayVersion}}
		updater := NewRoleProjectionUpdater(repo, gateway)
		err := updater.PersistFromSnapshotByUser(context.Background(), 1, 101, &authzapp.Snapshot{AuthzVersion: 12})
		if err == nil || repo.updates != 0 || !op.AuthzProjectionPending() || op.AuthzPolicyVersion() != 10 {
			t.Fatalf("stale version %d changed projection: error=%v", gatewayVersion, err)
		}
	}
}

func TestProjectionCannotRegressPersistedPolicyVersion(t *testing.T) {
	repo := newFakeOperatorRepo()
	op := domain.NewOperator(1, 101, "test")
	op.ReplaceRolesProjection(nil, nil, 20, nil, true)
	err := persistOperatorRoleProjection(context.Background(), repo, op, iambridge.OperatorRoleProjection{PolicyVersion: 19}, false)
	if err == nil || repo.updates != 0 || op.AuthzPolicyVersion() != 20 || !op.AuthzProjectionPending() {
		t.Fatal("older projection replaced current state")
	}
}
