package operator

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	storeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/stretchr/testify/require"
	"testing"
)

type scopeRepoStub struct {
	domain.Repository
	target *domain.Operator
}

func (r scopeRepoStub) FindByUser(context.Context, int64, int64) (*domain.Operator, error) {
	return domain.NewOperator(1, 9, "HQ"), nil
}
func (r scopeRepoStub) FindByID(context.Context, domain.ID) (*domain.Operator, error) {
	return r.target, nil
}

type scopeGatewayStub struct {
	iambridge.OperatorScopeGateway
	calls int
}

func (g *scopeGatewayStub) LoadOperatorAssignmentFacts(context.Context, int64) (iambridge.OperatorAssignmentFacts, error) {
	g.calls++
	return iambridge.OperatorAssignmentFacts{PolicyVersion: 8, Assignments: []iambridge.OperatorAssignmentFact{
		{AssignmentID: "1", ManagementProtection: "standard", Scope: &iambridge.OperatorAssignmentScope{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}},
		{AssignmentID: "2", ManagementProtection: "protected", Scope: &iambridge.OperatorAssignmentScope{OrgID: 2, Kind: "all_stores"}},
		{AssignmentID: "3", ManagementProtection: "standard"},
	}}, nil
}
func TestScopeConfigurationFiltersCompanyButKeepsProtection(t *testing.T) {
	gateway := &scopeGatewayStub{}
	repo := scopeRepoStub{target: domain.NewOperator(1, 10, "Target")}
	svc := NewScopeService(repo, gateway)
	ctx := actorctx.WithOperatorOrgID(actorctx.WithGrantingUserID(context.Background(), 9), 1)
	ctx = authz.WithSnapshot(ctx, &authz.Snapshot{AuthzVersion: 8, ScopeContractVersion: 1, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 1, Kind: "all_stores"}}}}})
	result, err := svc.Get(ctx, 10)
	require.NoError(t, err)
	require.Len(t, result.Assignments, 1)
	require.Len(t, result.Unconfigured, 1)
	require.True(t, result.ProtectedAccess)
	repo.target = domain.NewOperator(2, 10, "Foreign")
	svc = NewScopeService(repo, gateway)
	_, err = svc.Get(ctx, 10)
	require.Error(t, err)
	require.Equal(t, 1, gateway.calls)
	_, err = svc.Get(context.Background(), 10)
	require.Error(t, err)
	require.Equal(t, 1, gateway.calls)
}

type scopeStoreStub struct {
	org    int64
	active bool
}

func (s scopeStoreStub) Get(context.Context, int64, uint64) (*storeDomain.Store, error) {
	return storeDomain.Restore(storeDomain.State{ID: 7, OrgID: s.org, IsActive: s.active}), nil
}
func TestScopeTargetsRequireActiveCompanyStoreAndBusinessRole(t *testing.T) {
	role := iambridge.OperatorScopedRole{RoleName: string(domain.RoleAssessmentOperator), Scope: iambridge.OperatorAssignmentScope{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}
	svc := &ScopeService{writes: ScopeWriteDependencies{Stores: scopeStoreStub{org: 1, active: true}}}
	require.NoError(t, svc.validateTargets(context.Background(), 1, []iambridge.OperatorScopedRole{role}))
	for _, store := range []scopeStoreStub{{org: 1}, {org: 2, active: true}} {
		svc.writes.Stores = store
		require.Error(t, svc.validateTargets(context.Background(), 1, []iambridge.OperatorScopedRole{role}))
	}
	svc.writes.Stores = scopeStoreStub{org: 1, active: true}
	role.RoleName = string(domain.RoleQSAdmin)
	require.Error(t, svc.validateTargets(context.Background(), 1, []iambridge.OperatorScopedRole{role}))
	role.RoleName = string(domain.RoleAssessmentOperator)
	role.Scope.OrgID = 2
	require.Error(t, svc.validateTargets(context.Background(), 1, []iambridge.OperatorScopedRole{role}))
}
