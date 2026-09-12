package operator

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/stretchr/testify/require"
	"testing"
)

type writeScopeRepo struct {
	scopeRepoStub
	writes int
	err    error
}

func (r *writeScopeRepo) Update(context.Context, *domain.Operator) error { r.writes++; return r.err }

type writeScopeGateway struct {
	iambridge.OperatorScopeGateway
	facts         iambridge.OperatorAssignmentFacts
	reads, writes int
	version       int64
	err           error
	repo          *writeScopeRepo
}

func (g *writeScopeGateway) LoadOperatorAssignmentFacts(context.Context, int64) (iambridge.OperatorAssignmentFacts, error) {
	g.reads++
	return g.facts, nil
}
func (g *writeScopeGateway) ReplaceOperatorScopedRoles(_ context.Context, org, user int64, _ []iambridge.OperatorScopedRole, version int64, actor, reason string) (int64, error) {
	if org != 1 || user != 10 || actor != "user:9" || reason != "test" || g.repo.writes != 1 {
		panic("incorrect mutation boundary")
	}
	g.writes++
	g.version = version
	return 0, g.err
}

type scopeGate struct {
	err   error
	calls int
}

func (g *scopeGate) WithinMutation(ctx context.Context, _ int64, fn func(context.Context) error) error {
	g.calls++
	if g.err != nil {
		return g.err
	}
	return fn(ctx)
}
func TestScopeReplaceFailureBoundaries(t *testing.T) {
	for _, name := range []string{"retiring", "stale", "protected", "legacy", "local_failure", "remote_failure"} {
		t.Run(name, func(t *testing.T) {
			repo := &writeScopeRepo{scopeRepoStub: scopeRepoStub{target: domain.NewOperator(1, 10, "Target")}}
			gateway := &writeScopeGateway{repo: repo, facts: iambridge.OperatorAssignmentFacts{PolicyVersion: 8}}
			gate := &scopeGate{}
			input := UpdateScopeConfiguration{OperatorID: 10, ExpectedPolicyVersion: 8, Reason: "test", Roles: []iambridge.OperatorScopedRole{{RoleName: string(domain.RoleAssessmentOperator), Scope: iambridge.OperatorAssignmentScope{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}}}
			switch name {
			case "retiring":
				gate.err = errors.New("retired")
			case "stale":
				input.ExpectedPolicyVersion = 7
			case "protected":
				gateway.facts.Assignments = []iambridge.OperatorAssignmentFact{{ManagementProtection: "protected"}}
			case "legacy":
				gateway.facts.Assignments = []iambridge.OperatorAssignmentFact{{RoleName: string(domain.RoleAssessmentOperator), ManagementProtection: "standard"}}
			case "local_failure":
				repo.err = errors.New("local persistence unavailable")
			case "remote_failure":
				gateway.err = errors.New("remote outcome unknown")
			}
			svc := NewScopeService(repo, gateway, ScopeWriteDependencies{Stores: scopeStoreStub{org: 1, active: true}, Gate: gate})
			ctx := actorctx.WithOperatorOrgID(actorctx.WithGrantingUserID(context.Background(), 9), 1)
			ctx = authz.WithSnapshot(ctx, &authz.Snapshot{AuthzVersion: 8, ScopeContractVersion: 1, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 1, Kind: "all_stores"}}}}})
			_, err := svc.Replace(ctx, input)
			require.Error(t, err)
			if name == "remote_failure" {
				require.Equal(t, 1, gateway.writes)
				require.EqualValues(t, 8, gateway.version)
				require.Equal(t, 1, repo.writes)
			} else {
				require.Zero(t, gateway.writes)
			}
			if name != "remote_failure" && name != "local_failure" {
				require.Zero(t, repo.writes)
			}
			if name == "retiring" {
				require.Zero(t, gateway.reads)
			}
		})
	}
}
