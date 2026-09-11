package aibridge

import (
	"context"
	"errors"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"testing"
)

type managementStub struct {
	calls int
	scope EvaluationScope
}

func (g *managementStub) GetEvaluation(_ context.Context, scope EvaluationScope) (EvaluationState, error) {
	g.calls++
	g.scope = scope
	return EvaluationState{RunID: scope.RunID}, nil
}
func (g *managementStub) ResolveUnknown(ctx context.Context, scope EvaluationScope, _ UnknownResolution) (EvaluationState, error) {
	return g.GetEvaluation(ctx, scope)
}
func adminContext() context.Context {
	return authz.WithSnapshot(context.Background(), &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}})
}
func managementScope() EvaluationScope {
	return EvaluationScope{RunID: "00000000-0000-4000-8000-000000000001", OrganizationID: 7, OperatorUserID: 42}
}
func resolutionCommand() UnknownResolution {
	return UnknownResolution{ExpectedVersion: 6, ExecutionID: "execution:1", Decision: "authorize_replacement", Reason: "人工核对", Confirm: true, AcknowledgedDuplicateCallAndCostRisk: true}
}
func TestEvaluationManagementRequiresCurrentOrgAdminForEveryOperation(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	scope := managementScope()
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}})} {
		if _, err := service.Get(ctx, scope); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatalf("get: %v", err)
		}
		if _, err := service.Resolve(ctx, scope, resolutionCommand()); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatalf("resolve: %v", err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("denied operation reached AI")
	}
	if _, err := service.Resolve(adminContext(), scope, resolutionCommand()); err != nil {
		t.Fatal(err)
	}
	if gateway.calls != 1 || gateway.scope != scope {
		t.Fatal("authorized scope not forwarded")
	}
	// A subsequent request with revoked permissions must not reuse the earlier decision.
	revoked := authz.WithSnapshot(context.Background(), &authz.Snapshot{})
	if _, err := service.Resolve(revoked, scope, resolutionCommand()); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if gateway.calls != 1 {
		t.Fatal("revoked request reached AI")
	}
}
func TestEvaluationManagementRejectsUnconfirmedAndMissingIdentity(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	for _, change := range []func(*UnknownResolution){func(c *UnknownResolution) { c.Confirm = false }, func(c *UnknownResolution) { c.AcknowledgedDuplicateCallAndCostRisk = false }, func(c *UnknownResolution) { c.ExpectedVersion = 0 }, func(c *UnknownResolution) { c.Decision = "retry" }} {
		command := resolutionCommand()
		change(&command)
		if _, err := service.Resolve(adminContext(), managementScope(), command); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	scope := managementScope()
	scope.OperatorUserID = 0
	if _, err := service.Get(adminContext(), scope); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if gateway.calls != 0 {
		t.Fatal("invalid command reached AI")
	}
}

func (g *managementStub) StartEvaluation(ctx context.Context, s EvaluationScope, _ EvaluationStart) (EvaluationState, error) {
	return g.GetEvaluation(ctx, s)
}

func TestStartRequiresCurrentPermissionAndExplicitValidCommand(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	command := EvaluationStart{ExpectedVersion: 1, Reason: "启动评测", Confirm: true}
	for _, change := range []func(*EvaluationStart){func(c *EvaluationStart) { c.Confirm = false }, func(c *EvaluationStart) { c.ExpectedVersion = 0 }, func(c *EvaluationStart) { c.Reason = " " }, func(c *EvaluationStart) { c.Reason = "<invalid>" }} {
		invalid := command
		change(&invalid)
		if _, err := service.Start(adminContext(), managementScope(), invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("invalid start forwarded")
	}
	if _, err := service.Start(adminContext(), managementScope(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(context.Background(), managementScope(), command); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if gateway.calls != 1 || gateway.scope != managementScope() {
		t.Fatal("revoked start forwarded or scope drift")
	}
}
