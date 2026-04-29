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

	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	dbmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	driverMysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type config struct {
	mysqlDSN         string
	orgID            int64
	planID           uint64
	allPlans         bool
	groupByPlan      bool
	taskCreatedStart *time.Time
	taskCreatedEnd   *time.Time
	plannedStart     *time.Time
	plannedEnd       *time.Time
	minRate          float64
	maxRate          float64
	targetRate       float64
	seed             string
	downgradeStatus  string
	backupSuffix     string
	previewLimit     int
	timeout          time.Duration
	apply            bool
	rebuildStats     bool
}

type scopeSummary struct {
	Groups               int64
	Tasks                int64
	CompletedBefore      int64
	EligibleCompleted    int64
	DowngradedTasks      int64
	CompletedAfter       int64
	GroupsStillAboveMax  int64
	GroupsBelowMinBefore int64
	MinRateBefore        sql.NullFloat64
	MaxRateBefore        sql.NullFloat64
	MinRateAfter         sql.NullFloat64
	MaxRateAfter         sql.NullFloat64
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
	if summary.DowngradedTasks == 0 {
		log.Print("no eligible completed tasks need downgrade")
		return
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to mutate task/assessment/footprint data")
		return
	}
	if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
		log.Fatalf("invalid --backup-suffix: %v", err)
	}

	affectedFrom, affectedTo, err := affectedStatisticsWindow(ctx, conn)
	if err != nil {
		log.Fatalf("load affected statistics window: %v", err)
	}
	if err := prepareFootprintScope(ctx, conn); err != nil {
		log.Fatalf("prepare footprint scope: %v", err)
	}
	if err := backupMySQL(ctx, conn, cfg.backupSuffix); err != nil {
		log.Fatalf("backup mysql: %v", err)
	}
	if err := repairMySQL(ctx, conn, cfg); err != nil {
		log.Fatalf("repair mysql: %v", err)
	}
	after, err := loadPostRepairSummary(ctx, conn, cfg)
	if err != nil {
		log.Fatalf("load post-repair summary: %v", err)
	}
	printSummary("after", after)

	if cfg.rebuildStats {
		if err := rebuildStatistics(ctx, db, cfg.orgID, affectedFrom, affectedTo); err != nil {
			log.Fatalf("rebuild statistics projections: %v", err)
		}
		log.Printf("statistics projections rebuilt: org=%d from=%s to=%s", cfg.orgID, formatDay(affectedFrom), formatDay(affectedTo))
	} else {
		log.Print("statistics projections were not rebuilt; run rebuild_operating_statistics or pass --rebuild-statistics")
	}
	log.Print("mock plan task completion-rate repair completed")
}

