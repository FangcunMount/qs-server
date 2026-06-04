package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type config struct {
	mysqlDSN            string
	orgID               int64
	allOrgs             bool
	collapsedDateRaw    string
	targetStartDateRaw  string
	targetEndDateRaw    string
	testeeSource        string
	planIDs             uint64CSVFlag
	timeout             time.Duration
	apply               bool
	skipBackup          bool
	backupSuffix        string
	previewLimit        int
	rewriteTaskOpenAt   bool
	rewriteTaskExpireAt bool
	rewriteScoreTimes   bool
	refreshTesteeStats  bool
}

type uint64CSVFlag []uint64

func (f *uint64CSVFlag) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*f))
	for _, item := range *f {
		parts = append(parts, strconv.FormatUint(item, 10))
	}
	return strings.Join(parts, ",")
}

func (f *uint64CSVFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parsed, err := strconv.ParseUint(item, 10, 64)
		if err != nil || parsed == 0 {
			return fmt.Errorf("invalid unsigned integer %q", item)
		}
		*f = append(*f, parsed)
	}
	return nil
}

type scopeSummary struct {
	Assessments     int64
	Plans           int64
	Testees         int64
	AssessmentScore int64
	MinTargetDate   sql.NullString
	MaxTargetDate   sql.NullString
}

type fieldChangeSummary struct {
	AssessmentCreatedAt   int64
	AssessmentSubmittedAt int64
	AssessmentInterpreted int64
	AssessmentFailedAt    int64
	AssessmentUpdatedAt   int64
	TaskOpenAt            int64
	TaskCompletedAt       int64
	TaskExpireAt          int64
	TaskUpdatedAt         int64
}

type dailySummary struct {
	TargetDate  string
	Assessments int64
	Plans       int64
	Testees     int64
}

type previewRow struct {
	AssessmentID uint64
	TaskID       uint64
	OrgID        int64
	PlanID       uint64
	TesteeID     uint64
	TargetDate   string
	OldCreatedAt sql.NullTime
	NewCreatedAt sql.NullTime
	OldSubmitted sql.NullTime
	NewSubmitted sql.NullTime
	OldReported  sql.NullTime
	NewReported  sql.NullTime
	OldOpenAt    sql.NullTime
	NewOpenAt    sql.NullTime
	OldCompleted sql.NullTime
	NewCompleted sql.NullTime
}

type statementResult struct {
	Name     string
	Affected int64
}

