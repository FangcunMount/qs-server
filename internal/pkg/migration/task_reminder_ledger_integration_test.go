//go:build integration

package migration

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestTaskReminderLedgerMigratorFrom86AndRefusesDataLoss(t *testing.T) {
	dsn := os.Getenv("QS_M5_REMINDER_MIGRATION_MYSQL_DSN")
	if dsn == "" {
		t.Skip("disposable task reminder migration MySQL not configured")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(parsed.DBName, "qs_m5_reminder_migrate_test_") {
		t.Fatal("disposable qs_m5_reminder_migrate_test_ database required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(), "DROP TABLE IF EXISTS task_opened_reminder_delivery"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "DROP TABLE IF EXISTS schema_migrations"); err != nil {
		t.Fatal(err)
	}
	config := ensureConfigDefaults(&Config{Enabled: true, Database: parsed.DBName})
	instance, err := NewMySQLDriver(db).CreateInstance(migrations, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Force(86); err != nil {
		t.Fatal(err)
	}
	version, changed, err := NewMigrator(db, config).Run()
	if err != nil || !changed || version != 87 {
		t.Fatalf("migration 86->87: version=%d changed=%t err=%v", version, changed, err)
	}
	var exists int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema=DATABASE() AND table_name='task_opened_reminder_delivery'`).Scan(&exists); err != nil || exists != 1 {
		t.Fatalf("reminder ledger table missing: exists=%d err=%v", exists, err)
	}
	version, changed, err = NewMigrator(db, config).Run()
	if err != nil || changed || version != 87 {
		t.Fatalf("repeat migration changed schema: version=%d changed=%t err=%v", version, changed, err)
	}
	if err := instance.Migrate(86); err == nil {
		t.Fatal("down migration unexpectedly discarded durable reminder responsibility")
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema=DATABASE() AND table_name='task_opened_reminder_delivery'`).Scan(&exists); err != nil || exists != 1 {
		t.Fatalf("failed down migration removed reminder ledger: exists=%d err=%v", exists, err)
	}
}
