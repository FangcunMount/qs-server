package actor

import (
	"database/sql"
	"strings"
	"testing"

	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestBuildActiveTesteeRelationsQueryDocumentsWorkbenchAssignmentContract(t *testing.T) {
	db := newDryRunActorDB(t)
	var rows []ClinicianRelationPO

	stmt := buildActiveTesteeRelationsQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&ClinicianRelationPO{}),
		9,
		[]uint64{3002, 3001, 3001},
		[]string{"primary", "attending", "collaborator"},
	).Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"org_id = ?",
		"testee_id IN",
		"is_active = ?",
		"deleted_at IS NULL",
		"relation_type IN",
		"ORDER BY testee_id ASC",
		"CASE relation_type WHEN 'primary' THEN 0",
		"bound_at DESC",
		"id DESC",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), true} {
		if !containsActorVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func newDryRunActorDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/qs_server_dry_run?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn:                      conn,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}
	return db
}

func containsActorVar(values []interface{}, want interface{}) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