func main() {
	cfg := parseFlags()
	_, collapsedRaw := mustParseDate("collapsed-date", cfg.collapsedDateRaw)
	_, targetStartRaw := mustParseDate("target-start-date", cfg.targetStartDateRaw)
	_, targetEndRaw := mustParseDate("target-end-date", cfg.targetEndDateRaw)
	if !dateRawBefore(targetStartRaw, targetEndRaw) {
		log.Fatal("--target-end-date must be later than --target-start-date")
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

	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("mysql conn: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close mysql conn: %v", err)
		}
	}()

	if err := pingAndPrepare(ctx, conn); err != nil {
		log.Fatalf("prepare mysql: %v", err)
	}
	if err := prepareScope(ctx, conn, cfg, collapsedRaw, targetStartRaw, targetEndRaw); err != nil {
		log.Fatalf("prepare rewrite scope: %v", err)
	}

	summary, err := loadScopeSummary(ctx, conn)
	if err != nil {
		log.Fatalf("load scope summary: %v", err)
	}
	log.Printf("scope: %s collapsed_date=%s target=[%s,%s) testee_source=%q plan_ids=%s apply=%v backup=%v",
		scopeDescription(cfg), collapsedRaw, targetStartRaw, targetEndRaw, cfg.testeeSource, planScopeDescription(cfg.planIDs), cfg.apply, !cfg.skipBackup)
	log.Printf("candidate assessments=%d plans=%d testees=%d assessment_scores=%d target_date_min=%s target_date_max=%s",
		summary.Assessments, summary.Plans, summary.Testees, summary.AssessmentScore, nullString(summary.MinTargetDate), nullString(summary.MaxTargetDate))
	if summary.Assessments == 0 {
		log.Print("scope is empty; nothing to rewrite")
		return
	}

	fieldSummary, err := loadFieldChangeSummary(ctx, conn, collapsedRaw)
	if err != nil {
		log.Fatalf("load field change summary: %v", err)
	}
	log.Printf("fields on collapsed date: assessment.created_at=%d submitted_at=%d interpreted_at=%d failed_at=%d updated_at=%d task.open_at=%d task.completed_at=%d task.expire_at=%d task.updated_at=%d",
		fieldSummary.AssessmentCreatedAt, fieldSummary.AssessmentSubmittedAt, fieldSummary.AssessmentInterpreted, fieldSummary.AssessmentFailedAt, fieldSummary.AssessmentUpdatedAt,
		fieldSummary.TaskOpenAt, fieldSummary.TaskCompletedAt, fieldSummary.TaskExpireAt, fieldSummary.TaskUpdatedAt)

	dailyRows, err := loadDailySummaries(ctx, conn)
	if err != nil {
		log.Fatalf("load daily summaries: %v", err)
	}
	for _, row := range dailyRows {
		log.Printf("target day %s assessments=%d plans=%d testees=%d", row.TargetDate, row.Assessments, row.Plans, row.Testees)
	}

	previewRows, err := loadPreviewRows(ctx, conn, cfg.previewLimit)
	if err != nil {
		log.Fatalf("load preview rows: %v", err)
	}
	printPreviewRows(previewRows)

	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to rewrite source timestamps")
		return
	}
	if !cfg.skipBackup {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
		if err := backupSourceRows(ctx, conn, cfg); err != nil {
			log.Fatalf("backup source rows: %v", err)
		}
	}

	results, err := applyRewrite(ctx, conn, cfg, collapsedRaw)
	if err != nil {
		log.Fatalf("apply rewrite: %v", err)
	}
	for _, item := range results {
		log.Printf("applied %-24s affected_rows=%d", item.Name, item.Affected)
	}
	log.Print("seeddata assessment timestamp rewrite completed; rebuild statistics facts and aggregates for the same window next")
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to repair; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "repair all organizations")
	flag.StringVar(&cfg.collapsedDateRaw, "collapsed-date", "", "bad day where earlier seeddata assessments were concentrated, format YYYY-MM-DD")
	flag.StringVar(&cfg.targetStartDateRaw, "target-start-date", "", "inclusive lower bound for assessment_task.planned_at target date, format YYYY-MM-DD")
	flag.StringVar(&cfg.targetEndDateRaw, "target-end-date", "", "exclusive upper bound for assessment_task.planned_at target date, format YYYY-MM-DD")
	flag.StringVar(&cfg.testeeSource, "testee-source", "daily_simulation", "testee.source safety filter; empty string disables this filter")
	flag.Var(&cfg.planIDs, "plan-id", "plan ID to repair; repeat or comma-separate. Empty means all matching plans")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout, e.g. 30m, 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup table creation before applying changes")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of candidate rows to preview in dry-run")
	flag.BoolVar(&cfg.rewriteTaskOpenAt, "rewrite-task-open-at", true, "rewrite assessment_task.open_at when it is on --collapsed-date")
	flag.BoolVar(&cfg.rewriteTaskExpireAt, "rewrite-task-expire-at", false, "rewrite assessment_task.expire_at when it is on --collapsed-date")
	flag.BoolVar(&cfg.rewriteScoreTimes, "rewrite-score-times", true, "rewrite assessment_score created_at/updated_at for affected assessments")
	flag.BoolVar(&cfg.refreshTesteeStats, "refresh-testee-stats", true, "refresh denormalized testee assessment stats for affected testees")
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
	if strings.TrimSpace(cfg.collapsedDateRaw) == "" {
		log.Fatal("--collapsed-date is required")
	}
	if strings.TrimSpace(cfg.targetStartDateRaw) == "" {
		log.Fatal("--target-start-date is required")
	}
	if strings.TrimSpace(cfg.targetEndDateRaw) == "" {
		log.Fatal("--target-end-date is required")
	}
	if cfg.previewLimit < 0 {
		log.Fatal("--preview-limit must be greater than or equal to 0")
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
	db, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return nil, err
	}
	return db, nil
}

