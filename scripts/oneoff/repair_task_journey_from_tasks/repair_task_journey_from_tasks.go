package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	driverMysql "github.com/go-sql-driver/mysql"
)

type config struct {
	mysqlDSN            string
	orgID               int64
	planID              uint64
	allPlans            bool
	taskCreatedStart    *time.Time
	taskCreatedEnd      *time.Time
	taskCompletedStart  *time.Time
	taskCompletedEnd    *time.Time
	plannedStart        *time.Time
	plannedEnd          *time.Time
	includeDetachedTags bool
	insertMissingEvents bool
	backupSuffix        string
	previewLimit        int
	timeout             time.Duration
	apply               bool
}

type scopeSummary struct {
	TaskAssessments              int64
	CompletedAssessments         int64
	IncompleteAssessments        int64
	Episodes                     int64
	Footprints                   int64
	IncompleteEpisodesToClear    int64
	IncompleteFootprintsToClear  int64
	CompletedFootprintsToRestore int64
	MissingCompletedFootprints   int64
}

func main() {
	cfg := parseFlags()
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

	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("mysql conn: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close mysql conn: %v", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		log.Fatalf("set mysql names: %v", err)
	}

	if err := prepareScope(ctx, conn, cfg); err != nil {
		log.Fatalf("prepare repair scope: %v", err)
	}
	summary, err := loadSummary(ctx, conn, cfg)
	if err != nil {
		log.Fatalf("load summary: %v", err)
	}
	printSummary("preview", summary)
	if err := printPreviewRows(ctx, conn, cfg.previewLimit); err != nil {
		log.Fatalf("print preview: %v", err)
	}
	if summary.TaskAssessments == 0 {
		log.Print("no task-linked journey facts found")
		return
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to repair assessment_episode and behavior_footprint")
		return
	}
	if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
		log.Fatalf("invalid --backup-suffix: %v", err)
	}
	if err := backupMySQL(ctx, conn, cfg.backupSuffix); err != nil {
		log.Fatalf("backup mysql: %v", err)
	}
	if err := repairMySQL(ctx, conn, cfg); err != nil {
		log.Fatalf("repair mysql: %v", err)
	}
	after, err := loadSummary(ctx, conn, cfg)
	if err != nil {
		log.Fatalf("load post-repair summary: %v", err)
	}
	printSummary("after", after)
	log.Print("task-based journey repair completed; rebuild statistics projections and clear stats query cache afterwards")
}

func parseFlags() config {
	var cfg config
	var taskCreatedStartRaw, taskCreatedEndRaw string
	var taskCompletedStartRaw, taskCompletedEndRaw string
	var plannedStartRaw, plannedEndRaw string

	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true&loc=Local")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID")
	flag.Uint64Var(&cfg.planID, "plan-id", 0, "optional assessment plan ID; use --all-plans when omitted")
	flag.BoolVar(&cfg.allPlans, "all-plans", false, "allow repairing all plans under the org when --plan-id is omitted")
	flag.StringVar(&taskCreatedStartRaw, "task-created-start", "", "optional inclusive task created_at start, format 2006-01-02 or 2006-01-02 15:04:05")
	flag.StringVar(&taskCreatedEndRaw, "task-created-end", "", "optional exclusive task created_at end")
	flag.StringVar(&taskCompletedStartRaw, "task-completed-start", "", "optional inclusive task completed_at start")
	flag.StringVar(&taskCompletedEndRaw, "task-completed-end", "", "optional exclusive task completed_at end")
	flag.StringVar(&plannedStartRaw, "planned-start", "", "optional inclusive planned_at start")
	flag.StringVar(&plannedEndRaw, "planned-end", "", "optional exclusive planned_at end")
	flag.BoolVar(&cfg.includeDetachedTags, "include-detached-tags", true, "also find assessments/footprints tagged with properties_json.detached_task_id")
	flag.BoolVar(&cfg.insertMissingEvents, "insert-missing-events", true, "insert missing completed-task assessment service footprints")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of selected task-assessment rows to preview")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.Parse()

	if strings.TrimSpace(cfg.mysqlDSN) == "" {
		log.Fatal("--mysql-dsn is required")
	}
	if cfg.orgID <= 0 {
		log.Fatal("--org-id is required")
	}
	if cfg.planID == 0 && !cfg.allPlans {
		log.Fatal("either --plan-id or --all-plans is required")
	}

	cfg.taskCreatedStart = parseOptionalTimeFlag("--task-created-start", taskCreatedStartRaw)
	cfg.taskCreatedEnd = parseOptionalTimeFlag("--task-created-end", taskCreatedEndRaw)
	cfg.taskCompletedStart = parseOptionalTimeFlag("--task-completed-start", taskCompletedStartRaw)
	cfg.taskCompletedEnd = parseOptionalTimeFlag("--task-completed-end", taskCompletedEndRaw)
	cfg.plannedStart = parseOptionalTimeFlag("--planned-start", plannedStartRaw)
	cfg.plannedEnd = parseOptionalTimeFlag("--planned-end", plannedEndRaw)
	requirePairedRange("--task-created-start", cfg.taskCreatedStart, "--task-created-end", cfg.taskCreatedEnd)
	requirePairedRange("--task-completed-start", cfg.taskCompletedStart, "--task-completed-end", cfg.taskCompletedEnd)
	requirePairedRange("--planned-start", cfg.plannedStart, "--planned-end", cfg.plannedEnd)
	requireBefore("--task-created", cfg.taskCreatedStart, cfg.taskCreatedEnd)
	requireBefore("--task-completed", cfg.taskCompletedStart, cfg.taskCompletedEnd)
	requireBefore("--planned", cfg.plannedStart, cfg.plannedEnd)
	return cfg
}

