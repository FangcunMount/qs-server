package operatorretirement

import (
	"context"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"testing"
	"time"
)

type recoveryMemory struct {
	*memoryRepo
	receipt *domain.Recovery
	writes  int
}

func (r *recoveryMemory) FindRecovery(context.Context, string) (*domain.Recovery, error) {
	return r.receipt, nil
}
func (r *recoveryMemory) Recover(_ context.Context, v domain.Recovery) error {
	r.receipt = &v
	r.writes++
	return nil
}
func recoveryFixture() (*Service, *recoveryMemory, *gateway, context.Context, RecoveryCommand) {
	_, base, g, ctx, cmd := fixture()
	task, _ := domain.New(7, 1, 8, 9, 3, "exit", "retire", time.Now())
	_ = task.MarkRevoked(10, time.Now())
	_ = task.Complete(10, false, time.Now())
	base.task = &task
	r := &recoveryMemory{memoryRepo: base}
	g.roles = []string{"user"}
	g.version = 10
	cmd.ExpectedVersion = 5
	cmd.RequestID = "recovery"
	cmd.Reason = "restore headquarters test identity"
	return NewMaintenanceService(r, g), r, g, ctx, RecoveryCommand{Command: cmd, ExpectedPolicyVersion: 10, WritesStopped: true}
}
func TestRecoveryPreservesSelfServiceAndNeverWritesIAM(t *testing.T) {
	s, r, g, ctx, cmd := recoveryFixture()
	if _, err := s.RecoverIdentity(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecoverIdentity(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if r.writes != 1 || g.calls != 0 {
		t.Fatal("duplicate recovery or IAM mutation")
	}
	cmd.Reason = "different request"
	if _, err := s.RecoverIdentity(ctx, cmd); err == nil {
		t.Fatal("conflicting receipt accepted")
	}
}
func TestRecoveryFailsClosedBeforeLocalWrite(t *testing.T) {
	for _, name := range []string{"no administrator", "writes active", "backend role", "protected", "policy drift", "other company", "not completed"} {
		t.Run(name, func(t *testing.T) {
			s, r, g, ctx, cmd := recoveryFixture()
			switch name {
			case "no administrator":
				ctx = context.Background()
			case "writes active":
				cmd.WritesStopped = false
			case "backend role":
				g.roles = []string{"qs:assessment_operator"}
			case "protected":
				g.protected = true
			case "policy drift":
				g.version++
			case "other company":
				r.others = 1
			case "not completed":
				r.task.Stage = domain.Revoked
			}
			if _, err := s.RecoverIdentity(ctx, cmd); err == nil || r.writes != 0 || g.calls != 0 {
				t.Fatal("unsafe recovery", err)
			}
		})
	}
}

// The second IAM read must reject a write that occurred after the initial recovery checks.
type driftingRecoveryGateway struct {
	*gateway
	reads int
}

func (g *driftingRecoveryGateway) LoadOperatorRoleProjection(ctx context.Context, org, user int64) (iambridge.OperatorRoleProjection, error) {
	g.reads++
	p, err := g.gateway.LoadOperatorRoleProjection(ctx, org, user)
	if g.reads > 1 {
		p.PolicyVersion++
	}
	return p, err
}
func TestRecoveryRejectsAuthorizationDriftBeforeLocalCommit(t *testing.T) {
	_, r, g, ctx, cmd := recoveryFixture()
	drift := &driftingRecoveryGateway{gateway: g}
	s := NewMaintenanceService(r, drift)
	if _, err := s.RecoverIdentity(ctx, cmd); err == nil || r.writes != 0 || drift.reads != 2 {
		t.Fatal("authorization drift allowed recovery", err)
	}
}

func TestRecoveryPreviewDoesNotWriteOrRequirePausedWindow(t *testing.T) {
	s, r, g, ctx, cmd := recoveryFixture()
	cmd.WritesStopped = false
	value, err := s.PreviewIdentityRecovery(ctx, cmd)
	if err != nil || value == nil || r.writes != 0 || g.calls != 0 {
		t.Fatal("preview mutated identity", err)
	}
	if _, err = s.RecoverIdentity(ctx, cmd); err == nil {
		t.Fatal("apply skipped maintenance gate")
	}
}