func pingAndPrepare(ctx context.Context, conn *sql.Conn) error {
	if err := conn.PingContext(ctx); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func prepareScope(ctx context.Context, conn *sql.Conn, cfg config, collapsedDate, targetStart, targetEnd string) error {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS seeddata_assessment_time_rewrite_scope`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE seeddata_assessment_time_rewrite_scope (
  assessment_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  task_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  plan_id BIGINT UNSIGNED NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  questionnaire_code VARCHAR(100) NOT NULL,
  target_date DATE NOT NULL,
  old_assessment_created_at DATETIME(3) NULL,
  new_assessment_created_at DATETIME(3) NULL,
  old_assessment_updated_at DATETIME(3) NULL,
  new_assessment_updated_at DATETIME(3) NULL,
  old_submitted_at DATETIME(3) NULL,
  new_submitted_at DATETIME(3) NULL,
  old_interpreted_at DATETIME(3) NULL,
  new_interpreted_at DATETIME(3) NULL,
  old_failed_at DATETIME(3) NULL,
  new_failed_at DATETIME(3) NULL,
  old_task_open_at DATETIME(3) NULL,
  new_task_open_at DATETIME(3) NULL,
  old_task_completed_at DATETIME(3) NULL,
  new_task_completed_at DATETIME(3) NULL,
  old_task_expire_at DATETIME(3) NULL,
  new_task_expire_at DATETIME(3) NULL,
  old_task_updated_at DATETIME(3) NULL,
  new_task_updated_at DATETIME(3) NULL,
  KEY idx_seeddata_scope_org_plan (org_id, plan_id),
  KEY idx_seeddata_scope_target_date (target_date),
  KEY idx_seeddata_scope_testee (org_id, testee_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
SET
  @seed_rewrite_collapsed_date = CAST(? AS DATE),
  @seed_rewrite_target_start = CAST(? AS DATE),
  @seed_rewrite_target_end = CAST(? AS DATE)`, collapsedDate, targetStart, targetEnd); err != nil {
		return err
	}

	query, args := buildScopeInsertSQL(cfg)
	_, err := conn.ExecContext(ctx, query, args...)
	return err
}