func parseFlags() config {
	var cfg config
	var taskCreatedStartRaw, taskCreatedEndRaw string
	var plannedStartRaw, plannedEndRaw string

	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true&loc=Local")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID")
	flag.Uint64Var(&cfg.planID, "plan-id", 0, "optional assessment plan ID; use --all-plans when omitted")
	flag.BoolVar(&cfg.allPlans, "all-plans", false, "allow repairing all plans under the org when --plan-id is omitted")
	flag.BoolVar(&cfg.groupByPlan, "group-by-plan", false, "calculate completion rate per testee+plan instead of per testee")
	flag.StringVar(&taskCreatedStartRaw, "task-created-start", "", "optional inclusive task created_at start, format 2006-01-02 or 2006-01-02 15:04:05")
	flag.StringVar(&taskCreatedEndRaw, "task-created-end", "", "optional exclusive task created_at end")
	flag.StringVar(&plannedStartRaw, "planned-start", "", "optional inclusive planned_at start")
	flag.StringVar(&plannedEndRaw, "planned-end", "", "optional exclusive planned_at end")
	flag.Float64Var(&cfg.minRate, "min-rate", 0.30, "lower completion-rate bound used for validation")
	flag.Float64Var(&cfg.maxRate, "max-rate", 0.70, "upper completion-rate bound; groups above this are repaired")
	flag.Float64Var(&cfg.targetRate, "target-rate", 0.50, "target completion-rate after downgrading eligible completed tasks")
	flag.StringVar(&cfg.seed, "seed", "mock-plan-task-completion-rate-v1", "stable seed used to choose tasks to downgrade")
	flag.StringVar(&cfg.downgradeStatus, "downgrade-status", "expired", "task status for downgraded tasks: expired or opened")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of selected task rows to preview")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.rebuildStats, "rebuild-statistics", true, "after apply, rebuild operating statistics projections for affected window and plan snapshots")
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
	if cfg.minRate < 0 || cfg.maxRate > 1 || cfg.targetRate < 0 || cfg.targetRate > 1 || cfg.minRate > cfg.targetRate || cfg.targetRate > cfg.maxRate {
		log.Fatal("rates must satisfy 0 <= min-rate <= target-rate <= max-rate <= 1")
	}
	cfg.downgradeStatus = strings.ToLower(strings.TrimSpace(cfg.downgradeStatus))
	switch cfg.downgradeStatus {
	case "expired", "opened":
	default:
		log.Fatal("--downgrade-status must be expired or opened")
	}
	cfg.seed = strings.TrimSpace(cfg.seed)
	if cfg.seed == "" {
		log.Fatal("--seed cannot be empty")
	}

	cfg.taskCreatedStart = parseOptionalTimeFlag("--task-created-start", taskCreatedStartRaw)
	cfg.taskCreatedEnd = parseOptionalTimeFlag("--task-created-end", taskCreatedEndRaw)
	cfg.plannedStart = parseOptionalTimeFlag("--planned-start", plannedStartRaw)
	cfg.plannedEnd = parseOptionalTimeFlag("--planned-end", plannedEndRaw)
	requirePairedRange("--task-created-start", cfg.taskCreatedStart, "--task-created-end", cfg.taskCreatedEnd)
	requirePairedRange("--planned-start", cfg.plannedStart, "--planned-end", cfg.plannedEnd)
	requireBefore("--task-created", cfg.taskCreatedStart, cfg.taskCreatedEnd)
	requireBefore("--planned", cfg.plannedStart, cfg.plannedEnd)
	return cfg
}

func openMySQL(dsn string) (*sql.DB, error) {
	c, err := driverMysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	c.ParseTime = true
	if c.Collation == "" {
		c.Collation = "utf8mb4_unicode_ci"
	}
	return sql.Open("mysql", c.FormatDSN())
}

func prepareScope(ctx context.Context, conn *sql.Conn, cfg config) error {
	statements := []string{
		"DROP TEMPORARY TABLE IF EXISTS repair_mock_task_group_stats",
		"DROP TEMPORARY TABLE IF EXISTS repair_mock_task_completion_scope",
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := prepareGroupStats(ctx, conn, cfg); err != nil {
		return err
	}
	return prepareTaskScope(ctx, conn, cfg)
}

func prepareGroupStats(ctx context.Context, conn *sql.Conn, cfg config) error {
	groupPlanExpr := "CAST(0 AS UNSIGNED)"
	if cfg.groupByPlan {
		groupPlanExpr = "t.plan_id"
	}
	where, args := buildTaskFilters(cfg, "t")
	query := fmt.Sprintf(`
CREATE TEMPORARY TABLE repair_mock_task_group_stats DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci AS
SELECT
  g.org_id,
  g.group_plan_id,
  g.testee_id,
  g.total_tasks,
  g.completed_tasks,
  g.eligible_completed_tasks,
  g.keep_completed_tasks,
  LEAST(g.eligible_completed_tasks, GREATEST(g.completed_tasks - g.keep_completed_tasks, 0)) AS downgrade_tasks
FROM (
  SELECT
    scoped.org_id,
    scoped.group_plan_id,
    scoped.testee_id,
    COUNT(*) AS total_tasks,
    SUM(scoped.is_completed) AS completed_tasks,
    SUM(scoped.is_eligible_completed) AS eligible_completed_tasks,
    CAST(GREATEST(CEIL(COUNT(*) * ?), LEAST(FLOOR(COUNT(*) * ?), ROUND(COUNT(*) * ?))) AS UNSIGNED) AS keep_completed_tasks
  FROM (
    SELECT
      t.org_id,
      %s AS group_plan_id,
      t.testee_id,
      CASE WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci OR t.completed_at IS NOT NULL THEN 1 ELSE 0 END AS is_completed,
      CASE
        WHEN (t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci OR t.completed_at IS NOT NULL)
         AND t.assessment_id IS NOT NULL
         AND a.id IS NOT NULL
         AND a.origin_type COLLATE utf8mb4_unicode_ci = 'plan' COLLATE utf8mb4_unicode_ci
         AND a.origin_id COLLATE utf8mb4_unicode_ci = CAST(t.plan_id AS CHAR) COLLATE utf8mb4_unicode_ci
        THEN 1 ELSE 0
      END AS is_eligible_completed
    FROM assessment_task t
    LEFT JOIN assessment a
      ON a.id = t.assessment_id
     AND a.org_id = t.org_id
     AND a.deleted_at IS NULL
    WHERE %s
  ) scoped
  GROUP BY scoped.org_id, scoped.group_plan_id, scoped.testee_id
) g
WHERE g.total_tasks > 0
  AND g.completed_tasks / g.total_tasks > ?
  AND LEAST(g.eligible_completed_tasks, GREATEST(g.completed_tasks - g.keep_completed_tasks, 0)) > 0`, groupPlanExpr, where)
	allArgs := []any{cfg.minRate, cfg.maxRate, cfg.targetRate}
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, cfg.maxRate)
	_, err := conn.ExecContext(ctx, query, allArgs...)
	return err
}

