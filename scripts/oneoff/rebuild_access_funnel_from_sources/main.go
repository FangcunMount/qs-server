package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const scopeTableName = "oneoff_access_funnel_rebuild_scope"

type config struct {
	mysqlDSN              string
	orgID                 int64
	allOrgs               bool
	startDateRaw          string
	endDateRaw            string
	timeout               time.Duration
	apply                 bool
	skipBackup            bool
	backupSuffix          string
	testeeSourceRaw       string
	inferredTesteeCreated bool
	previewLimit          int
}

type rebuildScopeSummary struct {
	SourceKind        string
	Rows              int64
	TesteeCreated     int64
	AssignmentCreated int64
}

type dailyPreviewRow struct {
	StatDate          string
	SourceKind        string
	Rows              int64
	TesteeCreated     int64
	AssignmentCreated int64
}

type statementResult struct {
	Name     string
	Affected int64
}

type statementSpec struct {
	Name  string
	Query string
	Args  []any
}

func main() {
	cfg := parseFlags()
	startDate := mustParseDate("start-date", cfg.startDateRaw)
	endDate := parseOptionalDate("end-date", cfg.endDateRaw)
	if endDate == nil {
		tomorrow := beginningOfDay(time.Now()).AddDate(0, 0, 1)
		endDate = &tomorrow
	}
	if !startDate.Before(*endDate) {
		log.Fatal("--start-date must be before --end-date")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	db, err := openMySQL(cfg.mysqlDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("close mysql: %v", err)
		}
	}()
	if err := pingAndPrepare(ctx, db); err != nil {
		log.Fatalf("prepare mysql: %v", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("open mysql connection: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close mysql connection: %v", err)
		}
	}()

	if err := prepareScope(ctx, conn, cfg, startDate, *endDate); err != nil {
		log.Fatalf("prepare rebuild scope: %v", err)
	}
	if err := printPreview(ctx, conn, cfg, startDate, *endDate); err != nil {
		log.Fatalf("preview rebuild scope: %v", err)
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to rebuild assessment_entry_intake_log and statistics_journey_daily.access_*")
		return
	}

	if !cfg.skipBackup {
		if cfg.backupSuffix == "" {
			cfg.backupSuffix = time.Now().Format("20060102_150405")
		}
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
		if err := createBackups(ctx, conn, cfg, startDate, *endDate); err != nil {
			log.Fatalf("create backups: %v", err)
		}
	}

	results, err := applyRebuild(ctx, conn, cfg, startDate, *endDate)
	if err != nil {
		log.Fatalf("apply rebuild: %v", err)
	}
	for _, item := range results {
		log.Printf("applied %-42s affected_rows=%d", item.Name, item.Affected)
	}
	log.Print("access funnel rebuild completed")
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to rebuild; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "rebuild all organizations")
	flag.StringVar(&cfg.startDateRaw, "start-date", "2025-01-01", "inclusive lower bound, format YYYY-MM-DD")
	flag.StringVar(&cfg.endDateRaw, "end-date", "", "exclusive upper bound, format YYYY-MM-DD; default is tomorrow")
	flag.DurationVar(&cfg.timeout, "timeout", 4*time.Hour, "overall script timeout, e.g. 30m, 4h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup tables; only use when an external backup already exists")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", "", "backup table suffix, letters/digits/underscore only; default is current timestamp")
	flag.StringVar(&cfg.testeeSourceRaw, "testee-source", "daily_simulation", "comma-separated testee.source values for inferring missing intake logs; empty means no source filter")
	flag.BoolVar(&cfg.inferredTesteeCreated, "inferred-testee-created", true, "mark inferred intake rows as testee_created=1")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 30, "maximum daily preview rows to print")
	flag.Parse()

	if strings.TrimSpace(cfg.mysqlDSN) == "" {
		log.Fatal("--mysql-dsn is required")
	}
	if cfg.allOrgs && cfg.orgID > 0 {
		log.Fatal("--org-id and --all-orgs are mutually exclusive")
	}
	if !cfg.allOrgs && cfg.orgID <= 0 {
		log.Fatal("one of --org-id or --all-orgs is required")
	}
	if cfg.previewLimit < 0 {
		log.Fatal("--preview-limit must be >= 0")
	}
	if cfg.backupSuffix != "" {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
	}
	return cfg
}

func openMySQL(dsn string) (*sql.DB, error) {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	c.ParseTime = true
	if c.Collation == "" {
		c.Collation = "utf8mb4_unicode_ci"
	}
	return sql.Open("mysql", c.FormatDSN())
}

