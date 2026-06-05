package statistics

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestBuildDueAnalyticsPendingEventsQueryDocumentsDueOrderContract(t *testing.T) {
	db := newDryRunStatisticsDB(t)
	now := time.Date(2026, 6, 5, 16, 8, 0, 0, time.UTC)
	var rows []AnalyticsPendingEventPO

	stmt := buildDueAnalyticsPendingEventsQuery(
		db.Session(&gorm.Session{DryRun: true}),
		now,
	).Limit(50).Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"next_attempt_at <= ?",
		"deleted_at IS NULL",
		"ORDER BY next_attempt_at ASC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	if !containsStatisticsVar(stmt.Vars, now) || !containsStatisticsVar(stmt.Vars, 50) {
		t.Fatalf("query vars = %#v, want now/limit", stmt.Vars)
	}
}

func newDryRunStatisticsDB(t *testing.T) *gorm.DB {
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

func containsStatisticsVar(values []interface{}, want interface{}) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
