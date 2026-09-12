package operatorretirement

import (
	"context"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMySQLRecoveryArchivesExitAndRollsBackEveryLocalChange(t *testing.T) {
	db := isolatedMySQL(t)
	exec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	migration := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("../../../../../pkg/migration/migrations/mysql", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	exec("CREATE TABLE operators (id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT, user_id BIGINT, version INT UNSIGNED, is_active BOOLEAN, deleted_at DATETIME(3), updated_at DATETIME(3), updated_by BIGINT, roles JSON, effective_roles JSON, authz_policy_version BIGINT, authz_projected_at DATETIME(3), authz_projection_pending BOOLEAN)")
	exec(migration("000074_operator_retirement_tasks.up.sql"))
	exec(migration("000079_operator_recovery_archives.up.sql"))
	// Empty archive rollback is executable via the same MySQL multi-statement protocol as migrations.
	exec(migration("000079_operator_recovery_archives.down.sql"))
	exec(migration("000079_operator_recovery_archives.up.sql"))
	exec("INSERT INTO operators(id,org_id,user_id,version,is_active,roles,effective_roles) VALUES(7,1,8,1,TRUE,JSON_ARRAY('qs:result_reviewer'),JSON_ARRAY('qs:result_reviewer')),(9,1,9,1,TRUE,JSON_ARRAY('qs:admin'),JSON_ARRAY('qs:admin'))")
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	task, err := domain.New(7, 1, 8, 9, 1, "exit", "retire", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Begin(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err = task.MarkRevoked(10, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err = task.Complete(10, false, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Finish(ctx, task); err != nil {
		t.Fatal(err)
	}
	var original taskPO
	if err = db.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	recovery, err := domain.NewRecovery(task, 9, 3, 10, "recover", "HQ identity explicitly requested", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Recover(ctx, recovery); err == nil {
		t.Fatal("recovery without user lock accepted")
	}
	exec("CREATE TRIGGER refuse_archive_completion BEFORE DELETE ON operator_retirement_tasks FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected recovery failure'")
	if err = repo.WithOperatorLock(ctx, 1, 7, func(c context.Context) error { return repo.Recover(c, recovery) }); err == nil {
		t.Fatal("injected failure ignored")
	}
	var count int64
	if err = db.Model(&recoveryPO{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial archive", count, err)
	}
	if err = db.Table("operators").Where("id=7 AND is_active=FALSE AND deleted_at IS NOT NULL AND version=3").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("partial restore", count, err)
	}
	if err = repo.WithinMutation(ctx, 8, func(context.Context) error { return nil }); err == nil {
		t.Fatal("failed recovery removed exit block")
	}
	exec("DROP TRIGGER refuse_archive_completion")
	if err = repo.WithOperatorLock(ctx, 1, 7, func(c context.Context) error { return repo.Recover(c, recovery) }); err != nil {
		t.Fatal(err)
	}
	restored, err := repo.Find(ctx, 1, 7)
	if err != nil || restored.Version != 4 {
		t.Fatal(restored, err)
	}
	if err = db.Table("operators").Where("id=7 AND JSON_LENGTH(roles)=0 AND JSON_LENGTH(effective_roles)=0 AND authz_policy_version=10").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("roles restored", count, err)
	}
	receipt, err := repo.FindRecovery(ctx, "recover")
	if err != nil || receipt == nil || *receipt != recovery {
		t.Fatal(receipt, err)
	}
	var archive recoveryPO
	if err = db.First(&archive).Error; err != nil {
		t.Fatal(err)
	}
	if archive.RetirementPayload != original.Payload || archive.RetirementBeforeState != original.BeforeState {
		t.Fatal("historical exit rewritten")
	}
	if err = repo.WithinMutation(ctx, 8, func(context.Context) error { return nil }); err != nil {
		t.Fatal("recovered identity still blocked", err)
	}
	if err = db.Exec(migration("000079_operator_recovery_archives.down.sql")).Error; err == nil {
		t.Fatal("populated recovery archive silently deleted")
	}
	// A later explicit retirement starts a new lifecycle; the old exit remains in its immutable archive.
	next, err := domain.New(7, 1, 8, 9, 4, "second-exit", "retire again", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Begin(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err = repo.WithinMutation(ctx, 8, func(context.Context) error { return nil }); err == nil {
		t.Fatal("new exit not enforced")
	}
	exec("UPDATE operator_recovery_archives SET retirement_before_state='{}'")
	if _, err = repo.FindRecovery(ctx, "recover"); err == nil {
		t.Fatal("corrupt history accepted")
	}
}

func TestMySQLRecoveryPreflightBeforeArchiveMigrationOnlyAcceptsKnownCleanSchema(t *testing.T) {
	db := isolatedMySQL(t)
	if err := db.Exec("CREATE TABLE schema_migrations(version BIGINT UNSIGNED PRIMARY KEY,dirty BOOLEAN); INSERT INTO schema_migrations VALUES(78,FALSE)").Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	if value, err := repo.FindRecovery(context.Background(), "not-yet-applied"); err != nil || value != nil {
		t.Fatal("pre-migration read failed", err)
	}
	if err := db.Exec("UPDATE schema_migrations SET dirty=TRUE").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindRecovery(context.Background(), "missing"); err == nil {
		t.Fatal("dirty schema accepted")
	}
	if err := db.Exec("UPDATE schema_migrations SET version=79,dirty=FALSE").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindRecovery(context.Background(), "missing"); err == nil {
		t.Fatal("missing required archive treated as no history")
	}
}