func pingAndPrepare(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func prepareScope(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) error {
	if _, err := conn.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS "+scopeTableName); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE `+scopeTableName+` (
  source_kind VARCHAR(16) NOT NULL,
  existing_id BIGINT UNSIGNED NULL,
  org_id BIGINT NOT NULL,
  clinician_id BIGINT UNSIGNED NOT NULL,
  entry_id BIGINT UNSIGNED NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  testee_created TINYINT(1) NOT NULL DEFAULT 0,
  assignment_created TINYINT(1) NOT NULL DEFAULT 0,
  intake_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_scope_kind (source_kind),
  KEY idx_scope_org_time (org_id, intake_at),
  KEY idx_scope_unique (org_id, clinician_id, entry_id, testee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, buildExistingIntakeScopeInsert(cfg), buildExistingIntakeScopeArgs(cfg, startDate, endDate)...); err != nil {
		return fmt.Errorf("load existing intake logs: %w", err)
	}
	if _, err := conn.ExecContext(ctx, buildInferredIntakeScopeInsert(cfg), buildInferredIntakeScopeArgs(cfg, startDate, endDate)...); err != nil {
		return fmt.Errorf("load inferred intake logs: %w", err)
	}
	return nil
}

func printPreview(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) error {
	log.Printf("scope: %s start=%s end=%s apply=%v testee_sources=%q inferred_testee_created=%v",
		scopeDescription(cfg), formatDay(startDate), formatDay(endDate), cfg.apply, strings.TrimSpace(cfg.testeeSourceRaw), cfg.inferredTesteeCreated)

	summaries, err := loadScopeSummaries(ctx, conn)
	if err != nil {
		return err
	}
	var totalRows, totalTesteeCreated, totalAssignmentCreated int64
	for _, item := range summaries {
		totalRows += item.Rows
		totalTesteeCreated += item.TesteeCreated
		totalAssignmentCreated += item.AssignmentCreated
		log.Printf("candidate %-10s rows=%d testee_created=%d assignment_created=%d",
			item.SourceKind, item.Rows, item.TesteeCreated, item.AssignmentCreated)
	}
	log.Printf("candidate %-10s rows=%d testee_created=%d assignment_created=%d",
		"total", totalRows, totalTesteeCreated, totalAssignmentCreated)

	resolveCount, err := countResolveLogs(ctx, conn, cfg, startDate, endDate)
	if err != nil {
		return err
	}
	aggregateRows, err := countAggregateRows(ctx, conn, cfg, startDate, endDate)
	if err != nil {
		return err
	}
	log.Printf("source resolve_logs=%d aggregate_rows_to_reset=%d", resolveCount, aggregateRows)

	if cfg.previewLimit > 0 {
		rows, err := loadDailyPreview(ctx, conn, cfg.previewLimit)
		if err != nil {
			return err
		}
		for _, item := range rows {
			log.Printf("preview day=%s kind=%s rows=%d testee_created=%d assignment_created=%d",
				item.StatDate, item.SourceKind, item.Rows, item.TesteeCreated, item.AssignmentCreated)
		}
	}
	return nil
}

func loadScopeSummaries(ctx context.Context, conn *sql.Conn) ([]rebuildScopeSummary, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT source_kind, COUNT(*), COALESCE(SUM(testee_created), 0), COALESCE(SUM(assignment_created), 0)
FROM `+scopeTableName+`
GROUP BY source_kind
ORDER BY source_kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []rebuildScopeSummary
	for rows.Next() {
		var item rebuildScopeSummary
		if err := rows.Scan(&item.SourceKind, &item.Rows, &item.TesteeCreated, &item.AssignmentCreated); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadDailyPreview(ctx context.Context, conn *sql.Conn, limit int) ([]dailyPreviewRow, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT DATE(intake_at) AS stat_date, source_kind, COUNT(*), COALESCE(SUM(testee_created), 0), COALESCE(SUM(assignment_created), 0)
FROM `+scopeTableName+`
GROUP BY DATE(intake_at), source_kind
ORDER BY stat_date, source_kind
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []dailyPreviewRow
	for rows.Next() {
		var item dailyPreviewRow
		if err := rows.Scan(&item.StatDate, &item.SourceKind, &item.Rows, &item.TesteeCreated, &item.AssignmentCreated); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func countResolveLogs(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) (int64, error) {
	query := `SELECT COUNT(*) FROM assessment_entry_resolve_log l WHERE l.deleted_at IS NULL AND l.resolved_at >= ? AND l.resolved_at < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "l")
	var count int64
	if err := conn.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func countAggregateRows(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) (int64, error) {
	query := `SELECT COUNT(*) FROM statistics_journey_daily d WHERE d.deleted_at IS NULL AND d.subject_type = 'org' AND d.subject_id = 0 AND d.stat_date >= ? AND d.stat_date < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "d")
	var count int64
	if err := conn.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func createBackups(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) error {
	intakeBackup := backupTableName("oneoff_bak_access_intake_log", cfg.backupSuffix)
	journeyBackup := backupTableName("oneoff_bak_access_journey_daily", cfg.backupSuffix)

	if _, err := conn.ExecContext(ctx, "CREATE TABLE "+intakeBackup+" LIKE assessment_entry_intake_log"); err != nil {
		return fmt.Errorf("create %s: %w", intakeBackup, err)
	}
	intakeSQL := "INSERT INTO " + intakeBackup + " SELECT * FROM assessment_entry_intake_log l WHERE l.deleted_at IS NULL AND l.intake_at >= ? AND l.intake_at < ?"
	intakeArgs := []any{startDate, endDate}
	intakeSQL, intakeArgs = appendOrgPredicate(intakeSQL, intakeArgs, cfg, "l")
	if _, err := conn.ExecContext(ctx, intakeSQL, intakeArgs...); err != nil {
		return fmt.Errorf("backup assessment_entry_intake_log: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "CREATE TABLE "+journeyBackup+" LIKE statistics_journey_daily"); err != nil {
		return fmt.Errorf("create %s: %w", journeyBackup, err)
	}
	journeySQL := "INSERT INTO " + journeyBackup + " SELECT * FROM statistics_journey_daily d WHERE d.deleted_at IS NULL AND d.subject_type = 'org' AND d.subject_id = 0 AND d.stat_date >= ? AND d.stat_date < ?"
	journeyArgs := []any{startDate, endDate}
	journeySQL, journeyArgs = appendOrgPredicate(journeySQL, journeyArgs, cfg, "d")
	if _, err := conn.ExecContext(ctx, journeySQL, journeyArgs...); err != nil {
		return fmt.Errorf("backup statistics_journey_daily: %w", err)
	}
	log.Printf("backup tables created: %s, %s", intakeBackup, journeyBackup)
	return nil
}

func applyRebuild(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) ([]statementResult, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	results := make([]statementResult, 0, 6)
	for _, stmt := range []statementSpec{
		buildDeleteIntakeLogs(cfg, startDate, endDate),
		{Name: "restore_existing_intake_logs", Query: restoreExistingIntakeLogsSQL},
		{Name: "insert_inferred_intake_logs", Query: insertInferredIntakeLogsSQL},
		buildResetAccessAggregates(cfg, startDate, endDate),
		buildUpsertAccessAggregates(cfg, startDate, endDate),
	} {
		res, err := tx.ExecContext(ctx, stmt.Query, stmt.Args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.Name, err)
		}
		affected, _ := res.RowsAffected()
		results = append(results, statementResult{Name: stmt.Name, Affected: affected})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func buildExistingIntakeScopeInsert(cfg config) string {
	query := `
INSERT INTO ` + scopeTableName + ` (
  source_kind, existing_id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  'existing',
  l.id,
  l.org_id,
  l.clinician_id,
  l.entry_id,
  l.testee_id,
  l.testee_created,
  l.assignment_created,
  l.intake_at,
  l.created_at,
  l.updated_at
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND l.intake_at >= ?
  AND l.intake_at < ?`
	query, _ = appendOrgPredicate(query, nil, cfg, "l")
	return query
}

func buildExistingIntakeScopeArgs(cfg config, startDate, endDate time.Time) []any {
	args := []any{startDate, endDate}
	_, args = appendOrgPredicate("", args, cfg, "l")
	return args
}

func buildInferredIntakeScopeInsert(cfg config) string {
	testeeSources := parseCSV(cfg.testeeSourceRaw)
	sourcePredicate := ""
	if len(testeeSources) > 0 {
		sourcePredicate = " AND t.source IN (" + placeholders(len(testeeSources)) + ")"
	}
	testeeCreatedValue := "0"
	if cfg.inferredTesteeCreated {
		testeeCreatedValue = "1"
	}

	query := `
INSERT INTO ` + scopeTableName + ` (
  source_kind, existing_id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  'inferred',
  NULL,
  cr.org_id,
  cr.clinician_id,
  cr.source_id AS entry_id,
  cr.testee_id,
  ` + testeeCreatedValue + ` AS testee_created,
  1 AS assignment_created,
  MIN(cr.bound_at) AS intake_at,
  MIN(cr.created_at) AS created_at,
  MAX(cr.updated_at) AS updated_at
FROM clinician_relation cr
INNER JOIN testee t
  ON t.id = cr.testee_id
 AND t.org_id = cr.org_id
 AND t.deleted_at IS NULL
INNER JOIN assessment_entry ae
  ON ae.id = cr.source_id
 AND ae.org_id = cr.org_id
 AND ae.clinician_id = cr.clinician_id
 AND ae.deleted_at IS NULL
LEFT JOIN assessment_entry_intake_log l
  ON l.org_id = cr.org_id
 AND l.clinician_id = cr.clinician_id
 AND l.entry_id = cr.source_id
 AND l.testee_id = cr.testee_id
 AND l.deleted_at IS NULL
WHERE cr.deleted_at IS NULL
  AND cr.source_type = 'assessment_entry'
  AND cr.source_id IS NOT NULL
  AND cr.relation_type IN ('assigned', 'primary', 'attending', 'collaborator')
  AND cr.bound_at >= ?
  AND cr.bound_at < ?
  AND l.id IS NULL` + sourcePredicate
	query, _ = appendOrgPredicate(query, nil, cfg, "cr")
	query += `
GROUP BY cr.org_id, cr.clinician_id, cr.source_id, cr.testee_id`
	return query
}

func buildInferredIntakeScopeArgs(cfg config, startDate, endDate time.Time) []any {
	args := []any{startDate, endDate}
	for _, item := range parseCSV(cfg.testeeSourceRaw) {
		args = append(args, item)
	}
	_, args = appendOrgPredicate("", args, cfg, "cr")
	return args
}

func buildDeleteIntakeLogs(cfg config, startDate, endDate time.Time) statementSpec {
	query := `DELETE FROM assessment_entry_intake_log WHERE deleted_at IS NULL AND intake_at >= ? AND intake_at < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "")
	return statementSpec{Name: "delete_window_intake_logs", Query: query, Args: args}
}

func buildResetAccessAggregates(cfg config, startDate, endDate time.Time) statementSpec {
	query := `
UPDATE statistics_journey_daily
SET
  access_entry_opened_count = 0,
  access_intake_confirmed_count = 0,
  access_testee_created_count = 0,
  access_care_relationship_established_count = 0,
  updated_at = NOW(3)
WHERE deleted_at IS NULL
  AND subject_type = 'org'
  AND subject_id = 0
  AND stat_date >= ?
  AND stat_date < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "")
	return statementSpec{Name: "reset_access_aggregate_columns", Query: query, Args: args}
}

func buildUpsertAccessAggregates(cfg config, startDate, endDate time.Time) statementSpec {
	query := `
INSERT INTO statistics_journey_daily (
  org_id,
  subject_type,
  subject_id,
  clinician_id,
  entry_id,
  stat_date,
  access_entry_opened_count,
  access_intake_confirmed_count,
  access_testee_created_count,
  access_care_relationship_established_count,
  created_at,
  updated_at,
  deleted_at
)
SELECT
  d.org_id,
  'org',
  0,
  0,
  0,
  d.stat_date,
  GREATEST(COALESCE(o.entry_opened_count, 0), COALESCE(i.intake_confirmed_count, 0)),
  COALESCE(i.intake_confirmed_count, 0),
  COALESCE(tc.testee_created_count, 0),
  COALESCE(crr.care_relationship_established_count, 0),
  NOW(3),
  NOW(3),
  NULL
FROM (
  SELECT org_id, DATE(resolved_at) AS stat_date
  FROM assessment_entry_resolve_log
  WHERE deleted_at IS NULL AND resolved_at >= ? AND resolved_at < ?` + orgPredicateSQL(cfg, "") + `
  GROUP BY org_id, DATE(resolved_at)
  UNION
  SELECT org_id, DATE(intake_at) AS stat_date
  FROM assessment_entry_intake_log
  WHERE deleted_at IS NULL AND intake_at >= ? AND intake_at < ?` + orgPredicateSQL(cfg, "") + `
  GROUP BY org_id, DATE(intake_at)
) d
LEFT JOIN (
  SELECT org_id, DATE(resolved_at) AS stat_date, COUNT(*) AS entry_opened_count
  FROM assessment_entry_resolve_log
  WHERE deleted_at IS NULL AND resolved_at >= ? AND resolved_at < ?` + orgPredicateSQL(cfg, "") + `
  GROUP BY org_id, DATE(resolved_at)
) o ON o.org_id = d.org_id AND o.stat_date = d.stat_date
LEFT JOIN (
  SELECT org_id, DATE(intake_at) AS stat_date, COUNT(*) AS intake_confirmed_count
  FROM assessment_entry_intake_log
  WHERE deleted_at IS NULL AND intake_at >= ? AND intake_at < ?` + orgPredicateSQL(cfg, "") + `
  GROUP BY org_id, DATE(intake_at)
) i ON i.org_id = d.org_id AND i.stat_date = d.stat_date
LEFT JOIN (
  SELECT org_id, DATE(intake_at) AS stat_date, COUNT(*) AS testee_created_count
  FROM assessment_entry_intake_log
  WHERE deleted_at IS NULL AND testee_created = 1 AND intake_at >= ? AND intake_at < ?` + orgPredicateSQL(cfg, "") + `
  GROUP BY org_id, DATE(intake_at)
) tc ON tc.org_id = d.org_id AND tc.stat_date = d.stat_date
LEFT JOIN (
  SELECT org_id, DATE(intake_at) AS stat_date, COUNT(*) AS care_relationship_established_count
  FROM assessment_entry_intake_log
  WHERE deleted_at IS NULL AND assignment_created = 1 AND intake_at >= ? AND intake_at < ?` + orgPredicateSQL(cfg, "") + `
  GROUP BY org_id, DATE(intake_at)
) crr ON crr.org_id = d.org_id AND crr.stat_date = d.stat_date
ON DUPLICATE KEY UPDATE
  access_entry_opened_count = VALUES(access_entry_opened_count),
  access_intake_confirmed_count = VALUES(access_intake_confirmed_count),
  access_testee_created_count = VALUES(access_testee_created_count),
  access_care_relationship_established_count = VALUES(access_care_relationship_established_count),
  updated_at = VALUES(updated_at),
  deleted_at = NULL`

	args := make([]any, 0, 18)
	for i := 0; i < 6; i++ {
		args = append(args, startDate, endDate)
		if cfg.orgID > 0 {
			args = append(args, cfg.orgID)
		}
	}
	return statementSpec{Name: "upsert_access_aggregate_columns", Query: query, Args: args}
}

const restoreExistingIntakeLogsSQL = `
INSERT INTO assessment_entry_intake_log (
  id,
  org_id,
  clinician_id,
  entry_id,
  testee_id,
  testee_created,
  assignment_created,
  intake_at,
  created_at,
  updated_at
)
SELECT
  existing_id,
  org_id,
  clinician_id,
  entry_id,
  testee_id,
  testee_created,
  assignment_created,
  intake_at,
  created_at,
  updated_at
FROM ` + scopeTableName + `
WHERE source_kind = 'existing'
ORDER BY intake_at, existing_id`

const insertInferredIntakeLogsSQL = `
INSERT INTO assessment_entry_intake_log (
  org_id,
  clinician_id,
  entry_id,
  testee_id,
  testee_created,
  assignment_created,
  intake_at,
  created_at,
  updated_at
)
SELECT
  org_id,
  clinician_id,
  entry_id,
  testee_id,
  testee_created,
  assignment_created,
  intake_at,
  created_at,
  updated_at
FROM ` + scopeTableName + `
WHERE source_kind = 'inferred'
ORDER BY intake_at, org_id, clinician_id, entry_id, testee_id`

func appendOrgPredicate(query string, args []any, cfg config, alias string) (string, []any) {
	if cfg.orgID <= 0 {
		return query, args
	}
	column := "org_id"
	if strings.TrimSpace(alias) != "" {
		column = strings.TrimSpace(alias) + ".org_id"
	}
	query += " AND " + column + " = ?"
	args = append(args, cfg.orgID)
	return query, args
}

func orgPredicateSQL(cfg config, alias string) string {
	if cfg.orgID <= 0 {
		return ""
	}
	column := "org_id"
	if strings.TrimSpace(alias) != "" {
		column = strings.TrimSpace(alias) + ".org_id"
	}
	return " AND " + column + " = ?"
}

func parseCSV(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	values := make([]string, n)
	for i := range values {
		values[i] = "?"
	}
	return strings.Join(values, ",")
}

func scopeDescription(cfg config) string {
	if cfg.allOrgs {
		return "all_orgs"
	}
	return "org_id=" + strconv.FormatInt(cfg.orgID, 10)
}

func mustParseDate(name, raw string) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		log.Fatalf("--%s is required", name)
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		log.Fatalf("invalid --%s %q: %v", name, raw, err)
	}
	return beginningOfDay(parsed)
}

func parseOptionalDate(name, raw string) *time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	parsed := mustParseDate(name, value)
	return &parsed
}

func beginningOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func formatDay(t time.Time) string {
	return t.Format("2006-01-02")
}

func backupTableName(prefix, suffix string) string {
	return prefix + "_" + suffix
}

func validateBackupSuffix(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("suffix is empty")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("suffix %q contains invalid character %q", value, r)
	}
	return nil
}
