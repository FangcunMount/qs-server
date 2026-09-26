//go:build integration

package attentionprojection

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAttentionProjectionEvidenceIdentityAndSuccessAreStableMySQL(t *testing.T) {
	dsn := os.Getenv("QS_ATTENTION_LEDGER_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_ATTENTION_LEDGER_MYSQL_REQUIRED") == "1" {
			t.Fatal("disposable attention ledger MySQL DSN is required")
		}
		t.Skip("disposable attention ledger MySQL is not configured")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "127.0.0.1:3306" || parsed.DBName != "attention_ledger_ci" {
		t.Fatal("test requires disposable attention_ledger_ci MySQL at 127.0.0.1:3306")
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	const tableStart = "CREATE TABLE `interpretation_attention_projection` ("
	migration, err := os.ReadFile("../migration/migrations/mysql/000068_migrate_interpretation_runtime_ledgers.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	from := strings.Index(string(migration), tableStart)
	if from < 0 {
		t.Fatal("production attention projection schema not found")
	}
	statement := string(migration)[from:]
	statement = statement[:strings.Index(statement, ";")+1]
	if _, err := sqlDB.ExecContext(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = sqlDB.ExecContext(context.Background(), "DROP TABLE interpretation_attention_projection")
	}()
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewMySQLStore(gormDB)
	if err != nil {
		t.Fatal(err)
	}
	input := PendingInput{
		EventID: "evt-immutable", ReportID: "report-1", AssessmentID: "assessment-1",
		TesteeID: 99, RiskLevel: "severe", MarkKeyFocus: true,
	}
	if succeeded, err := store.EnsurePending(t.Context(), input); err != nil || succeeded {
		t.Fatalf("first EnsurePending succeeded=%t err=%v", succeeded, err)
	}
	changed := input
	changed.ReportID = "report-2"
	if _, err := store.EnsurePending(t.Context(), changed); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("pending identity conflict = %v", err)
	}
	if err := store.MarkSucceeded(t.Context(), input.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePending(t.Context(), changed); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("succeeded identity conflict = %v", err)
	}
	if status, err := store.RecordFailure(t.Context(), input.EventID, "late RPC failure", 1); err != nil || status != StatusSucceeded {
		t.Fatalf("late failure status=%q err=%v", status, err)
	}
	if succeeded, err := store.EnsurePending(t.Context(), input); err != nil || !succeeded {
		t.Fatalf("exact retry succeeded=%t err=%v", succeeded, err)
	}
	record, err := store.GetByEventID(t.Context(), input.EventID)
	if err != nil || !matchesPendingInput(record, input) || record.Status != StatusSucceeded || record.Attempt != 0 {
		t.Fatalf("persisted evidence changed: record=%+v err=%v", record, err)
	}
}
