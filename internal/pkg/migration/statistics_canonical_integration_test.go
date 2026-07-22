//go:build integration

package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
)

func TestStatisticsCanonicalSchemaFromEmptyDatabaseAndRetirementRollback(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is required for migration integration tests")
	}

	db, databaseName := openStatisticsMigrationDatabase(t, dsn)

	entries, err := os.ReadDir("migrations/mysql")
	if err != nil {
		t.Fatal(err)
	}
	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	sort.Strings(migrations)
	for _, name := range migrations {
		execSQLMigration(t, db, name)
	}

	assertCanonicalStatisticsSchema(t, db, databaseName)
	assertStatisticsCollectorIndexes(t, db, databaseName)

	execSQLMigration(t, db, "000056_retire_statistics_v1.down.sql")
	assertMySQLTable(t, db, databaseName, "statistics_org_snapshot", true)
	assertMySQLTable(t, db, databaseName, "statistics_v2_org_snapshot", true)
	for _, table := range retiredStatisticsTables() {
		assertMySQLTable(t, db, databaseName, table, true)
	}

	execSQLMigration(t, db, "000056_retire_statistics_v1.up.sql")
	assertCanonicalStatisticsSchema(t, db, databaseName)
	assertStatisticsCollectorIndexes(t, db, databaseName)
}

func openStatisticsMigrationDatabase(t *testing.T, dsn string) (*sql.DB, string) {
	t.Helper()
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("qs_statistics_migration_%d", time.Now().UnixNano())
	cfg.DBName = ""
	cfg.MultiStatements = true
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`")
		_ = server.Close()
	})

	cfg.DBName = databaseName
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, databaseName
}

func assertCanonicalStatisticsSchema(t *testing.T, db *sql.DB, databaseName string) {
	t.Helper()
	for _, table := range []string{
		"statistics_access_fact",
		"statistics_assessment_fact",
		"statistics_plan_fact",
		"statistics_access_daily",
		"statistics_assessment_daily",
		"statistics_plan_activity_daily",
		"statistics_plan_fulfillment_daily",
		"statistics_org_snapshot",
		"statistics_sync_run",
	} {
		assertMySQLTable(t, db, databaseName, table, true)
	}
	assertMySQLTable(t, db, databaseName, "statistics_v2_org_snapshot", false)
	for _, table := range retiredStatisticsTables() {
		assertMySQLTable(t, db, databaseName, table, false)
	}
}

func retiredStatisticsTables() []string {
	return []string{
		"behavior_footprint",
		"assessment_episode",
		"analytics_pending_event",
		"statistics_journey_daily",
		"statistics_content_daily",
		"statistics_plan_daily",
	}
}

func assertMySQLTable(t *testing.T, db *sql.DB, databaseName, table string, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name=?",
		databaseName,
		table,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if (count == 1) != want {
		t.Fatalf("table %s exists=%v, want %v", table, count == 1, want)
	}
}

func assertStatisticsCollectorIndexes(t *testing.T, db *sql.DB, databaseName string) {
	t.Helper()
	for table, indexes := range map[string][]string{
		"plan_enrollment": {"idx_enrollment_collect_joined", "idx_enrollment_collect_closed", "idx_enrollment_collect_terminated"},
		"assessment_task": {"idx_task_collect_created", "idx_task_collect_opened", "idx_task_collect_completed", "idx_task_collect_expired", "idx_task_collect_canceled"},
		"assessment":      {"idx_assessment_collect_created", "idx_assessment_collect_failed"},
	} {
		for _, index := range indexes {
			var count int
			if err := db.QueryRowContext(t.Context(),
				"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name=?",
				databaseName,
				table,
				index,
			).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count == 0 {
				t.Fatalf("index %s.%s does not exist", table, index)
			}
		}
	}
}
