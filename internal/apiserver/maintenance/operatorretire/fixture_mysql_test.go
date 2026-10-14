package operatorretire

import (
	"context"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	actorfixture "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/testfixture"
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

type factsFake struct {
	facts     []authz.AssignmentRoleFact
	version   int64
	writes    int
	fail      bool
	changedBy string
}

func (f *factsFake) LoadFresh(context.Context, string) (*authz.Snapshot, error) {
	return &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}, AuthzVersion: f.version}, nil
}
func (f *factsFake) LoadAssignmentFacts(context.Context, string) (*authz.Snapshot, error) {
	return &authz.Snapshot{AssignmentFacts: append([]authz.AssignmentRoleFact{}, f.facts...), AssignmentFactsComplete: true, AuthzVersion: f.version}, nil
}
func (f *factsFake) IsEnabled() bool { return true }
func (f *factsFake) ReplaceManagedOperatorRoles(_ context.Context, _, _ int64, _ []string, changedBy, _ string) (int64, error) {
	f.changedBy = changedBy
	if f.fail {
		return 0, fmt.Errorf("injected IAM failure")
	}
	f.facts = retained(f.facts)
	f.version++
	f.writes++
	return f.version, nil
}
func (f *factsFake) LoadOperatorRoleProjection(context.Context, int64, int64) (iambridge.OperatorRoleProjection, error) {
	p := iambridge.OperatorRoleProjection{PolicyVersion: f.version}
	for _, r := range f.facts {
		p.DirectRoles = append(p.DirectRoles, r.RoleName)
		p.EffectiveRoles = append(p.EffectiveRoles, r.RoleName)
	}
	return p, nil
}
func migration(t *testing.T, db *gorm.DB, number int, direction string) error {
	t.Helper()
	paths, e := filepath.Glob(fmt.Sprintf("../../../pkg/migration/migrations/mysql/%06d_*.%s.sql", number, direction))
	if e != nil || len(paths) != 1 {
		t.Fatal("migration missing", number, paths, e)
	}
	raw, e := os.ReadFile(paths[0])
	if e != nil {
		t.Fatal(e)
	}
	return db.Exec(string(raw)).Error
}
func fixture(t *testing.T) (*gorm.DB, *factsFake) {
	db := isolatedMySQL(t)
	ddl := []string{
		"CREATE TABLE operators(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,user_id BIGINT,version INT UNSIGNED,is_active BOOLEAN,deleted_at DATETIME(3),updated_at DATETIME(3),updated_by BIGINT,roles JSON,effective_roles JSON,authz_policy_version BIGINT,authz_projected_at DATETIME(3),authz_projection_pending BOOLEAN,UNIQUE KEY uk_operators_org_user(org_id,user_id),KEY idx_operators_org_deleted_id(org_id,deleted_at,id),KEY idx_operators_org_active_deleted_id(org_id,is_active,deleted_at,id))",
		"CREATE TABLE clinician(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,operator_id BIGINT UNSIGNED NULL,clinician_type VARCHAR(32),deleted_at DATETIME(3),UNIQUE KEY uk_clinician_org_operator(org_id,operator_id),KEY idx_operator_id(operator_id))",
		"INSERT INTO operators(id,org_id,user_id,version,is_active,roles,effective_roles,authz_policy_version,authz_projection_pending) VALUES(7,1,8,3,TRUE,JSON_ARRAY('qs:result_reviewer'),JSON_ARRAY('qs:result_reviewer'),10,FALSE),(9,1,9,1,TRUE,JSON_ARRAY('qs:admin'),JSON_ARRAY('qs:admin'),10,FALSE)",
		"INSERT INTO clinician(id,org_id,operator_id,clinician_type)VALUES(70,1,7,'doctor')",
	}
	for _, s := range ddl {
		if e := db.Exec(s).Error; e != nil {
			t.Fatal(e)
		}
	}
	for _, table := range []string{"actor_stores", "clinician_relation", "assessment_entry", "testee", "assessment", "assessment_plan", "assessment_task", "plan_enrollment"} {
		if e := db.Exec("CREATE TABLE " + table + "(id BIGINT PRIMARY KEY)").Error; e != nil {
			t.Fatal(e)
		}
	}
	for _, n := range []int{74, 75} {
		if e := migration(t, db, n, "up"); e != nil {
			t.Fatal(e)
		}
	}
	f := &factsFake{version: 10, facts: []authz.AssignmentRoleFact{{RoleID: "1", RoleName: "user", ManagementProtection: "standard"}, {RoleID: "2", RoleName: "qs:result_reviewer", ManagementProtection: "standard"}}}
	return db, f
}
func installCurrentActorColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	// The original Actor-column resource runs on the disposable prior shape.
	if err := db.Exec("ALTER TABLE operators ADD COLUMN name VARCHAR(100), ADD COLUMN email VARCHAR(255), ADD COLUMN phone VARCHAR(20), ADD COLUMN created_at DATETIME(3), ADD COLUMN created_by BIGINT, ADD COLUMN deleted_by BIGINT; ALTER TABLE testee ADD COLUMN tags JSON").Error; err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("../../../pkg/migration/migrations/mysql", "000102_retire_actor_compatibility_columns.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	actorfixture.RetireActorColumns(t, db, raw)
}

func retained(facts []authz.AssignmentRoleFact) []authz.AssignmentRoleFact {
	out := []authz.AssignmentRoleFact{}
	for _, f := range facts {
		if f.RoleName == "user" {
			out = append(out, f)
		}
	}
	return out
}
