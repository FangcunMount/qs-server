package operatorrecovery

import (
	"context"
	"fmt"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	driver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolatedMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_OPERATOR_RETIREMENT_REQUIRE_MYSQL") == "true" {
			t.Fatal("QS_SERVER_TEST_MYSQL_DSN required")
		}
		t.Skip("MySQL integration: set QS_SERVER_TEST_MYSQL_DSN")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.MultiStatements = true
	admin, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to isolated MySQL")
	}
	name := fmt.Sprintf("qs_operator_exit_test_%d", time.Now().UnixNano())
	if err = admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP DATABASE " + name).Error; sql, _ := admin.DB(); _ = sql.Close() })
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open test database")
	}
	t.Cleanup(func() { sql, _ := db.DB(); _ = sql.Close() })
	return db
}

type facts struct {
	writes  int
	version int64
}

func (f *facts) IsEnabled() bool { return true }
func (f *facts) LoadFresh(context.Context, string) (*authz.Snapshot, error) {
	return &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}, AuthzVersion: f.version}, nil
}
func (f *facts) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	return iambridge.OperatorRoleProjection{PolicyVersion: f.version, DirectRoles: []string{"user"}, EffectiveRoles: []string{"user"}}, nil
}
func (f *facts) ReplaceManagedOperatorRoles(context.Context, int64, int64, []string, string, string) (int64, error) {
	f.writes++
	return 0, fmt.Errorf("recovery must not assign roles")
}

func TestMySQLRecoveryToolPreflightDriftReplayAndReceiptIntegrity(t *testing.T) {
	db := isolatedMySQL(t)
	exec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE TABLE operators (id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT, user_id BIGINT, version INT UNSIGNED, is_active BOOLEAN, name VARCHAR(100), deleted_at DATETIME(3), updated_at DATETIME(3), updated_by BIGINT, roles JSON, effective_roles JSON, authz_policy_version BIGINT, authz_projected_at DATETIME(3), authz_projection_pending BOOLEAN)")
	for _, name := range []string{"000074_operator_retirement_tasks.up.sql", "000079_operator_recovery_archives.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("../../../pkg/migration/migrations/mysql", name))
		if err != nil {
			t.Fatal(err)
		}
		exec(string(raw))
	}
	exec("INSERT INTO operators(id,org_id,user_id,version,is_active,name) VALUES(7,1,800008,1,TRUE,'test'),(9,1,900009,1,TRUE,'admin')")
	repo := repository.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	task, err := domain.New(7, 1, 800008, 900009, 1, "exit", "retire", now)
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
	gateway := &facts{version: 10}
	tool := New(db, gateway, gateway)
	cmd := app.RecoveryCommand{Command: app.Command{OrgID: 1, ActorID: 900009, OperatorID: 7, ExpectedVersion: 3, RequestID: "restore", Reason: "reviewed HQ identity"}, ExpectedPolicyVersion: 10}
	before, err := tool.Preflight(ctx, cmd)
	if err != nil || before.Fingerprint == "" {
		t.Fatal(before, err)
	}
	again, err := tool.Preflight(ctx, cmd)
	if err != nil || again.Fingerprint != before.Fingerprint {
		t.Fatal("unstable fingerprint", err)
	}
	if _, err = tool.Apply(ctx, cmd, before.Fingerprint); err == nil {
		t.Fatal("unpaused writes accepted")
	}
	cmd.WritesStopped = true
	if _, err = tool.Apply(ctx, cmd, "wrong"); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	exec("UPDATE operators SET name='changed' WHERE id=7")
	if _, err = tool.Apply(ctx, cmd, before.Fingerprint); err == nil {
		t.Fatal("full row drift ignored")
	}
	reviewed, err := tool.Preflight(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := tool.Apply(ctx, cmd, reviewed.Fingerprint)
	if err != nil || applied.State != "historical_completed" {
		t.Fatal(applied, err)
	}
	var version uint32
	if err = db.Table("operators").Select("version").Where("id=7").Scan(&version).Error; err != nil || version != 4 {
		t.Fatal("unexpected recovery version", version, err)
	}
	if _, err = tool.Apply(ctx, cmd, reviewed.Fingerprint); err != nil {
		t.Fatal("replay failed", err)
	}
	// Subsequent legitimate edits must never be silently overwritten by replay.
	exec("UPDATE operators SET name='later edit',version=version+1 WHERE id=7")
	status, err := tool.Status(ctx, cmd)
	if err != nil || status.State != "historical_completed" {
		t.Fatal(status, err)
	}
	if _, err = tool.Apply(ctx, cmd, reviewed.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err = db.Table("operators").Select("version").Where("id=7").Scan(&version).Error; err != nil || version != 5 {
		t.Fatal("replay overwrote later change", version, err)
	}
	if gateway.writes != 0 {
		t.Fatal("IAM modified")
	}
	exec("UPDATE operator_recovery_archives SET checksum=REPEAT('0',64)")
	if _, err = tool.Status(ctx, cmd); err == nil {
		t.Fatal("corrupt receipt accepted")
	}
}