func openMySQL(dsn string) (*sql.DB, error) {
	c, err := driverMysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	c.ParseTime = true
	c.Loc = time.Local
	if c.Collation == "" {
		c.Collation = "utf8mb4_unicode_ci"
	}
	if c.Params == nil {
		c.Params = make(map[string]string)
	}
	if _, ok := c.Params["time_zone"]; !ok {
		c.Params["time_zone"] = mysqlTimeZoneOffset(time.Now().In(time.Local))
	}
	return sql.Open("mysql", c.FormatDSN())
}

func prepareScope(ctx context.Context, conn *sql.Conn, cfg config) error {
	statements := []string{
		"DROP TEMPORARY TABLE IF EXISTS repair_task_journey_scope",
		"DROP TEMPORARY TABLE IF EXISTS repair_task_journey_footprint_scope",
		createTaskJourneyScopeSQL,
		createFootprintScopeSQL,
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := insertCurrentTaskAssessments(ctx, conn, cfg); err != nil {
		return err
	}
	if cfg.includeDetachedTags {
		if err := insertDetachedTaskAssessmentsByAssessment(ctx, conn, cfg); err != nil {
			return err
		}
		if err := insertDetachedTaskAssessmentsByAnswerSheet(ctx, conn, cfg); err != nil {
			return err
		}
	}
	return prepareFootprintScope(ctx, conn)
}

func insertCurrentTaskAssessments(ctx context.Context, conn *sql.Conn, cfg config) error {
	where, args := buildTaskFilters(cfg, "t")
	query := fmt.Sprintf(`
INSERT IGNORE INTO repair_task_journey_scope (
  task_id, org_id, plan_id, testee_id, task_status, task_completed_at,
  task_assessment_id, assessment_id, answersheet_id, report_id,
  submitted_at, assessment_created_at, report_generated_at, failed_at,
  assessment_status, failure_reason, is_completed, scope_source
)
SELECT
  t.id, t.org_id, t.plan_id, t.testee_id, t.status, t.completed_at,
  t.assessment_id, a.id, a.answer_sheet_id, existing.report_id,
  COALESCE(a.submitted_at, a.created_at), a.created_at,
  CASE WHEN a.interpreted_at IS NOT NULL THEN a.interpreted_at ELSE NULL END,
  a.failed_at, a.status, COALESCE(a.failure_reason, ''),
  CASE
    WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci
     AND t.completed_at IS NOT NULL
     AND t.assessment_id IS NOT NULL
    THEN 1 ELSE 0
  END,
  'task_assessment'
FROM assessment_task t
JOIN assessment a
  ON a.org_id = t.org_id
 AND a.id = t.assessment_id
 AND a.deleted_at IS NULL
LEFT JOIN assessment_episode existing
  ON existing.org_id = a.org_id
 AND existing.answersheet_id = a.answer_sheet_id
 AND existing.deleted_at IS NULL
WHERE %s
  AND t.assessment_id IS NOT NULL`, where)
	_, err := conn.ExecContext(ctx, query, args...)
	return err
}

func insertDetachedTaskAssessmentsByAssessment(ctx context.Context, conn *sql.Conn, cfg config) error {
	where, args := buildTaskFilters(cfg, "t")
	query := fmt.Sprintf(`
INSERT IGNORE INTO repair_task_journey_scope (
  task_id, org_id, plan_id, testee_id, task_status, task_completed_at,
  task_assessment_id, assessment_id, answersheet_id, report_id,
  submitted_at, assessment_created_at, report_generated_at, failed_at,
  assessment_status, failure_reason, is_completed, scope_source
)
SELECT DISTINCT
  t.id, t.org_id, t.plan_id, t.testee_id, t.status, t.completed_at,
  t.assessment_id, a.id, a.answer_sheet_id, existing.report_id,
  COALESCE(a.submitted_at, a.created_at), a.created_at,
  CASE WHEN a.interpreted_at IS NOT NULL THEN a.interpreted_at ELSE NULL END,
  a.failed_at, a.status, COALESCE(a.failure_reason, ''),
  CASE
    WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci
     AND t.completed_at IS NOT NULL
     AND t.assessment_id = a.id
    THEN 1 ELSE 0
  END,
  'detached_footprint_assessment'
FROM assessment_task t
JOIN behavior_footprint bf
  ON bf.org_id = t.org_id
 AND bf.deleted_at IS NULL
 AND bf.assessment_id <> 0
 AND JSON_UNQUOTE(JSON_EXTRACT(bf.properties_json, '$.detached_task_id')) COLLATE utf8mb4_unicode_ci = CAST(t.id AS CHAR) COLLATE utf8mb4_unicode_ci
JOIN assessment a
  ON a.org_id = t.org_id
 AND a.id = bf.assessment_id
 AND a.deleted_at IS NULL
LEFT JOIN assessment_episode existing
  ON existing.org_id = a.org_id
 AND existing.answersheet_id = a.answer_sheet_id
 AND existing.deleted_at IS NULL
WHERE %s`, where)
	_, err := conn.ExecContext(ctx, query, args...)
	return err
}

func insertDetachedTaskAssessmentsByAnswerSheet(ctx context.Context, conn *sql.Conn, cfg config) error {
	where, args := buildTaskFilters(cfg, "t")
	query := fmt.Sprintf(`
INSERT IGNORE INTO repair_task_journey_scope (
  task_id, org_id, plan_id, testee_id, task_status, task_completed_at,
  task_assessment_id, assessment_id, answersheet_id, report_id,
  submitted_at, assessment_created_at, report_generated_at, failed_at,
  assessment_status, failure_reason, is_completed, scope_source
)
SELECT DISTINCT
  t.id, t.org_id, t.plan_id, t.testee_id, t.status, t.completed_at,
  t.assessment_id, a.id, a.answer_sheet_id, existing.report_id,
  COALESCE(a.submitted_at, a.created_at), a.created_at,
  CASE WHEN a.interpreted_at IS NOT NULL THEN a.interpreted_at ELSE NULL END,
  a.failed_at, a.status, COALESCE(a.failure_reason, ''),
  CASE
    WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci
     AND t.completed_at IS NOT NULL
     AND t.assessment_id = a.id
    THEN 1 ELSE 0
  END,
  'detached_footprint_answersheet'
FROM assessment_task t
JOIN behavior_footprint bf
  ON bf.org_id = t.org_id
 AND bf.deleted_at IS NULL
 AND bf.answersheet_id <> 0
 AND JSON_UNQUOTE(JSON_EXTRACT(bf.properties_json, '$.detached_task_id')) COLLATE utf8mb4_unicode_ci = CAST(t.id AS CHAR) COLLATE utf8mb4_unicode_ci
JOIN assessment a
  ON a.org_id = t.org_id
 AND a.answer_sheet_id = bf.answersheet_id
 AND a.deleted_at IS NULL
LEFT JOIN assessment_episode existing
  ON existing.org_id = a.org_id
 AND existing.answersheet_id = a.answer_sheet_id
 AND existing.deleted_at IS NULL
WHERE %s`, where)
	_, err := conn.ExecContext(ctx, query, args...)
	return err
}

func prepareFootprintScope(ctx context.Context, conn *sql.Conn) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "collect footprints by answersheet",
			sql: `
INSERT IGNORE INTO repair_task_journey_footprint_scope (
  footprint_id, task_id, org_id, plan_id, assessment_id, answersheet_id, is_completed
)
SELECT bf.id, s.task_id, s.org_id, s.plan_id, s.assessment_id, s.answersheet_id, s.is_completed
FROM repair_task_journey_scope s
STRAIGHT_JOIN behavior_footprint bf
  ON bf.org_id = s.org_id
 AND bf.answersheet_id = s.answersheet_id
WHERE bf.deleted_at IS NULL
  AND bf.event_name COLLATE utf8mb4_unicode_ci IN (
    'answersheet_submitted' COLLATE utf8mb4_unicode_ci,
    'assessment_created' COLLATE utf8mb4_unicode_ci,
    'report_generated' COLLATE utf8mb4_unicode_ci
  )`,
		},
		{
			name: "collect footprints by assessment",
			sql: `
INSERT IGNORE INTO repair_task_journey_footprint_scope (
  footprint_id, task_id, org_id, plan_id, assessment_id, answersheet_id, is_completed
)
SELECT bf.id, s.task_id, s.org_id, s.plan_id, s.assessment_id, s.answersheet_id, s.is_completed
FROM repair_task_journey_scope s
STRAIGHT_JOIN behavior_footprint bf
  ON bf.org_id = s.org_id
 AND bf.assessment_id = s.assessment_id
WHERE bf.deleted_at IS NULL
  AND bf.event_name COLLATE utf8mb4_unicode_ci IN (
    'answersheet_submitted' COLLATE utf8mb4_unicode_ci,
    'assessment_created' COLLATE utf8mb4_unicode_ci,
    'report_generated' COLLATE utf8mb4_unicode_ci
  )`,
		},
	}
	for _, statement := range statements {
		log.Printf("prepare footprint scope: %s", statement.name)
		if _, err := conn.ExecContext(ctx, statement.sql); err != nil {
			return err
		}
	}
	var count int64
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM repair_task_journey_footprint_scope").Scan(&count); err != nil {
		return err
	}
	log.Printf("prepare footprint scope: matched footprints=%d", count)
	return nil
}

