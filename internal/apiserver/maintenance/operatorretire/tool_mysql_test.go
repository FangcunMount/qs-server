package operatorretire

import (
	"context"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
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
	facts   []authz.AssignmentRoleFact
	version int64
	writes  int
	fail    bool
}

func (f *factsFake) LoadFresh(context.Context, string) (*authz.Snapshot, error) {
	return &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}, AuthzVersion: f.version}, nil
}
func (f *factsFake) LoadAssignmentFacts(context.Context, string) (*authz.Snapshot, error) {
	return &authz.Snapshot{AssignmentFacts: append([]authz.AssignmentRoleFact{}, f.facts...), AssignmentFactsComplete: true, AuthzVersion: f.version}, nil
}
func (f *factsFake) IsEnabled() bool { return true }
func (f *factsFake) ReplaceManagedOperatorRoles(context.Context, int64, int64, []string, string, string) (int64, error) {
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
func fixture(t *testing.T) (*gorm.DB, *Tool, *factsFake) {
	db := isolatedMySQL(t)
	ddl := []string{
		"CREATE TABLE staff(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,user_id BIGINT,version INT UNSIGNED,is_active BOOLEAN,deleted_at DATETIME(3),updated_at DATETIME(3),updated_by BIGINT,roles JSON,effective_roles JSON,authz_policy_version BIGINT,authz_projected_at DATETIME(3),authz_projection_pending BOOLEAN,UNIQUE KEY uk_staff_org_user(org_id,user_id),KEY idx_staff_org_deleted_id(org_id,deleted_at,id),KEY idx_staff_org_active_deleted_id(org_id,is_active,deleted_at,id))",
		"CREATE TABLE clinician(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,operator_id BIGINT UNSIGNED NULL,clinician_type VARCHAR(32),deleted_at DATETIME(3),UNIQUE KEY uk_clinician_org_operator(org_id,operator_id),KEY idx_operator_id(operator_id))",
		"INSERT INTO staff(id,org_id,user_id,version,is_active,roles,effective_roles,authz_policy_version,authz_projection_pending) VALUES(7,1,8,3,TRUE,JSON_ARRAY('qs:result_reviewer'),JSON_ARRAY('qs:result_reviewer'),10,FALSE),(9,1,9,1,TRUE,JSON_ARRAY('qs:admin'),JSON_ARRAY('qs:admin'),10,FALSE)",
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
	tool, e := New(db, f, f, "legacy")
	if e != nil {
		t.Fatal(e)
	}
	return db, tool, f
}
func TestMySQLRetirementCutoverAndRestore(t *testing.T) {
	db, tool, f := fixture(t)
	ctx := context.Background()
	if e := migration(t, db, 76, "up"); e == nil {
		t.Fatal("unretired doctor passed schema guard")
	}
	before, e := tool.Preflight(ctx, "doctor-test", 9)
	if e != nil {
		t.Fatal(e)
	}
	done, e := tool.Apply(ctx, before.MigrationID, before.Fingerprint, 9, true)
	if e != nil {
		t.Fatal(e)
	}
	if done.State != "applied_unchanged" || f.writes != 1 {
		t.Fatal(done.State, f.writes)
	}
	// Refreshing an unrelated administrator projection to the already committed
	// IAM policy is expected convergence, not a new authorization fact.
	if e = db.Exec("UPDATE staff SET authz_policy_version=11,authz_projected_at=NOW(3) WHERE id=9").Error; e != nil {
		t.Fatal(e)
	}
	if _, e = tool.Verify(ctx, before.MigrationID); e != nil {
		t.Fatal("projection refresh reported as fact drift", e)
	}
	for i := 0; i < 2; i++ {
		if _, e = tool.Preflight(ctx, before.MigrationID, 9); e != nil {
			t.Fatal(e)
		}
		if _, e = tool.Apply(ctx, before.MigrationID, before.Fingerprint, 9, true); e != nil {
			t.Fatal(e)
		}
	}
	if f.writes != 1 {
		t.Fatal("replay revoked again")
	}
	if _, e = tool.Apply(ctx, before.MigrationID, "wrong", 9, true); e == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	if e = migration(t, db, 76, "up"); e != nil {
		t.Fatal(e)
	}
	final, e := New(db, f, f, "final")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = final.Verify(ctx, before.MigrationID); e != nil {
		t.Fatal(e)
	}
	if e = migration(t, db, 76, "down"); e != nil {
		t.Fatal(e)
	}
	if _, e = tool.Verify(ctx, before.MigrationID); e != nil {
		t.Fatal(e)
	}
	db.Exec("UPDATE clinician_operator_binding_archive SET record_sha256=REPEAT('0',64)")
	if r, e := tool.Status(ctx, before.MigrationID); e == nil || r.State != "invalid" {
		t.Fatal("corrupt archive accepted", r, e)
	}
}
func TestMySQLRetirementResumeAndDrift(t *testing.T) {
	db, tool, f := fixture(t)
	ctx := context.Background()
	before, e := tool.Preflight(ctx, "doctor-resume", 9)
	if e != nil {
		t.Fatal(e)
	}
	f.fail = true
	if _, e = tool.Apply(ctx, before.MigrationID, before.Fingerprint, 9, true); e == nil {
		t.Fatal("IAM failure ignored")
	}
	var active bool
	db.Table("staff").Select("is_active").Where("id=7").Scan(&active)
	if active {
		t.Fatal("failed exit not disabled")
	}
	db.Exec("UPDATE clinician SET operator_id=NULL WHERE id=70")
	f.fail = false
	if _, e = tool.Apply(ctx, before.MigrationID, before.Fingerprint, 9, true); e == nil {
		t.Fatal("binding drift accepted")
	}
	db.Exec("UPDATE clinician SET operator_id=7 WHERE id=70")
	if _, e = tool.Apply(ctx, before.MigrationID, before.Fingerprint, 9, true); e != nil {
		t.Fatal(e)
	}
	f.facts = append(f.facts, authz.AssignmentRoleFact{RoleID: "3", RoleName: "qs:content_manager", ManagementProtection: "standard"})
	f.version++
	r, e := tool.Status(ctx, before.MigrationID)
	if e != nil || r.State != "applied_drifted" {
		t.Fatal(r, e)
	}
	if _, e = tool.Apply(ctx, before.MigrationID, before.Fingerprint, 9, true); e == nil {
		t.Fatal("drift overwritten")
	}
	if f.writes != 1 {
		t.Fatal("unexpected authorization writes", f.writes)
	}
}
