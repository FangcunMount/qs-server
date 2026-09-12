package operatorretire

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"testing"
)

func TestMySQLSelectedExitPreflightResumeVerifyAndHistoricalManifestIsolation(t *testing.T) {
	db, _, facts := fixture(t)
	if err := db.Exec("RENAME TABLE staff TO operators; UPDATE operators SET user_id=800018 WHERE id=7; UPDATE operators SET user_id=900019 WHERE id=9").Error; err != nil {
		t.Fatal(err)
	}
	tool, err := New(db, facts, facts, "final")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cmd := app.Command{OrgID: 1, ActorID: 900019, OperatorID: 7, ExpectedVersion: 3, RequestID: "selected-exit", Reason: "explicitly reviewed personnel exit"}
	before, err := tool.SelectedPreflight(ctx, cmd)
	if err != nil || before.State != "pending" || before.Fingerprint == "" {
		t.Fatal(before, err)
	}
	again, err := tool.SelectedPreflight(ctx, cmd)
	if err != nil || again.Fingerprint != before.Fingerprint {
		t.Fatal("unstable preview", err)
	}
	if _, err = tool.SelectedApply(ctx, cmd, before.Fingerprint, false); err == nil {
		t.Fatal("maintenance gate skipped")
	}
	if _, err = tool.SelectedApply(ctx, cmd, "wrong", true); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	var count int64
	if err = db.Table("operator_retirement_tasks").Count(&count).Error; err != nil || count != 0 || facts.writes != 0 {
		t.Fatal("preview or rejected apply wrote facts", count, err)
	}
	facts.fail = true
	if _, err = tool.SelectedApply(ctx, cmd, before.Fingerprint, true); err == nil {
		t.Fatal("IAM failure ignored")
	}
	if err = db.Table("operators").Where("id=7 AND is_active=FALSE AND deleted_at IS NULL").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("failed revoke left admission open", count, err)
	}
	facts.fail = false
	resumed, err := tool.SelectedPreflight(ctx, cmd)
	if err != nil || resumed.State != "disabled" {
		t.Fatal(resumed, err)
	}
	if _, err = tool.SelectedApply(ctx, cmd, resumed.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	done, err := tool.SelectedVerify(ctx, cmd)
	if err != nil || done.State != "verified" {
		t.Fatal(done, err)
	}
	if _, err = tool.SelectedApply(ctx, cmd, resumed.Fingerprint, true); err != nil || facts.writes != 1 {
		t.Fatal("replay performed revocation", facts.writes, err)
	}
	if err = db.Table("operator_retirement_manifests").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("historical doctor manifest modified", count, err)
	}
	facts.facts = append(facts.facts, authz.AssignmentRoleFact{RoleID: "new", RoleName: "qs:result_reviewer", ManagementProtection: "standard"})
	if _, err = tool.SelectedVerify(ctx, cmd); err == nil {
		t.Fatal("historical completion concealed current backend grant")
	}
}
