package operatorretirement

import (
	"context"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"
	"testing"
)

type memoryRepo struct {
	task                          *domain.Task
	disabled, deleted, failFinish bool
	others                        int64
}

func (r *memoryRepo) WithOperatorLock(ctx context.Context, _ int64, _ uint64, fn func(context.Context) error) error {
	return fn(ctx)
}
func (r *memoryRepo) Find(context.Context, int64, uint64) (port.Target, error) {
	return port.Target{ID: 7, OrgID: 1, UserID: 8, Version: 3}, nil
}
func (r *memoryRepo) FindTask(context.Context, int64, uint64) (*domain.Task, error) {
	if r.task == nil {
		return nil, nil
	}
	v := *r.task
	return &v, nil
}
func (r *memoryRepo) OtherMemberships(context.Context, int64, uint64) (int64, error) {
	return r.others, nil
}
func (r *memoryRepo) Begin(_ context.Context, t domain.Task) error {
	r.disabled = true
	r.task = &t
	return nil
}
func (r *memoryRepo) Save(_ context.Context, t domain.Task) error { r.task = &t; return nil }
func (r *memoryRepo) Finish(_ context.Context, t domain.Task) error {
	if r.failFinish {
		return fmt.Errorf("database unavailable")
	}
	r.deleted = true
	r.task = &t
	return nil
}

type gateway struct {
	protected bool
	r         *memoryRepo
	roles     []string
	version   int64
	calls     int
	fail      bool
}

func (g *gateway) IsEnabled() bool { return true }
func (g *gateway) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	return iambridge.OperatorRoleProjection{ProtectedAccess: g.protected, DirectRoles: g.roles, EffectiveRoles: g.roles, PolicyVersion: g.version}, nil
}
func (g *gateway) ReplaceManagedOperatorRoles(context.Context, int64, int64, []string, string, string) (int64, error) {
	if !g.r.disabled {
		panic("revocation before local disable")
	}
	g.calls++
	if g.fail {
		return 0, fmt.Errorf("IAM unavailable")
	}
	g.roles = []string{"user"}
	g.version++
	return g.version, nil
}
func fixture() (*Service, *memoryRepo, *gateway, context.Context, Command) {
	r := &memoryRepo{}
	g := &gateway{r: r, roles: []string{"qs:result_reviewer"}, version: 10}
	ctx := authz.WithSnapshot(context.Background(), &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}})
	return NewService(r, g), r, g, ctx, Command{OrgID: 1, ActorID: 9, OperatorID: 7, ExpectedVersion: 3, RequestID: "exit-7", Reason: "doctor backend retirement"}
}
func TestRetirementResumesAfterIAMFailureAndCompletedReplayDoesNotWrite(t *testing.T) {
	s, r, g, ctx, cmd := fixture()
	g.fail = true
	if _, err := s.Execute(ctx, cmd); err == nil || !r.disabled || r.deleted || r.task.Stage != domain.Disabled {
		t.Fatal("unsafe failure state", err)
	}
	g.fail = false
	if task, err := s.Execute(ctx, cmd); err != nil || task.Stage != domain.Completed || !r.deleted {
		t.Fatal(task, err)
	}
	calls := g.calls
	if _, err := s.Execute(ctx, cmd); err != nil || g.calls != calls {
		t.Fatal("completed replay wrote IAM", err)
	}
}
func TestFinishFailureResumesWithoutRepeatedRevocation(t *testing.T) {
	s, r, g, ctx, cmd := fixture()
	r.failFinish = true
	task, err := s.Execute(ctx, cmd)
	if err == nil || task.Stage != domain.Revoked || r.deleted {
		t.Fatal("false completion", task, err)
	}
	r.failFinish = false
	if _, err = s.Execute(ctx, cmd); err != nil || g.calls != 1 || !r.deleted {
		t.Fatal("unsafe retry", err)
	}
}
func TestPreflightGuardsDoNotDisableOrRevoke(t *testing.T) {
	for _, scenario := range []string{"permission", "version", "cross-company", "admin", "unknown", "self"} {
		t.Run(scenario, func(t *testing.T) {
			s, r, g, ctx, cmd := fixture()
			switch scenario {
			case "permission":
				ctx = context.Background()
			case "version":
				cmd.ExpectedVersion = 4
			case "cross-company":
				r.others = 1
			case "admin":
				g.roles = []string{"qs:admin"}
			case "unknown":
				g.roles = []string{"unexpected"}
			case "self":
				cmd.ActorID = 8
			}
			if _, err := s.Execute(ctx, cmd); err == nil || r.disabled || g.calls != 0 {
				t.Fatal("guard did not fail closed", err)
			}
		})
	}
}
func TestRequestConflictCannotChangeExistingTask(t *testing.T) {
	s, r, g, ctx, cmd := fixture()
	g.fail = true
	_, _ = s.Execute(ctx, cmd)
	cmd.RequestID = "other"
	calls := g.calls
	if _, err := s.Execute(ctx, cmd); err == nil || g.calls != calls || r.task.RequestID != "exit-7" {
		t.Fatal("request conflict accepted", err)
	}
}

func TestGlobalAdministratorCannotBeRetiredWhenAppScopedRolesAreEmpty(t *testing.T) {
	s, r, g, ctx, cmd := fixture()
	g.roles = nil
	g.protected = true
	if _, err := s.Execute(ctx, cmd); err == nil {
		t.Fatal("protected administrator accepted")
	}
	if r.disabled || r.deleted || g.calls != 0 {
		t.Fatal("protected account modified")
	}
}