func buildTaskFilters(cfg config, alias string) (string, []any) {
	conditions := []string{fmt.Sprintf("%s.org_id = ?", alias), fmt.Sprintf("%s.deleted_at IS NULL", alias)}
	args := []any{cfg.orgID}
	if cfg.planID != 0 {
		conditions = append(conditions, fmt.Sprintf("%s.plan_id = ?", alias))
		args = append(args, cfg.planID)
	}
	if cfg.taskCreatedStart != nil {
		conditions = append(conditions, fmt.Sprintf("%s.created_at >= ?", alias))
		args = append(args, *cfg.taskCreatedStart)
	}
	if cfg.taskCreatedEnd != nil {
		conditions = append(conditions, fmt.Sprintf("%s.created_at < ?", alias))
		args = append(args, *cfg.taskCreatedEnd)
	}
	if cfg.taskCompletedStart != nil {
		conditions = append(conditions, fmt.Sprintf("%s.completed_at >= ?", alias))
		args = append(args, *cfg.taskCompletedStart)
	}
	if cfg.taskCompletedEnd != nil {
		conditions = append(conditions, fmt.Sprintf("%s.completed_at < ?", alias))
		args = append(args, *cfg.taskCompletedEnd)
	}
	if cfg.plannedStart != nil {
		conditions = append(conditions, fmt.Sprintf("%s.planned_at >= ?", alias))
		args = append(args, *cfg.plannedStart)
	}
	if cfg.plannedEnd != nil {
		conditions = append(conditions, fmt.Sprintf("%s.planned_at < ?", alias))
		args = append(args, *cfg.plannedEnd)
	}
	return strings.Join(conditions, " AND "), args
}