func buildScopeInsertSQL(cfg config) (string, []any) {
	where := `
WHERE a.deleted_at IS NULL
  AND t.deleted_at IS NULL
  AND te.deleted_at IS NULL
  AND a.origin_type = 'plan'
  AND DATE(t.planned_at) >= @seed_rewrite_target_start
  AND DATE(t.planned_at) < @seed_rewrite_target_end
  AND DATE(t.planned_at) <> @seed_rewrite_collapsed_date
  AND (
    DATE(a.created_at) = @seed_rewrite_collapsed_date
    OR (a.updated_at IS NOT NULL AND DATE(a.updated_at) = @seed_rewrite_collapsed_date)
    OR (a.submitted_at IS NOT NULL AND DATE(a.submitted_at) = @seed_rewrite_collapsed_date)
    OR (a.interpreted_at IS NOT NULL AND DATE(a.interpreted_at) = @seed_rewrite_collapsed_date)
    OR (a.failed_at IS NOT NULL AND DATE(a.failed_at) = @seed_rewrite_collapsed_date)
    OR (t.updated_at IS NOT NULL AND DATE(t.updated_at) = @seed_rewrite_collapsed_date)
    OR (t.completed_at IS NOT NULL AND DATE(t.completed_at) = @seed_rewrite_collapsed_date)`
	if cfg.rewriteTaskOpenAt {
		where += `
    OR (t.open_at IS NOT NULL AND DATE(t.open_at) = @seed_rewrite_collapsed_date)`
	}
	if cfg.rewriteTaskExpireAt {
		where += `
    OR (t.expire_at IS NOT NULL AND DATE(t.expire_at) = @seed_rewrite_collapsed_date)`
	}
	where += `
  )`

	args := make([]any, 0, 4+len(cfg.planIDs))
	if !cfg.allOrgs {
		where += `
  AND a.org_id = ?`
		args = append(args, cfg.orgID)
	}
	if strings.TrimSpace(cfg.testeeSource) != "" {
		where += `
  AND te.source = ?`
		args = append(args, strings.TrimSpace(cfg.testeeSource))
	}
	if len(cfg.planIDs) > 0 {
		where += `
  AND t.plan_id IN (` + placeholders(len(cfg.planIDs)) + `)`
		for _, id := range cfg.planIDs {
			args = append(args, id)
		}
	}

	query := `
INSERT INTO seeddata_assessment_time_rewrite_scope (
  assessment_id, task_id, org_id, plan_id, testee_id, questionnaire_code, target_date,
  old_assessment_created_at, new_assessment_created_at,
  old_assessment_updated_at, new_assessment_updated_at,
  old_submitted_at, new_submitted_at,
  old_interpreted_at, new_interpreted_at,
  old_failed_at, new_failed_at,
  old_task_open_at, new_task_open_at,
  old_task_completed_at, new_task_completed_at,
  old_task_expire_at, new_task_expire_at,
  old_task_updated_at, new_task_updated_at
)
SELECT
  a.id,
  t.id,
  a.org_id,
  t.plan_id,
  a.testee_id,
  a.questionnaire_code,
  DATE(t.planned_at) AS target_date,
  a.created_at,
  CASE WHEN DATE(a.created_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(a.created_at)) ELSE a.created_at END,
  a.updated_at,
  CASE WHEN a.updated_at IS NOT NULL AND DATE(a.updated_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(a.updated_at)) ELSE a.updated_at END,
  a.submitted_at,
  CASE WHEN a.submitted_at IS NOT NULL AND DATE(a.submitted_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(a.submitted_at)) ELSE a.submitted_at END,
  a.interpreted_at,
  CASE WHEN a.interpreted_at IS NOT NULL AND DATE(a.interpreted_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(a.interpreted_at)) ELSE a.interpreted_at END,
  a.failed_at,
  CASE WHEN a.failed_at IS NOT NULL AND DATE(a.failed_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(a.failed_at)) ELSE a.failed_at END,
  t.open_at,
  CASE WHEN t.open_at IS NOT NULL AND DATE(t.open_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(t.open_at)) ELSE t.open_at END,
  t.completed_at,
  CASE WHEN t.completed_at IS NOT NULL AND DATE(t.completed_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(t.completed_at)) ELSE t.completed_at END,
  t.expire_at,
  CASE WHEN t.expire_at IS NOT NULL AND DATE(t.expire_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(t.expire_at)) ELSE t.expire_at END,
  t.updated_at,
  CASE WHEN t.updated_at IS NOT NULL AND DATE(t.updated_at) = @seed_rewrite_collapsed_date THEN TIMESTAMP(DATE(t.planned_at), TIME(t.updated_at)) ELSE t.updated_at END
FROM assessment_task t
INNER JOIN assessment a ON a.id = t.assessment_id AND a.org_id = t.org_id
INNER JOIN testee te ON te.id = a.testee_id AND te.org_id = a.org_id` + where
	return query, args
}

func loadScopeSummary(ctx context.Context, conn *sql.Conn) (scopeSummary, error) {
	var summary scopeSummary
	if err := conn.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS assessments,
  COUNT(DISTINCT plan_id) AS plans,
  COUNT(DISTINCT testee_id) AS testees,
  CAST(MIN(target_date) AS CHAR) AS min_target_date,
  CAST(MAX(target_date) AS CHAR) AS max_target_date
FROM seeddata_assessment_time_rewrite_scope`).Scan(
		&summary.Assessments,
		&summary.Plans,
		&summary.Testees,
		&summary.MinTargetDate,
		&summary.MaxTargetDate,
	); err != nil {
		return scopeSummary{}, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM assessment_score s
INNER JOIN seeddata_assessment_time_rewrite_scope r ON r.assessment_id = s.assessment_id
WHERE s.deleted_at IS NULL`).Scan(&summary.AssessmentScore); err != nil {
		return scopeSummary{}, err
	}
	return summary, nil
}

