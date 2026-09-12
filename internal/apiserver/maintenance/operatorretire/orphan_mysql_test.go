package operatorretire

import (
	"context"
	"testing"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
)

func TestMySQLOrphanRevocationGuardsAndReplay(t *testing.T) {
	db, _, facts := fixture(t)
	require.NoError(t, db.Exec("RENAME TABLE staff TO operators").Error)
	tool, err := New(db, facts, facts, "final")
	require.NoError(t, err)
	cmd := OrphanCommand{OrgID: 1, ActorID: 9, UserID: 800038, RequestID: "orphan-1", Reason: "reviewed access removal"}
	ctx := context.Background()
	plan, err := tool.OrphanPreflight(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, "pending", plan.State)
	_, err = tool.OrphanApply(ctx, cmd, plan.Fingerprint, false)
	require.Error(t, err)
	_, err = tool.OrphanApply(ctx, cmd, "wrong", true)
	require.Error(t, err)
	facts.version++
	_, err = tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.Error(t, err)
	plan, err = tool.OrphanPreflight(ctx, cmd)
	require.NoError(t, err)
	// A membership in another company, even soft-deleted, forbids global replacement.
	require.NoError(t, db.Exec("UPDATE operators SET user_id=?,org_id=2,deleted_at=NOW() WHERE id=7", cmd.UserID).Error)
	_, err = tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.Error(t, err)
	require.NoError(t, db.Exec("UPDATE operators SET user_id=8,org_id=1,deleted_at=NULL WHERE id=7").Error)
	saved := append([]authz.AssignmentRoleFact{}, facts.facts...)
	facts.facts = append(facts.facts, authz.AssignmentRoleFact{RoleID: "unexpected", RoleName: "qs:content_manager", ManagementProtection: "standard"})
	_, err = tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.Error(t, err)
	facts.facts = saved
	facts.fail = true
	_, err = tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.Error(t, err)
	require.Zero(t, facts.writes)
	facts.fail = false
	done, err := tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.NoError(t, err)
	require.Equal(t, "verified", done.State)
	require.Greater(t, done.SubmittedPolicyVersion, plan.PolicyVersion)
	require.Equal(t, retained(saved), facts.facts)
	replay, err := tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.NoError(t, err)
	require.Equal(t, "already_unprivileged", replay.State)
	require.Equal(t, 1, facts.writes)
	// Restoring the same role names later still cannot replay the old fingerprint.
	facts.facts = saved
	facts.version++
	_, err = tool.OrphanApply(ctx, cmd, plan.Fingerprint, true)
	require.Error(t, err)
	require.Equal(t, 1, facts.writes)
	var count int64
	require.NoError(t, db.Table("operators").Where("user_id=?", cmd.UserID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Table("operator_retirement_tasks").Count(&count).Error)
	require.Zero(t, count)
}