func loadSummary(ctx context.Context, conn *sql.Conn, cfg config) (scopeSummary, error) {
	var summary scopeSummary
	if err := conn.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(CASE WHEN is_completed = 1 THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN is_completed = 0 THEN 1 ELSE 0 END), 0)
FROM repair_task_journey_scope`).Scan(
		&summary.TaskAssessments,
		&summary.CompletedAssessments,
		&summary.IncompleteAssessments,
	); err != nil {
		return summary, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM assessment_episode e
JOIN repair_task_journey_scope s ON s.org_id = e.org_id AND s.answersheet_id = e.answersheet_id
WHERE e.deleted_at IS NULL`).Scan(&summary.Episodes); err != nil {
		return summary, err
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM repair_task_journey_footprint_scope").Scan(&summary.Footprints); err != nil {
		return summary, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM assessment_episode e
JOIN repair_task_journey_scope s ON s.org_id = e.org_id AND s.answersheet_id = e.answersheet_id
WHERE s.is_completed = 0
  AND e.deleted_at IS NULL
  AND (e.entry_id IS NOT NULL OR e.clinician_id IS NOT NULL OR e.attributed_intake_at IS NOT NULL)`).Scan(&summary.IncompleteEpisodesToClear); err != nil {
		return summary, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM behavior_footprint bf
JOIN repair_task_journey_footprint_scope fs ON fs.footprint_id = bf.id
WHERE fs.is_completed = 0
  AND bf.deleted_at IS NULL
  AND (bf.entry_id <> 0 OR bf.clinician_id <> 0 OR bf.source_clinician_id <> 0)`).Scan(&summary.IncompleteFootprintsToClear); err != nil {
		return summary, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM behavior_footprint bf
JOIN repair_task_journey_footprint_scope fs ON fs.footprint_id = bf.id
WHERE fs.is_completed = 1
  AND bf.deleted_at IS NULL
  AND JSON_EXTRACT(bf.properties_json, '$.detached_task_id') IS NOT NULL`).Scan(&summary.CompletedFootprintsToRestore); err != nil {
		return summary, err
	}
	if cfg.insertMissingEvents {
		if err := conn.QueryRowContext(ctx, missingCompletedFootprintsCountSQL).Scan(&summary.MissingCompletedFootprints); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func printSummary(stage string, summary scopeSummary) {
	log.Printf("%s summary: task_assessments=%d completed=%d incomplete=%d episodes=%d footprints=%d incomplete_episodes_to_clear=%d incomplete_footprints_to_clear=%d completed_footprints_to_restore=%d missing_completed_footprints=%d",
		stage,
		summary.TaskAssessments,
		summary.CompletedAssessments,
		summary.IncompleteAssessments,
		summary.Episodes,
		summary.Footprints,
		summary.IncompleteEpisodesToClear,
		summary.IncompleteFootprintsToClear,
		summary.CompletedFootprintsToRestore,
		summary.MissingCompletedFootprints,
	)
}

func printPreviewRows(ctx context.Context, conn *sql.Conn, limit int) error {
	if limit <= 0 {
		return nil
	}
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`
SELECT task_id, plan_id, testee_id, task_status, is_completed, assessment_id, answersheet_id, scope_source
FROM repair_task_journey_scope
ORDER BY is_completed, task_id, assessment_id
LIMIT %d`, limit))
	if err != nil {
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Panicln("rows close err:", err)
		}
	}()
	for rows.Next() {
		var taskID, planID, testeeID, assessmentID, answerSheetID uint64
		var taskStatus, source string
		var completed int
		if err := rows.Scan(&taskID, &planID, &testeeID, &taskStatus, &completed, &assessmentID, &answerSheetID, &source); err != nil {
			return err
		}
		log.Printf("preview task=%d plan=%d testee=%d status=%s completed=%d assessment=%d answersheet=%d source=%s",
			taskID, planID, testeeID, taskStatus, completed, assessmentID, answerSheetID, source)
	}
	return rows.Err()
}

func backupMySQL(ctx context.Context, conn *sql.Conn, suffix string) error {
	statements := []string{
		fmt.Sprintf("CREATE TABLE repair_bak_task_journey_scope_%s AS SELECT * FROM repair_task_journey_scope", suffix),
		fmt.Sprintf(`CREATE TABLE repair_bak_task_journey_episode_%s AS
SELECT e.* FROM assessment_episode e
JOIN repair_task_journey_scope s ON s.org_id = e.org_id AND s.answersheet_id = e.answersheet_id
`, suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_task_journey_footprint_%s LIKE behavior_footprint", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO repair_bak_task_journey_footprint_%s
SELECT bf.* FROM behavior_footprint bf
JOIN repair_task_journey_footprint_scope fs ON fs.footprint_id = bf.id`, suffix),
	}
	for i, statement := range statements {
		log.Printf("backup mysql step %d/%d", i+1, len(statements))
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func repairMySQL(ctx context.Context, conn *sql.Conn, cfg config) (err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("rollback mysql repair: %v", rollbackErr)
			}
		}
	}()

	statements := []struct {
		name string
		sql  string
	}{
		{name: "restore completed assessment episodes", sql: restoreCompletedEpisodesSQL},
		{name: "insert missing completed assessment episodes", sql: insertMissingCompletedEpisodesSQL},
		{name: "clear incomplete episode attribution", sql: clearIncompleteEpisodeAttributionSQL},
		{name: "clear incomplete footprint attribution", sql: clearIncompleteFootprintAttributionSQL},
		{name: "restore completed footprint attribution", sql: restoreCompletedFootprintAttributionSQL},
	}
	if cfg.insertMissingEvents {
		statements = append(statements,
			struct {
				name string
				sql  string
			}{name: "insert missing completed footprints", sql: insertMissingCompletedFootprintsSQL},
		)
	}

	for _, statement := range statements {
		log.Printf("repair mysql step: %s", statement.name)
		result, err := tx.ExecContext(ctx, statement.sql)
		if err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
		if affected, err := result.RowsAffected(); err == nil {
			log.Printf("%s affected rows=%d", statement.name, affected)
		}
	}
	return tx.Commit()
}

