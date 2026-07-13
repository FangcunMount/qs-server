package eventoutbox

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestClaimDueEventsQueryDocumentsDueStaleAndOrderContract(t *testing.T) {
	db := newDryRunOutboxDB(t)
	store := NewStoreWithTopicResolver(db, eventcatalog.NewCatalog(nil))
	now := time.Date(2026, 6, 5, 16, 8, 0, 0, time.UTC)
	var rows []OutboxPO

	stmt := store.dueEventsSelectionQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&OutboxPO{}),
		now,
	).Order("created_at ASC").Limit(25).Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"status = ?",
		"next_attempt_at <= ?",
		"updated_at <= ?",
		"ORDER BY created_at ASC",
		"LIMIT ?",
		"FOR UPDATE SKIP LOCKED",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{outboxcore.StatusPending, outboxcore.StatusFailed, outboxcore.StatusPublishing, 25} {
		if !containsOutboxVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func TestOutboxStatusSnapshotQueriesDocumentStatusAndOldestOrderContract(t *testing.T) {
	db := newDryRunOutboxDB(t)

	var count int64
	countStmt := outboxStatusCountQuery(
		db.Session(&gorm.Session{DryRun: true}),
		outboxcore.StatusPending,
	).Count(&count).Statement
	if sql := countStmt.SQL.String(); !strings.Contains(sql, "status = ?") {
		t.Fatalf("count query sql %q does not contain status filter", sql)
	}
	if !containsOutboxVar(countStmt.Vars, outboxcore.StatusPending) {
		t.Fatalf("count query vars = %#v, want status", countStmt.Vars)
	}

	var oldest OutboxPO
	oldestStmt := outboxOldestStatusQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&OutboxPO{}),
		outboxcore.StatusPending,
	).Find(&oldest).Statement
	sql := oldestStmt.SQL.String()
	for _, token := range []string{
		"status = ?",
		"ORDER BY created_at ASC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("oldest query sql %q does not contain %q", sql, token)
		}
	}
	if !containsOutboxVar(oldestStmt.Vars, outboxcore.StatusPending) || !containsOutboxVar(oldestStmt.Vars, 1) {
		t.Fatalf("oldest query vars = %#v, want status/limit", oldestStmt.Vars)
	}
}

func newDryRunOutboxDB(t *testing.T) *gorm.DB {
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

func containsOutboxVar(values []interface{}, want interface{}) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