func loadFieldChangeSummary(ctx context.Context, conn *sql.Conn, collapsedDate string) (fieldChangeSummary, error) {
	var item fieldChangeSummary
	if err := conn.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN DATE(old_assessment_created_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_submitted_at IS NOT NULL AND DATE(old_submitted_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_interpreted_at IS NOT NULL AND DATE(old_interpreted_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_failed_at IS NOT NULL AND DATE(old_failed_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_assessment_updated_at IS NOT NULL AND DATE(old_assessment_updated_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_task_open_at IS NOT NULL AND DATE(old_task_open_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_task_completed_at IS NOT NULL AND DATE(old_task_completed_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_task_expire_at IS NOT NULL AND DATE(old_task_expire_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN old_task_updated_at IS NOT NULL AND DATE(old_task_updated_at) = CAST(? AS DATE) THEN 1 ELSE 0 END), 0)
FROM seeddata_assessment_time_rewrite_scope`,
		collapsedDate, collapsedDate, collapsedDate, collapsedDate, collapsedDate, collapsedDate, collapsedDate, collapsedDate, collapsedDate,
	).Scan(
		&item.AssessmentCreatedAt,
		&item.AssessmentSubmittedAt,
		&item.AssessmentInterpreted,
		&item.AssessmentFailedAt,
		&item.AssessmentUpdatedAt,
		&item.TaskOpenAt,
		&item.TaskCompletedAt,
		&item.TaskExpireAt,
		&item.TaskUpdatedAt,
	); err != nil {
		return fieldChangeSummary{}, err
	}
	return item, nil
}

func loadDailySummaries(ctx context.Context, conn *sql.Conn) ([]dailySummary, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT CAST(target_date AS CHAR), COUNT(*), COUNT(DISTINCT plan_id), COUNT(DISTINCT testee_id)
FROM seeddata_assessment_time_rewrite_scope
GROUP BY target_date
ORDER BY target_date`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("close daily summary rows: %v", err)
		}
	}()
	var results []dailySummary
	for rows.Next() {
		var item dailySummary
		if err := rows.Scan(&item.TargetDate, &item.Assessments, &item.Plans, &item.Testees); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func loadPreviewRows(ctx context.Context, conn *sql.Conn, limit int) ([]previewRow, error) {
	if limit == 0 {
		return nil, nil
	}
	rows, err := conn.QueryContext(ctx, `
SELECT
  assessment_id, task_id, org_id, plan_id, testee_id, CAST(target_date AS CHAR),
  old_assessment_created_at, new_assessment_created_at,
  old_submitted_at, new_submitted_at,
  old_interpreted_at, new_interpreted_at,
  old_task_open_at, new_task_open_at,
  old_task_completed_at, new_task_completed_at
FROM seeddata_assessment_time_rewrite_scope
ORDER BY target_date, plan_id, testee_id, task_id
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("close preview rows: %v", err)
		}
	}()
	results := make([]previewRow, 0, limit)
	for rows.Next() {
		var item previewRow
		if err := rows.Scan(
			&item.AssessmentID,
			&item.TaskID,
			&item.OrgID,
			&item.PlanID,
			&item.TesteeID,
			&item.TargetDate,
			&item.OldCreatedAt,
			&item.NewCreatedAt,
			&item.OldSubmitted,
			&item.NewSubmitted,
			&item.OldReported,
			&item.NewReported,
			&item.OldOpenAt,
			&item.NewOpenAt,
			&item.OldCompleted,
			&item.NewCompleted,
		); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func printPreviewRows(rows []previewRow) {
	for _, row := range rows {
		log.Printf("preview assessment_id=%d task_id=%d org_id=%d plan_id=%d testee_id=%d target_date=%s created:%s=>%s submitted:%s=>%s interpreted:%s=>%s task_open:%s=>%s task_completed:%s=>%s",
			row.AssessmentID, row.TaskID, row.OrgID, row.PlanID, row.TesteeID, row.TargetDate,
			formatNullTime(row.OldCreatedAt), formatNullTime(row.NewCreatedAt),
			formatNullTime(row.OldSubmitted), formatNullTime(row.NewSubmitted),
			formatNullTime(row.OldReported), formatNullTime(row.NewReported),
			formatNullTime(row.OldOpenAt), formatNullTime(row.NewOpenAt),
			formatNullTime(row.OldCompleted), formatNullTime(row.NewCompleted),
		)
	}
}

func backupSourceRows(ctx context.Context, conn *sql.Conn, cfg config) error {
	statements := []string{
		fmt.Sprintf("CREATE TABLE seeddata_rewrite_bak_assessment_%s LIKE assessment", cfg.backupSuffix),
		fmt.Sprintf(`INSERT IGNORE INTO seeddata_rewrite_bak_assessment_%s
SELECT a.* FROM assessment a
INNER JOIN seeddata_assessment_time_rewrite_scope s ON s.assessment_id = a.id`, cfg.backupSuffix),
		fmt.Sprintf("CREATE TABLE seeddata_rewrite_bak_assessment_task_%s LIKE assessment_task", cfg.backupSuffix),
		fmt.Sprintf(`INSERT IGNORE INTO seeddata_rewrite_bak_assessment_task_%s
SELECT t.* FROM assessment_task t
INNER JOIN seeddata_assessment_time_rewrite_scope s ON s.task_id = t.id`, cfg.backupSuffix),
	}
	if cfg.rewriteScoreTimes {
		statements = append(statements,
			fmt.Sprintf("CREATE TABLE seeddata_rewrite_bak_assessment_score_%s LIKE assessment_score", cfg.backupSuffix),
			fmt.Sprintf(`INSERT IGNORE INTO seeddata_rewrite_bak_assessment_score_%s
SELECT sc.* FROM assessment_score sc
INNER JOIN seeddata_assessment_time_rewrite_scope s ON s.assessment_id = sc.assessment_id
WHERE sc.deleted_at IS NULL`, cfg.backupSuffix),
		)
	}
	if cfg.refreshTesteeStats {
		statements = append(statements,
			fmt.Sprintf("CREATE TABLE seeddata_rewrite_bak_testee_%s LIKE testee", cfg.backupSuffix),
			fmt.Sprintf(`INSERT IGNORE INTO seeddata_rewrite_bak_testee_%s
SELECT te.* FROM testee te
INNER JOIN (
  SELECT DISTINCT org_id, testee_id
  FROM seeddata_assessment_time_rewrite_scope
) s ON s.org_id = te.org_id AND s.testee_id = te.id
WHERE te.deleted_at IS NULL`, cfg.backupSuffix),
		)
	}

	for i, statement := range statements {
		log.Printf("backup mysql step %d/%d", i+1, len(statements))
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func applyRewrite(ctx context.Context, conn *sql.Conn, cfg config, collapsedDate string) ([]statementResult, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	results := make([]statementResult, 0, 5)
	if result, err := execTx(ctx, tx, "assessment", `
UPDATE assessment a
INNER JOIN seeddata_assessment_time_rewrite_scope s ON s.assessment_id = a.id
SET
  a.created_at = s.new_assessment_created_at,
  a.updated_at = s.new_assessment_updated_at,
  a.submitted_at = s.new_submitted_at,
  a.interpreted_at = s.new_interpreted_at,
  a.failed_at = s.new_failed_at
WHERE a.deleted_at IS NULL`); err != nil {
		return nil, err
	} else {
		results = append(results, result)
	}

	taskSet := []string{
		"t.completed_at = s.new_task_completed_at",
		"t.updated_at = s.new_task_updated_at",
	}
	if cfg.rewriteTaskOpenAt {
		taskSet = append(taskSet, "t.open_at = s.new_task_open_at")
	}
	if cfg.rewriteTaskExpireAt {
		taskSet = append(taskSet, "t.expire_at = s.new_task_expire_at")
	}
	if result, err := execTx(ctx, tx, "assessment_task", `
UPDATE assessment_task t
INNER JOIN seeddata_assessment_time_rewrite_scope s ON s.task_id = t.id
SET
  `+strings.Join(taskSet, ",\n  ")+`
WHERE t.deleted_at IS NULL`); err != nil {
		return nil, err
	} else {
		results = append(results, result)
	}

	if cfg.rewriteScoreTimes {
		if result, err := execTx(ctx, tx, "assessment_score", `
UPDATE assessment_score sc
INNER JOIN seeddata_assessment_time_rewrite_scope s ON s.assessment_id = sc.assessment_id
SET
  sc.created_at = CASE WHEN DATE(sc.created_at) = CAST(? AS DATE) THEN TIMESTAMP(s.target_date, TIME(sc.created_at)) ELSE sc.created_at END,
  sc.updated_at = CASE WHEN sc.updated_at IS NOT NULL AND DATE(sc.updated_at) = CAST(? AS DATE) THEN TIMESTAMP(s.target_date, TIME(sc.updated_at)) ELSE sc.updated_at END
WHERE sc.deleted_at IS NULL
  AND (
    DATE(sc.created_at) = CAST(? AS DATE)
    OR (sc.updated_at IS NOT NULL AND DATE(sc.updated_at) = CAST(? AS DATE))
  )`, collapsedDate, collapsedDate, collapsedDate, collapsedDate); err != nil {
			return nil, err
		} else {
			results = append(results, result)
		}
	}

	if cfg.refreshTesteeStats {
		if result, err := execTx(ctx, tx, "testee_stats", `
UPDATE testee te
INNER JOIN (
  SELECT DISTINCT org_id, testee_id
  FROM seeddata_assessment_time_rewrite_scope
) s ON s.org_id = te.org_id AND s.testee_id = te.id
SET
  te.total_assessments = (
    SELECT COUNT(*)
    FROM assessment a
    WHERE a.org_id = te.org_id
      AND a.testee_id = te.id
      AND a.deleted_at IS NULL
  ),
  te.last_assessment_at = (
    SELECT MAX(COALESCE(a.interpreted_at, a.submitted_at, a.created_at))
    FROM assessment a
    WHERE a.org_id = te.org_id
      AND a.testee_id = te.id
      AND a.deleted_at IS NULL
  ),
  te.last_risk_level = (
    SELECT a2.risk_level
    FROM assessment a2
    WHERE a2.org_id = te.org_id
      AND a2.testee_id = te.id
      AND a2.deleted_at IS NULL
      AND a2.risk_level IS NOT NULL
    ORDER BY COALESCE(a2.interpreted_at, a2.submitted_at, a2.created_at) DESC, a2.id DESC
    LIMIT 1
  )
WHERE te.deleted_at IS NULL`); err != nil {
			return nil, err
		} else {
			results = append(results, result)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func execTx(ctx context.Context, tx *sql.Tx, name, query string, args ...any) (statementResult, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return statementResult{}, fmt.Errorf("%s: %w", name, err)
	}
	affected, _ := res.RowsAffected()
	return statementResult{Name: name, Affected: affected}, nil
}

func scopeDescription(cfg config) string {
	if cfg.allOrgs {
		return "all orgs"
	}
	return fmt.Sprintf("org_id=%d", cfg.orgID)
}

func planScopeDescription(ids []uint64) string {
	if len(ids) == 0 {
		return "<all>"
	}
	copied := append([]uint64(nil), ids...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	parts := make([]string, 0, len(copied))
	for _, id := range copied {
		parts = append(parts, strconv.FormatUint(id, 10))
	}
	return strings.Join(parts, ",")
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	items := make([]string, n)
	for i := range items {
		items[i] = "?"
	}
	return strings.Join(items, ",")
}

func mustParseDate(name, raw string) (time.Time, string) {
	raw = strings.TrimSpace(raw)
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		log.Fatalf("--%s must use YYYY-MM-DD: %v", name, err)
	}
	return parsed, parsed.Format("2006-01-02")
}

func dateRawBefore(left, right string) bool {
	return left < right
}

func validateBackupSuffix(s string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(s) {
		return fmt.Errorf("must match ^[A-Za-z0-9_]+$")
	}
	return nil
}

func nullString(v sql.NullString) string {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return "<null>"
	}
	return v.String
}

func formatNullTime(v sql.NullTime) string {
	if !v.Valid {
		return "<null>"
	}
	return v.Time.Format("2006-01-02 15:04:05")
}