func validateBackupSuffix(s string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(s) {
		return fmt.Errorf("must match ^[A-Za-z0-9_]+$")
	}
	return nil
}

func parseOptionalTimeFlag(name, raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	layouts := []string{time.DateTime, "2006-01-02 15:04:05.000", "2006-01-02"}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			if layout == "2006-01-02" {
				t = normalizeLocalDay(t)
			}
			return &t
		}
		lastErr = err
	}
	log.Fatalf("invalid %s: %v", name, lastErr)
	return nil
}

func requirePairedRange(startName string, start *time.Time, endName string, end *time.Time) {
	if (start == nil) != (end == nil) {
		log.Fatalf("%s and %s must be provided together", startName, endName)
	}
}

func requireBefore(label string, start *time.Time, end *time.Time) {
	if start != nil && end != nil && !start.Before(*end) {
		log.Fatalf("%s range must satisfy start < end", label)
	}
}

func normalizeLocalDay(value time.Time) time.Time {
	local := value.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

func mysqlTimeZoneOffset(value time.Time) string {
	_, offset := value.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("'%s%02d:%02d'", sign, hours, minutes)
}

const createTaskJourneyScopeSQL = `
CREATE TEMPORARY TABLE repair_task_journey_scope (
  task_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  plan_id BIGINT UNSIGNED NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  task_status VARCHAR(50) NOT NULL,
  task_completed_at DATETIME(3) NULL,
  task_assessment_id BIGINT UNSIGNED NULL,
  assessment_id BIGINT UNSIGNED NOT NULL,
  answersheet_id BIGINT UNSIGNED NOT NULL,
  report_id BIGINT UNSIGNED NULL,
  submitted_at DATETIME(3) NOT NULL,
  assessment_created_at DATETIME(3) NULL,
  report_generated_at DATETIME(3) NULL,
  failed_at DATETIME(3) NULL,
  assessment_status VARCHAR(50) NOT NULL,
  failure_reason VARCHAR(500) NULL,
  is_completed TINYINT NOT NULL,
  scope_source VARCHAR(50) NOT NULL,
  PRIMARY KEY (task_id, assessment_id),
  KEY idx_task_journey_scope_answersheet (org_id, answersheet_id),
  KEY idx_task_journey_scope_assessment (org_id, assessment_id),
  KEY idx_task_journey_scope_completed (is_completed)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const createFootprintScopeSQL = `
CREATE TEMPORARY TABLE repair_task_journey_footprint_scope (
  footprint_id VARCHAR(128) NOT NULL PRIMARY KEY,
  task_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  plan_id BIGINT UNSIGNED NOT NULL,
  assessment_id BIGINT UNSIGNED NOT NULL,
  answersheet_id BIGINT UNSIGNED NOT NULL,
  is_completed TINYINT NOT NULL,
  KEY idx_task_journey_footprint_task (task_id),
  KEY idx_task_journey_footprint_completed (is_completed)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const restoreCompletedEpisodesSQL = `
UPDATE assessment_episode e
JOIN repair_task_journey_scope s
  ON s.org_id = e.org_id
 AND s.answersheet_id = e.answersheet_id
SET
  e.testee_id = s.testee_id,
  e.assessment_id = s.assessment_id,
  e.report_id = COALESCE(e.report_id, s.report_id),
  e.submitted_at = s.submitted_at,
  e.assessment_created_at = s.assessment_created_at,
  e.report_generated_at = s.report_generated_at,
  e.failed_at = s.failed_at,
  e.status = CASE
    WHEN s.failed_at IS NOT NULL OR s.assessment_status COLLATE utf8mb4_unicode_ci = 'failed' COLLATE utf8mb4_unicode_ci THEN 'failed'
    WHEN s.report_generated_at IS NOT NULL OR s.assessment_status COLLATE utf8mb4_unicode_ci = 'interpreted' COLLATE utf8mb4_unicode_ci THEN 'completed'
    ELSE 'active'
  END,
  e.failure_reason = COALESCE(s.failure_reason, ''),
  e.deleted_at = NULL,
  e.updated_at = NOW(3)
WHERE s.is_completed = 1`

const insertMissingCompletedEpisodesSQL = `
INSERT INTO assessment_episode (
  episode_id, org_id, entry_id, clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, attributed_intake_at, submitted_at,
  assessment_created_at, report_generated_at, failed_at, status, failure_reason,
  created_at, updated_at
)
SELECT
  s.answersheet_id,
  s.org_id,
  NULL,
  NULL,
  s.testee_id,
  s.answersheet_id,
  s.assessment_id,
  s.report_id,
  NULL,
  s.submitted_at,
  s.assessment_created_at,
  s.report_generated_at,
  s.failed_at,
  CASE
    WHEN s.failed_at IS NOT NULL OR s.assessment_status COLLATE utf8mb4_unicode_ci = 'failed' COLLATE utf8mb4_unicode_ci THEN 'failed'
    WHEN s.report_generated_at IS NOT NULL OR s.assessment_status COLLATE utf8mb4_unicode_ci = 'interpreted' COLLATE utf8mb4_unicode_ci THEN 'completed'
    ELSE 'active'
  END,
  COALESCE(s.failure_reason, ''),
  NOW(3),
  NOW(3)
FROM repair_task_journey_scope s
LEFT JOIN assessment_episode e
  ON e.org_id = s.org_id
 AND e.answersheet_id = s.answersheet_id
 AND e.deleted_at IS NULL
WHERE s.is_completed = 1
  AND e.answersheet_id IS NULL`

const clearIncompleteEpisodeAttributionSQL = `
UPDATE assessment_episode e
JOIN repair_task_journey_scope s
  ON s.org_id = e.org_id
 AND s.answersheet_id = e.answersheet_id
SET
  e.entry_id = NULL,
  e.clinician_id = NULL,
  e.attributed_intake_at = NULL,
  e.updated_at = NOW(3)
WHERE s.is_completed = 0
  AND e.deleted_at IS NULL`

const clearIncompleteFootprintAttributionSQL = `
UPDATE behavior_footprint bf
JOIN repair_task_journey_footprint_scope fs ON fs.footprint_id = bf.id
SET
  bf.entry_id = 0,
  bf.clinician_id = 0,
  bf.source_clinician_id = 0,
  bf.properties_json = JSON_SET(
    COALESCE(bf.properties_json, JSON_OBJECT()),
    '$.repair_source', 'repair_task_journey_from_tasks',
    '$.task_status', 'not_completed',
    '$.detached_plan_id', fs.plan_id,
    '$.detached_task_id', fs.task_id
  ),
  bf.updated_at = NOW(3)
WHERE fs.is_completed = 0
  AND bf.deleted_at IS NULL`

const restoreCompletedFootprintAttributionSQL = `
UPDATE behavior_footprint bf
JOIN repair_task_journey_footprint_scope fs ON fs.footprint_id = bf.id
LEFT JOIN assessment_episode e
  ON e.org_id = fs.org_id
 AND e.answersheet_id = fs.answersheet_id
 AND e.deleted_at IS NULL
SET
  bf.entry_id = COALESCE(e.entry_id, 0),
  bf.clinician_id = COALESCE(e.clinician_id, 0),
  bf.source_clinician_id = 0,
  bf.properties_json = JSON_REMOVE(
    COALESCE(bf.properties_json, JSON_OBJECT()),
    '$.task_status',
    '$.detached_plan_id',
    '$.detached_task_id'
  ),
  bf.updated_at = NOW(3)
WHERE fs.is_completed = 1
  AND bf.deleted_at IS NULL`

const missingCompletedFootprintsCountSQL = `
SELECT COALESCE(SUM(missing_count), 0)
FROM (
  SELECT COUNT(*) AS missing_count
  FROM repair_task_journey_scope s
  WHERE s.is_completed = 1
    AND s.submitted_at IS NOT NULL
    AND NOT EXISTS (
      SELECT 1 FROM behavior_footprint bf
      WHERE bf.org_id = s.org_id
        AND bf.deleted_at IS NULL
        AND bf.event_name COLLATE utf8mb4_unicode_ci = 'answersheet_submitted' COLLATE utf8mb4_unicode_ci
        AND bf.answersheet_id = s.answersheet_id
    )
  UNION ALL
  SELECT COUNT(*)
  FROM repair_task_journey_scope s
  WHERE s.is_completed = 1
    AND s.assessment_created_at IS NOT NULL
    AND NOT EXISTS (
      SELECT 1 FROM behavior_footprint bf
      WHERE bf.org_id = s.org_id
        AND bf.deleted_at IS NULL
        AND bf.event_name COLLATE utf8mb4_unicode_ci = 'assessment_created' COLLATE utf8mb4_unicode_ci
        AND bf.assessment_id = s.assessment_id
    )
  UNION ALL
  SELECT COUNT(*)
  FROM repair_task_journey_scope s
  WHERE s.is_completed = 1
    AND s.report_generated_at IS NOT NULL
    AND NOT EXISTS (
      SELECT 1 FROM behavior_footprint bf
      WHERE bf.org_id = s.org_id
        AND bf.deleted_at IS NULL
        AND bf.event_name COLLATE utf8mb4_unicode_ci = 'report_generated' COLLATE utf8mb4_unicode_ci
        AND bf.assessment_id = s.assessment_id
    )
) missing`

const insertMissingCompletedFootprintsSQL = `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('task:', s.task_id, ':answersheet_submitted'),
  s.org_id,
  'answersheet',
  s.answersheet_id,
  'testee',
  s.testee_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  s.testee_id,
  s.answersheet_id,
  s.assessment_id,
  COALESCE(s.report_id, 0),
  'answersheet_submitted',
  s.submitted_at,
  JSON_OBJECT('repair_source', 'repair_task_journey_from_tasks', 'task_id', s.task_id, 'plan_id', s.plan_id),
  NOW(3),
  NOW(3)
FROM repair_task_journey_scope s
LEFT JOIN assessment_episode e ON e.org_id = s.org_id AND e.answersheet_id = s.answersheet_id AND e.deleted_at IS NULL
WHERE s.is_completed = 1
  AND s.submitted_at IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM behavior_footprint bf
    WHERE bf.org_id = s.org_id
      AND bf.deleted_at IS NULL
      AND bf.event_name COLLATE utf8mb4_unicode_ci = 'answersheet_submitted' COLLATE utf8mb4_unicode_ci
      AND bf.answersheet_id = s.answersheet_id
  )
UNION ALL
SELECT
  CONCAT('task:', s.task_id, ':assessment_created'),
  s.org_id,
  'assessment',
  s.assessment_id,
  'testee',
  s.testee_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  s.testee_id,
  s.answersheet_id,
  s.assessment_id,
  COALESCE(s.report_id, 0),
  'assessment_created',
  s.assessment_created_at,
  JSON_OBJECT('repair_source', 'repair_task_journey_from_tasks', 'task_id', s.task_id, 'plan_id', s.plan_id),
  NOW(3),
  NOW(3)
FROM repair_task_journey_scope s
LEFT JOIN assessment_episode e ON e.org_id = s.org_id AND e.answersheet_id = s.answersheet_id AND e.deleted_at IS NULL
WHERE s.is_completed = 1
  AND s.assessment_created_at IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM behavior_footprint bf
    WHERE bf.org_id = s.org_id
      AND bf.deleted_at IS NULL
      AND bf.event_name COLLATE utf8mb4_unicode_ci = 'assessment_created' COLLATE utf8mb4_unicode_ci
      AND bf.assessment_id = s.assessment_id
  )
UNION ALL
SELECT
  CONCAT('task:', s.task_id, ':report_generated'),
  s.org_id,
  'report',
  COALESCE(s.report_id, 0),
  'assessment',
  s.assessment_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  s.testee_id,
  s.answersheet_id,
  s.assessment_id,
  COALESCE(s.report_id, 0),
  'report_generated',
  s.report_generated_at,
  JSON_OBJECT('repair_source', 'repair_task_journey_from_tasks', 'task_id', s.task_id, 'plan_id', s.plan_id),
  NOW(3),
  NOW(3)
FROM repair_task_journey_scope s
LEFT JOIN assessment_episode e ON e.org_id = s.org_id AND e.answersheet_id = s.answersheet_id AND e.deleted_at IS NULL
WHERE s.is_completed = 1
  AND s.report_generated_at IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM behavior_footprint bf
    WHERE bf.org_id = s.org_id
      AND bf.deleted_at IS NULL
      AND bf.event_name COLLATE utf8mb4_unicode_ci = 'report_generated' COLLATE utf8mb4_unicode_ci
      AND bf.assessment_id = s.assessment_id
  )`