func prepareTaskScope(ctx context.Context, conn *sql.Conn, cfg config) error {
	groupPlanExpr := "CAST(0 AS UNSIGNED)"
	if cfg.groupByPlan {
		groupPlanExpr = "t.plan_id"
	}
	where, args := buildTaskFilters(cfg, "t")
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE repair_mock_task_completion_scope (
  task_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  org_id BIGINT NOT NULL,
  plan_id BIGINT UNSIGNED NOT NULL,
  group_plan_id BIGINT UNSIGNED NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  old_task_status VARCHAR(50) NOT NULL,
  old_task_assessment_id BIGINT UNSIGNED NULL,
  old_task_created_at DATETIME(3) NULL,
  old_open_at DATETIME(3) NULL,
  old_expire_at DATETIME(3) NULL,
  old_completed_at DATETIME(3) NULL,
  assessment_id BIGINT UNSIGNED NOT NULL,
  answer_sheet_id BIGINT UNSIGNED NOT NULL,
  old_origin_type VARCHAR(50) NOT NULL,
  old_origin_id VARCHAR(100) NULL,
  old_assessment_created_at DATETIME(3) NULL,
  old_assessment_submitted_at DATETIME(3) NULL,
  old_assessment_interpreted_at DATETIME(3) NULL,
  old_assessment_failed_at DATETIME(3) NULL,
  episode_id BIGINT UNSIGNED NULL,
  old_episode_entry_id BIGINT UNSIGNED NULL,
  old_episode_clinician_id BIGINT UNSIGNED NULL,
  old_attributed_intake_at DATETIME(3) NULL,
  report_id BIGINT UNSIGNED NULL,
  old_episode_submitted_at DATETIME(3) NULL,
  old_episode_assessment_created_at DATETIME(3) NULL,
  old_episode_report_generated_at DATETIME(3) NULL,
  repair_rank BIGINT NOT NULL,
  downgrade_tasks BIGINT NOT NULL,
  KEY idx_repair_mock_task_scope_testee (testee_id, group_plan_id),
  KEY idx_repair_mock_task_scope_assessment (assessment_id),
  KEY idx_repair_mock_task_scope_answersheet (answer_sheet_id),
  KEY idx_repair_mock_task_scope_org (org_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	query := fmt.Sprintf(`
INSERT INTO repair_mock_task_completion_scope (
  task_id, org_id, plan_id, group_plan_id, testee_id, old_task_status,
  old_task_assessment_id, old_task_created_at, old_open_at, old_expire_at,
  old_completed_at, assessment_id, answer_sheet_id, old_origin_type,
  old_origin_id, old_assessment_created_at, old_assessment_submitted_at,
  old_assessment_interpreted_at, old_assessment_failed_at, episode_id,
  old_episode_entry_id, old_episode_clinician_id, old_attributed_intake_at,
  report_id, old_episode_submitted_at, old_episode_assessment_created_at,
  old_episode_report_generated_at, repair_rank, downgrade_tasks
)
SELECT
  task_id, org_id, plan_id, group_plan_id, testee_id, old_task_status,
  old_task_assessment_id, old_task_created_at, old_open_at, old_expire_at,
  old_completed_at, assessment_id, answer_sheet_id, old_origin_type,
  old_origin_id, old_assessment_created_at, old_assessment_submitted_at,
  old_assessment_interpreted_at, old_assessment_failed_at, episode_id,
  old_episode_entry_id, old_episode_clinician_id, old_attributed_intake_at,
  report_id, old_episode_submitted_at, old_episode_assessment_created_at,
  old_episode_report_generated_at, repair_rank, downgrade_tasks
FROM (
  SELECT
    t.id AS task_id,
    t.org_id,
    t.plan_id,
    %s AS group_plan_id,
    t.testee_id,
    t.status AS old_task_status,
    t.assessment_id AS old_task_assessment_id,
    t.created_at AS old_task_created_at,
    t.open_at AS old_open_at,
    t.expire_at AS old_expire_at,
    t.completed_at AS old_completed_at,
    a.id AS assessment_id,
    a.answer_sheet_id,
    a.origin_type AS old_origin_type,
    a.origin_id AS old_origin_id,
    a.created_at AS old_assessment_created_at,
    a.submitted_at AS old_assessment_submitted_at,
    a.interpreted_at AS old_assessment_interpreted_at,
    a.failed_at AS old_assessment_failed_at,
    e.episode_id,
    e.entry_id AS old_episode_entry_id,
    e.clinician_id AS old_episode_clinician_id,
    e.attributed_intake_at AS old_attributed_intake_at,
    e.report_id,
    e.submitted_at AS old_episode_submitted_at,
    e.assessment_created_at AS old_episode_assessment_created_at,
    e.report_generated_at AS old_episode_report_generated_at,
    ROW_NUMBER() OVER (
      PARTITION BY %s, t.testee_id
      ORDER BY CRC32(CONCAT(?, ':', t.id)), t.id
    ) AS repair_rank,
    g.downgrade_tasks
  FROM assessment_task t
  INNER JOIN repair_mock_task_group_stats g
    ON g.org_id = t.org_id
   AND g.group_plan_id = %s
   AND g.testee_id = t.testee_id
  INNER JOIN assessment a
    ON a.id = t.assessment_id
   AND a.org_id = t.org_id
   AND a.deleted_at IS NULL
   AND a.origin_type COLLATE utf8mb4_unicode_ci = 'plan' COLLATE utf8mb4_unicode_ci
   AND a.origin_id COLLATE utf8mb4_unicode_ci = CAST(t.plan_id AS CHAR) COLLATE utf8mb4_unicode_ci
  LEFT JOIN assessment_episode e
    ON e.org_id = t.org_id
   AND e.answersheet_id = a.answer_sheet_id
   AND e.deleted_at IS NULL
  WHERE %s
    AND (t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci OR t.completed_at IS NOT NULL)
    AND t.assessment_id IS NOT NULL
) ranked
WHERE ranked.repair_rank <= ranked.downgrade_tasks`, groupPlanExpr, groupPlanExpr, groupPlanExpr, where)
	allArgs := []any{cfg.seed}
	allArgs = append(allArgs, args...)
	_, err := conn.ExecContext(ctx, query, allArgs...)
	return err
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
	err := conn.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(total_tasks), 0),
  COALESCE(SUM(completed_tasks), 0),
  COALESCE(SUM(eligible_completed_tasks), 0),
  COALESCE(SUM(downgrade_tasks), 0),
  COALESCE(SUM(completed_tasks - downgrade_tasks), 0),
  COALESCE(SUM(CASE WHEN (completed_tasks - downgrade_tasks) / total_tasks > ? THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN completed_tasks / total_tasks < ? THEN 1 ELSE 0 END), 0),
  MIN(completed_tasks / total_tasks),
  MAX(completed_tasks / total_tasks),
  MIN((completed_tasks - downgrade_tasks) / total_tasks),
  MAX((completed_tasks - downgrade_tasks) / total_tasks)
FROM repair_mock_task_group_stats`, cfg.maxRate, cfg.minRate).Scan(
		&summary.Groups,
		&summary.Tasks,
		&summary.CompletedBefore,
		&summary.EligibleCompleted,
		&summary.DowngradedTasks,
		&summary.CompletedAfter,
		&summary.GroupsStillAboveMax,
		&summary.GroupsBelowMinBefore,
		&summary.MinRateBefore,
		&summary.MaxRateBefore,
		&summary.MinRateAfter,
		&summary.MaxRateAfter,
	)
	return summary, err
}

func loadPostRepairSummary(ctx context.Context, conn *sql.Conn, cfg config) (scopeSummary, error) {
	groupPlanExpr := "CAST(0 AS UNSIGNED)"
	groupBy := "t.testee_id"
	existsPlanPredicate := "s.group_plan_id = g.group_plan_id"
	if cfg.groupByPlan {
		groupPlanExpr = "t.plan_id"
		groupBy = "t.plan_id, t.testee_id"
	}
	where, args := buildTaskFilters(cfg, "t")
	query := fmt.Sprintf(`
SELECT
  COUNT(*),
  COALESCE(SUM(total_tasks), 0),
  COALESCE(SUM(completed_tasks), 0),
  0,
  0,
  COALESCE(SUM(completed_tasks), 0),
  COALESCE(SUM(CASE WHEN completed_tasks / total_tasks > ? THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN completed_tasks / total_tasks < ? THEN 1 ELSE 0 END), 0),
  MIN(completed_tasks / total_tasks),
  MAX(completed_tasks / total_tasks),
  MIN(completed_tasks / total_tasks),
  MAX(completed_tasks / total_tasks)
FROM (
  SELECT
    %s AS group_plan_id,
    t.testee_id,
    COUNT(*) AS total_tasks,
    SUM(CASE WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci OR t.completed_at IS NOT NULL THEN 1 ELSE 0 END) AS completed_tasks
  FROM assessment_task t
  WHERE %s
  GROUP BY %s
) g
WHERE EXISTS (
  SELECT 1 FROM repair_mock_task_completion_scope s WHERE s.testee_id = g.testee_id AND %s
)`, groupPlanExpr, where, groupBy, existsPlanPredicate)
	allArgs := []any{cfg.maxRate, cfg.minRate}
	allArgs = append(allArgs, args...)
	var summary scopeSummary
	err := conn.QueryRowContext(ctx, query, allArgs...).Scan(
		&summary.Groups,
		&summary.Tasks,
		&summary.CompletedBefore,
		&summary.EligibleCompleted,
		&summary.DowngradedTasks,
		&summary.CompletedAfter,
		&summary.GroupsStillAboveMax,
		&summary.GroupsBelowMinBefore,
		&summary.MinRateBefore,
		&summary.MaxRateBefore,
		&summary.MinRateAfter,
		&summary.MaxRateAfter,
	)
	return summary, err
}

func printSummary(stage string, summary scopeSummary) {
	log.Printf("%s summary: groups=%d tasks=%d completed_before=%d eligible_completed=%d downgrade=%d completed_after=%d groups_still_above_max=%d groups_below_min=%d rate_before=[%s,%s] rate_after=[%s,%s]",
		stage,
		summary.Groups,
		summary.Tasks,
		summary.CompletedBefore,
		summary.EligibleCompleted,
		summary.DowngradedTasks,
		summary.CompletedAfter,
		summary.GroupsStillAboveMax,
		summary.GroupsBelowMinBefore,
		formatRate(summary.MinRateBefore),
		formatRate(summary.MaxRateBefore),
		formatRate(summary.MinRateAfter),
		formatRate(summary.MaxRateAfter),
	)
}

func printPreviewRows(ctx context.Context, conn *sql.Conn, limit int) error {
	if limit <= 0 {
		return nil
	}
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`
SELECT task_id, plan_id, testee_id, assessment_id, answer_sheet_id, old_completed_at, repair_rank, downgrade_tasks
FROM repair_mock_task_completion_scope
ORDER BY testee_id, group_plan_id, repair_rank, task_id
LIMIT %d`, limit))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var taskID, planID, testeeID, assessmentID, answerSheetID uint64
		var completedAt sql.NullTime
		var rank, downgradeTasks int64
		if err := rows.Scan(&taskID, &planID, &testeeID, &assessmentID, &answerSheetID, &completedAt, &rank, &downgradeTasks); err != nil {
			return err
		}
		log.Printf("preview downgrade task=%d plan=%d testee=%d assessment=%d answersheet=%d completed_at=%s rank=%d/%d",
			taskID, planID, testeeID, assessmentID, answerSheetID, formatNullTime(completedAt), rank, downgradeTasks)
	}
	return rows.Err()
}

func affectedStatisticsWindow(ctx context.Context, conn *sql.Conn) (time.Time, time.Time, error) {
	var from, to sql.NullTime
	err := conn.QueryRowContext(ctx, `
SELECT
  DATE(MIN(LEAST(
    COALESCE(old_task_created_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_open_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_expire_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_completed_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_assessment_created_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_assessment_submitted_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_assessment_interpreted_at, '9999-12-31 23:59:59.999'),
    COALESCE(old_assessment_failed_at, '9999-12-31 23:59:59.999')
  ))),
  DATE_ADD(DATE(MAX(GREATEST(
    COALESCE(old_task_created_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_open_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_expire_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_completed_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_assessment_created_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_assessment_submitted_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_assessment_interpreted_at, '1000-01-01 00:00:00.000'),
    COALESCE(old_assessment_failed_at, '1000-01-01 00:00:00.000')
  ))), INTERVAL 1 DAY)
FROM repair_mock_task_completion_scope`).Scan(&from, &to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !from.Valid || !to.Valid {
		today := normalizeLocalDay(time.Now())
		return today, today.AddDate(0, 0, 1), nil
	}
	return normalizeLocalDay(from.Time), normalizeLocalDay(to.Time), nil
}

func prepareFootprintScope(ctx context.Context, conn *sql.Conn) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "drop old footprint scope",
			sql:  "DROP TEMPORARY TABLE IF EXISTS repair_mock_task_footprint_scope",
		},
		{
			name: "create footprint scope",
			sql: `
CREATE TEMPORARY TABLE repair_mock_task_footprint_scope (
  footprint_id VARCHAR(128) NOT NULL PRIMARY KEY,
  task_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  plan_id BIGINT UNSIGNED NOT NULL,
  answer_sheet_id BIGINT UNSIGNED NOT NULL,
  assessment_id BIGINT UNSIGNED NOT NULL,
  KEY idx_repair_mock_footprint_scope_task (task_id),
  KEY idx_repair_mock_footprint_scope_org (org_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		},
		{
			name: "collect footprints by answersheet",
			sql: `
INSERT IGNORE INTO repair_mock_task_footprint_scope (
  footprint_id, task_id, org_id, plan_id, answer_sheet_id, assessment_id
)
SELECT bf.id, s.task_id, s.org_id, s.plan_id, s.answer_sheet_id, s.assessment_id
FROM repair_mock_task_completion_scope s
STRAIGHT_JOIN behavior_footprint bf
  ON bf.org_id = s.org_id
 AND bf.answersheet_id = s.answer_sheet_id
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
INSERT IGNORE INTO repair_mock_task_footprint_scope (
  footprint_id, task_id, org_id, plan_id, answer_sheet_id, assessment_id
)
SELECT bf.id, s.task_id, s.org_id, s.plan_id, s.answer_sheet_id, s.assessment_id
FROM repair_mock_task_completion_scope s
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
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM repair_mock_task_footprint_scope").Scan(&count); err != nil {
		return err
	}
	log.Printf("prepare footprint scope: matched footprints=%d", count)
	return nil
}

func backupMySQL(ctx context.Context, conn *sql.Conn, suffix string) error {
	statements := []string{
		fmt.Sprintf("CREATE TABLE repair_bak_mock_task_completion_scope_%s AS SELECT * FROM repair_mock_task_completion_scope", suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_mock_task_completion_task_%s AS SELECT t.* FROM assessment_task t INNER JOIN repair_mock_task_completion_scope s ON s.task_id = t.id", suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_mock_task_completion_assessment_%s AS SELECT a.* FROM assessment a INNER JOIN repair_mock_task_completion_scope s ON s.assessment_id = a.id", suffix),
	}
	for i, statement := range statements {
		log.Printf("backup mysql step %d/%d", i+1, 6)
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	episodeBackup := fmt.Sprintf(`
CREATE TABLE repair_bak_mock_task_completion_episode_%s AS
SELECT e.* FROM assessment_episode e
INNER JOIN repair_mock_task_completion_scope s ON s.answer_sheet_id = e.answersheet_id
WHERE e.deleted_at IS NULL`, suffix)
	log.Printf("backup mysql step 4/6")
	if _, err := conn.ExecContext(ctx, episodeBackup); err != nil {
		return err
	}
	footprintLike := fmt.Sprintf("CREATE TABLE repair_bak_mock_task_completion_footprint_%s LIKE behavior_footprint", suffix)
	log.Printf("backup mysql step 5/6")
	if _, err := conn.ExecContext(ctx, footprintLike); err != nil {
		return err
	}
	footprintInsert := fmt.Sprintf(`
INSERT IGNORE INTO repair_bak_mock_task_completion_footprint_%s
SELECT bf.* FROM behavior_footprint bf
INNER JOIN repair_mock_task_footprint_scope fs ON fs.footprint_id = bf.id`, suffix)
	log.Printf("backup mysql step 6/6")
	_, err := conn.ExecContext(ctx, footprintInsert)
	return err
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
		args []any
	}{
		{
			name: "downgrade assessment_task",
			sql: `
UPDATE assessment_task t
INNER JOIN repair_mock_task_completion_scope s ON s.task_id = t.id
SET
  t.status = ?,
  t.completed_at = NULL,
  t.assessment_id = NULL,
  t.expire_at = CASE
    WHEN ? COLLATE utf8mb4_unicode_ci = 'expired' COLLATE utf8mb4_unicode_ci
    THEN LEAST(COALESCE(t.expire_at, t.open_at, t.planned_at, t.created_at, NOW(3)), NOW(3))
    ELSE t.expire_at
  END,
  t.updated_at = NOW(3),
  t.version = t.version + 1`,
			args: []any{cfg.downgradeStatus, cfg.downgradeStatus},
		},
		{
			name: "convert assessment origin to adhoc",
			sql: `
UPDATE assessment a
INNER JOIN repair_mock_task_completion_scope s ON s.assessment_id = a.id
SET
  a.origin_type = 'adhoc',
  a.origin_id = NULL,
  a.updated_at = NOW(3),
  a.version = a.version + 1
WHERE a.deleted_at IS NULL`,
		},
		{
			name: "clear episode entry attribution",
			sql: `
UPDATE assessment_episode e
INNER JOIN repair_mock_task_completion_scope s ON s.answer_sheet_id = e.answersheet_id
SET
  e.entry_id = NULL,
  e.clinician_id = NULL,
  e.attributed_intake_at = NULL,
  e.updated_at = NOW(3)
WHERE e.deleted_at IS NULL`,
		},
		{
			name: "clear footprint entry attribution",
			sql: `
UPDATE behavior_footprint bf
INNER JOIN repair_mock_task_footprint_scope fs ON fs.footprint_id = bf.id
SET
  bf.entry_id = 0,
  bf.clinician_id = 0,
  bf.source_clinician_id = 0,
  bf.properties_json = JSON_SET(
    COALESCE(bf.properties_json, JSON_OBJECT()),
    '$.repair_source', 'repair_mock_plan_task_completion_rate',
    '$.origin_type', 'adhoc',
    '$.detached_plan_id', fs.plan_id,
    '$.detached_task_id', fs.task_id
  ),
  bf.updated_at = NOW(3)
WHERE bf.deleted_at IS NULL`,
		},
	}
	for _, statement := range statements {
		log.Printf("repair mysql step: %s", statement.name)
		result, err := tx.ExecContext(ctx, statement.sql, statement.args...)
		if err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
		if affected, err := result.RowsAffected(); err == nil {
			log.Printf("%s affected rows=%d", statement.name, affected)
		}
	}
	return tx.Commit()
}

func rebuildStatistics(ctx context.Context, sqlDB *sql.DB, orgID int64, from, to time.Time) error {
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		return err
	}
	tx := gormDB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	repo := statisticsInfra.NewStatisticsRepository(gormDB)
	txCtx := dbmysql.WithTx(ctx, tx)
	if err := repo.RebuildDailyStatistics(txCtx, orgID, from, to); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := repo.RebuildAccumulatedStatistics(txCtx, orgID, time.Now().In(time.Local)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := repo.RebuildPlanStatistics(txCtx, orgID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit().Error
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

func formatNullTime(v sql.NullTime) string {
	if !v.Valid {
		return "<null>"
	}
	return v.Time.Format(time.DateTime)
}

func formatRate(v sql.NullFloat64) string {
	if !v.Valid {
		return "<null>"
	}
	return fmt.Sprintf("%.2f%%", v.Float64*100)
}

func formatDay(t time.Time) string {
	if t.IsZero() {
		return "<zero>"
	}
	return t.Format("2006-01-02")
}
