package operatorprepare

import (
	"context"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	actorrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
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
	name := fmt.Sprintf("qs_operator_prepare_test_%d", time.Now().UnixNano())
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
	admin  bool
	roles  []string
	writes int
}

func (f *facts) IsEnabled() bool { return true }
func (f *facts) LoadFresh(context.Context, string) (*authz.Snapshot, error) {
	s := &authz.Snapshot{AuthzVersion: 136}
	if f.admin {
		s.Permissions = []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}
	}
	return s, nil
}
func (f *facts) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	return iambridge.OperatorRoleProjection{PolicyVersion: 136, DirectRoles: f.roles, EffectiveRoles: f.roles}, nil
}
func (f *facts) ReplaceManagedOperatorRoles(context.Context, int64, int64, []string, string, string) (int64, error) {
	f.writes++
	return 0, fmt.Errorf("unexpected authorization write")
}
func TestPreparationMySQL(t *testing.T) {
	db := isolatedMySQL(t)
	require.NoError(t, db.AutoMigrate(&actorrepo.OperatorPO{}))
	raw, err := os.ReadFile(filepath.Join("../../../pkg/migration/migrations/mysql", "000074_operator_retirement_tasks.up.sql"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(raw)).Error)
	require.NoError(t, db.Exec("INSERT INTO operators(id,org_id,user_id,version,is_active,name,roles,effective_roles) VALUES(9,1,900029,1,TRUE,'admin','[]','[]')").Error)
	f := &facts{admin: true}
	identityErr := error(nil)
	tool := New(db, f, f, func(_ context.Context, user int64) error { require.Equal(t, int64(800028), user); return identityErr })
	cmd := Command{OrgID: 1, UserID: 800028, ActorID: 900029, Name: "store account", RequestID: "prepare-1", Reason: "reviewed store preparation"}
	ctx := context.Background()
	plan, err := tool.Preflight(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, "pending", plan.State)
	again, err := tool.Preflight(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, plan.Fingerprint, again.Fingerprint)
	_, err = tool.Apply(ctx, cmd, "wrong")
	require.Error(t, err)
	f.admin = false
	_, err = tool.Apply(ctx, cmd, plan.Fingerprint)
	require.Error(t, err)
	f.admin = true
	f.roles = []string{"qs:assessment_operator"}
	_, err = tool.Apply(ctx, cmd, plan.Fingerprint)
	require.Error(t, err)
	f.roles = nil
	identityErr = fmt.Errorf("inactive IAM user")
	_, err = tool.Apply(ctx, cmd, plan.Fingerprint)
	require.Error(t, err)
	identityErr = nil
	require.NoError(t, db.Exec("CREATE TRIGGER reject_prepare BEFORE INSERT ON operators FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'").Error)
	_, err = tool.Apply(ctx, cmd, plan.Fingerprint)
	require.Error(t, err)
	require.NoError(t, db.Exec("DROP TRIGGER reject_prepare").Error)
	var count int64
	require.NoError(t, db.Table("operators").Where("user_id=?", cmd.UserID).Count(&count).Error)
	require.Zero(t, count)
	done, err := tool.Apply(ctx, cmd, plan.Fingerprint)
	require.NoError(t, err)
	require.Equal(t, "created_inactive", done.State)
	var row actorrepo.OperatorPO
	require.NoError(t, db.First(&row, "id=?", done.OperatorID).Error)
	require.False(t, row.IsActive)
	require.Equal(t, cmd.ActorID, int64(row.CreatedBy))
	require.Empty(t, row.Roles)
	require.Empty(t, row.EffectiveRoles)
	repeated, err := tool.Apply(ctx, cmd, plan.Fingerprint)
	require.NoError(t, err)
	require.Equal(t, "existing_inactive", repeated.State)
	var unchanged actorrepo.OperatorPO
	require.NoError(t, db.First(&unchanged, "id=?", done.OperatorID).Error)
	require.Equal(t, row, unchanged)
	require.Zero(t, f.writes)
	require.NoError(t, db.Exec("UPDATE operators SET org_id=2 WHERE id=?", done.OperatorID).Error)
	_, err = tool.Apply(ctx, cmd, plan.Fingerprint)
	require.Error(t, err)
	require.NoError(t, db.Exec("UPDATE operators SET org_id=1,is_active=TRUE WHERE id=?", done.OperatorID).Error)
	_, err = tool.Apply(ctx, cmd, plan.Fingerprint)
	require.Error(t, err)
	require.Zero(t, f.writes)
}
