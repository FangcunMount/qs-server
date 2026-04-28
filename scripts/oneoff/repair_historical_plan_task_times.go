package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mysqlDSN         string
	mongoURI         string
	mongoDatabase    string
	orgID            int64
	planID           uint64
	taskCreatedStart string
	taskCreatedEnd   string
	plannedStart     string
	plannedEnd       string
	backupSuffix     string
	timeout          time.Duration
	apply            bool
	limit            int
	maxTasks         int
}

type scopeRow struct {
	TaskID        uint64
	OrgID         int64
	PlanID        uint64
	TesteeID      uint64
	AssessmentID  uint64
	AnswerSheetID uint64
	EpisodeID     sql.NullInt64
	ReportID      sql.NullInt64
	EntryID       uint64
	ClinicianID   uint64

	PlannedAt                     time.Time
	OldTaskCreatedAt              time.Time
	OldOpenAt                     sql.NullTime
	OldExpireAt                   sql.NullTime
	OldCompletedAt                sql.NullTime
	OldAssessmentCreatedAt        time.Time
	OldAssessmentSubmittedAt      sql.NullTime
	OldAssessmentInterpretedAt    sql.NullTime
	OldEpisodeSubmittedAt         sql.NullTime
	OldEpisodeAssessmentCreatedAt sql.NullTime
	OldEpisodeReportGeneratedAt   sql.NullTime

	NewTaskCreatedAt       time.Time
	NewOpenAt              time.Time
	NewExpireAt            time.Time
	NewCompletedAt         time.Time
	NewSubmittedAt         time.Time
	NewAssessmentCreatedAt time.Time
	NewInterpretedAt       sql.NullTime
	NewReportGeneratedAt   sql.NullTime
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

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		log.Fatalf("open mongo: %v", err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("ping mongo: %v", err)
	}
	mongoDB := mongoClient.Database(cfg.mongoDatabase)

	if _, err := db.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		log.Fatalf("set mysql names: %v", err)
	}

	rows, err := loadScope(ctx, db, cfg)
	if err != nil {
		log.Fatalf("load scope: %v", err)
	}
	printScope(rows, cfg.limit)
	if len(rows) == 0 {
		log.Print("scope is empty; nothing to repair")
		return
	}
	if cfg.maxTasks > 0 && len(rows) >= cfg.maxTasks {
		log.Printf("WARNING: Reached max-tasks limit (%d). Loaded %d tasks. Set --max-tasks higher or to 0 to load all tasks.", cfg.maxTasks, len(rows))
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to update MySQL and MongoDB")
		return
	}

	if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
		log.Fatalf("invalid --backup-suffix: %v", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("mysql conn: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close mysql conn: %v", err)
		}
	}()

	if err := prepareMySQLScope(ctx, conn, rows); err != nil {
		log.Fatalf("prepare mysql repair scope: %v", err)
	}
	if err := backupMySQL(ctx, conn, cfg.backupSuffix); err != nil {
		log.Fatalf("backup mysql: %v", err)
	}
	if err := backupMongo(ctx, mongoDB, rows, cfg.backupSuffix); err != nil {
		log.Fatalf("backup mongo: %v", err)
	}
	if err := repairMySQL(ctx, conn, cfg); err != nil {
		log.Fatalf("repair mysql: %v", err)
	}
	if err := repairMongo(ctx, mongoDB, rows); err != nil {
		log.Fatalf("repair mongo: %v", err)
	}

	log.Printf("DONE repaired tasks=%d assessments=%d answersheets=%d", len(rows), len(rows), len(rows))
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", "", "MongoDB URI")
	flag.StringVar(&cfg.mongoDatabase, "mongo-db", "", "MongoDB database name")
	flag.Int64Var(&cfg.orgID, "org-id", 1, "organization ID")
	flag.Uint64Var(&cfg.planID, "plan-id", 0, "assessment plan ID")
	flag.StringVar(&cfg.taskCreatedStart, "task-created-start", "", "inclusive task created_at start, format 2006-01-02 15:04:05")
	flag.StringVar(&cfg.taskCreatedEnd, "task-created-end", "", "exclusive task created_at end, format 2006-01-02 15:04:05")
	flag.StringVar(&cfg.plannedStart, "planned-start", "", "optional inclusive planned_at start")
	flag.StringVar(&cfg.plannedEnd, "planned-end", "", "optional exclusive planned_at end")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables/collections")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Minute, "overall script timeout, e.g. 30m, 1h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.IntVar(&cfg.limit, "preview-limit", 20, "number of rows to preview")
	flag.IntVar(&cfg.maxTasks, "max-tasks", 10000, "maximum number of tasks to repair; set to 0 for unlimited (use with caution!)")
	flag.Parse()

	required := map[string]string{
		"--mysql-dsn":          cfg.mysqlDSN,
		"--mongo-uri":          cfg.mongoURI,
		"--mongo-db":           cfg.mongoDatabase,
		"--task-created-start": cfg.taskCreatedStart,
		"--task-created-end":   cfg.taskCreatedEnd,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			log.Fatalf("%s is required", name)
		}
	}
	if cfg.planID == 0 {
		log.Fatal("--plan-id is required")
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

func loadScope(ctx context.Context, db *sql.DB, cfg config) (rows []scopeRow, err error) {
	query := `
SELECT
  t.id, t.org_id, t.plan_id, t.testee_id, t.assessment_id, a.answer_sheet_id,
  e.episode_id, e.report_id, COALESCE(e.entry_id, 0), COALESCE(e.clinician_id, 0),
  t.planned_at, t.created_at, t.open_at, t.expire_at, t.completed_at,
  a.created_at, a.submitted_at, a.interpreted_at,
  e.submitted_at, e.assessment_created_at, e.report_generated_at
FROM assessment_task t
INNER JOIN assessment a
  ON a.id = t.assessment_id
 AND a.deleted_at IS NULL
LEFT JOIN assessment_episode e
  ON e.answersheet_id = a.answer_sheet_id
 AND e.deleted_at IS NULL
WHERE t.org_id = ?
  AND t.plan_id = ?
  AND t.deleted_at IS NULL
  AND t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci
  AND t.assessment_id IS NOT NULL
  AND t.created_at >= ?
  AND t.created_at < ?
  AND a.origin_type COLLATE utf8mb4_unicode_ci = 'plan' COLLATE utf8mb4_unicode_ci
  AND a.origin_id COLLATE utf8mb4_unicode_ci = ? COLLATE utf8mb4_unicode_ci`

	args := []any{cfg.orgID, cfg.planID, cfg.taskCreatedStart, cfg.taskCreatedEnd, strconv.FormatUint(cfg.planID, 10)}
	if cfg.plannedStart != "" {
		query += " AND t.planned_at >= ?"
		args = append(args, cfg.plannedStart)
	}
	if cfg.plannedEnd != "" {
		query += " AND t.planned_at < ?"
		args = append(args, cfg.plannedEnd)
	}
	query += " ORDER BY t.planned_at, t.id"
	if cfg.maxTasks > 0 {
		query += fmt.Sprintf(" LIMIT %d", cfg.maxTasks)
	}

	rs, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rs.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	for rs.Next() {
		var row scopeRow
		if err := rs.Scan(
			&row.TaskID, &row.OrgID, &row.PlanID, &row.TesteeID, &row.AssessmentID, &row.AnswerSheetID,
			&row.EpisodeID, &row.ReportID, &row.EntryID, &row.ClinicianID,
			&row.PlannedAt, &row.OldTaskCreatedAt, &row.OldOpenAt, &row.OldExpireAt, &row.OldCompletedAt,
			&row.OldAssessmentCreatedAt, &row.OldAssessmentSubmittedAt, &row.OldAssessmentInterpretedAt,
			&row.OldEpisodeSubmittedAt, &row.OldEpisodeAssessmentCreatedAt, &row.OldEpisodeReportGeneratedAt,
		); err != nil {
			return nil, err
		}
		computeNewTimes(&row)
		rows = append(rows, row)
	}
	return rows, rs.Err()
}

func computeNewTimes(row *scopeRow) {
	completeDelay := 5 * time.Minute
	if row.OldOpenAt.Valid && row.OldCompletedAt.Valid {
		d := row.OldCompletedAt.Time.Sub(row.OldOpenAt.Time)
		if d >= 0 && d <= 24*time.Hour {
			if d < time.Minute {
				d = time.Minute
			}
			completeDelay = d
		}
	}
	createDelay := 30 * time.Second
	if row.OldAssessmentSubmittedAt.Valid {
		d := row.OldAssessmentCreatedAt.Sub(row.OldAssessmentSubmittedAt.Time)
		if d >= 0 && d <= time.Hour {
			createDelay = d
		}
	}
	interpretDelay := time.Minute
	if row.OldAssessmentInterpretedAt.Valid {
		d := row.OldAssessmentInterpretedAt.Time.Sub(row.OldAssessmentCreatedAt)
		if d >= 0 && d <= 24*time.Hour {
			if d < time.Second {
				d = time.Second
			}
			interpretDelay = d
		}
	}

	row.NewTaskCreatedAt = row.PlannedAt.Add(-7 * 24 * time.Hour)
	row.NewOpenAt = row.PlannedAt
	row.NewExpireAt = row.PlannedAt.Add(7 * 24 * time.Hour)
	row.NewCompletedAt = row.PlannedAt.Add(completeDelay)
	row.NewSubmittedAt = row.NewCompletedAt
	row.NewAssessmentCreatedAt = row.NewSubmittedAt.Add(createDelay)
	if row.OldAssessmentInterpretedAt.Valid {
		row.NewInterpretedAt = sql.NullTime{Time: row.NewAssessmentCreatedAt.Add(interpretDelay), Valid: true}
	}
	if row.OldEpisodeReportGeneratedAt.Valid {
		t := row.NewAssessmentCreatedAt.Add(interpretDelay)
		row.NewReportGeneratedAt = sql.NullTime{Time: t, Valid: true}
	}
}

func printScope(rows []scopeRow, limit int) {
	log.Printf("scope tasks=%d", len(rows))
	if len(rows) == 0 {
		return
	}
	log.Printf("planned_at range: %s -> %s", rows[0].PlannedAt.Format(time.DateTime), rows[len(rows)-1].PlannedAt.Format(time.DateTime))
	if limit > len(rows) {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		r := rows[i]
		log.Printf(
			"preview task=%d testee=%d assessment=%d answersheet=%d planned=%s old_completed=%s new_completed=%s new_interpreted=%s",
			r.TaskID, r.TesteeID, r.AssessmentID, r.AnswerSheetID,
			r.PlannedAt.Format(time.DateTime),
			formatNullTime(r.OldCompletedAt),
			r.NewCompletedAt.Format(time.DateTime),
			formatNullTime(r.NewInterpretedAt),
		)
	}
}

func prepareMySQLScope(ctx context.Context, conn *sql.Conn, rows []scopeRow) (err error) {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS repair_plan_task_time_scope`); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE repair_plan_task_time_scope (
  task_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  org_id BIGINT NOT NULL,
  plan_id BIGINT UNSIGNED NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  assessment_id BIGINT UNSIGNED NOT NULL,
  answer_sheet_id BIGINT UNSIGNED NOT NULL,
  episode_id BIGINT UNSIGNED NULL,
  report_id BIGINT UNSIGNED NULL,
  entry_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  clinician_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  planned_at DATETIME(3) NOT NULL,
  old_episode_submitted_at DATETIME(3) NULL,
  old_episode_assessment_created_at DATETIME(3) NULL,
  old_episode_report_generated_at DATETIME(3) NULL,
  new_task_created_at DATETIME(3) NOT NULL,
  new_open_at DATETIME(3) NOT NULL,
  new_expire_at DATETIME(3) NOT NULL,
  new_completed_at DATETIME(3) NOT NULL,
  new_submitted_at DATETIME(3) NOT NULL,
  new_assessment_created_at DATETIME(3) NOT NULL,
  new_interpreted_at DATETIME(3) NULL,
  new_report_generated_at DATETIME(3) NULL,
  KEY idx_repair_scope_answer_sheet_id (answer_sheet_id),
  KEY idx_repair_scope_assessment_id (assessment_id),
  KEY idx_repair_scope_org_id (org_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return err
	}
	stmt, err := conn.PrepareContext(ctx, `
INSERT INTO repair_plan_task_time_scope (
  task_id, org_id, plan_id, testee_id, assessment_id, answer_sheet_id,
  episode_id, report_id, entry_id, clinician_id, planned_at,
  old_episode_submitted_at, old_episode_assessment_created_at, old_episode_report_generated_at,
  new_task_created_at, new_open_at, new_expire_at, new_completed_at, new_submitted_at,
  new_assessment_created_at, new_interpreted_at, new_report_generated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stmt.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			r.TaskID, r.OrgID, r.PlanID, r.TesteeID, r.AssessmentID, r.AnswerSheetID,
			nullInt64ToAny(r.EpisodeID), nullInt64ToAny(r.ReportID), r.EntryID, r.ClinicianID, r.PlannedAt,
			nullTimeToAny(r.OldEpisodeSubmittedAt), nullTimeToAny(r.OldEpisodeAssessmentCreatedAt), nullTimeToAny(r.OldEpisodeReportGeneratedAt),
			r.NewTaskCreatedAt, r.NewOpenAt, r.NewExpireAt, r.NewCompletedAt, r.NewSubmittedAt,
			r.NewAssessmentCreatedAt, nullTimeToAny(r.NewInterpretedAt), nullTimeToAny(r.NewReportGeneratedAt),
		); err != nil {
			return err
		}
	}
	return nil
}

func backupMySQL(ctx context.Context, conn *sql.Conn, suffix string) error {
	statements := []string{
		fmt.Sprintf("CREATE TABLE repair_bak_assessment_task_%s AS SELECT t.* FROM assessment_task t INNER JOIN repair_plan_task_time_scope s ON s.task_id = t.id", suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_assessment_%s AS SELECT a.* FROM assessment a INNER JOIN repair_plan_task_time_scope s ON s.assessment_id = a.id", suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_assessment_score_%s AS SELECT sc.* FROM assessment_score sc INNER JOIN repair_plan_task_time_scope s ON s.assessment_id = sc.assessment_id WHERE sc.deleted_at IS NULL", suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_assessment_episode_%s AS SELECT e.* FROM assessment_episode e INNER JOIN repair_plan_task_time_scope s ON s.answer_sheet_id = e.answersheet_id WHERE e.deleted_at IS NULL", suffix),
		fmt.Sprintf("CREATE TABLE repair_bak_behavior_footprint_%s LIKE behavior_footprint", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO repair_bak_behavior_footprint_%s
SELECT bf.* FROM behavior_footprint bf
INNER JOIN repair_plan_task_time_scope s ON s.answer_sheet_id = bf.answersheet_id
WHERE bf.org_id = s.org_id
  AND bf.deleted_at IS NULL
  AND bf.event_name COLLATE utf8mb4_unicode_ci IN ('answersheet_submitted' COLLATE utf8mb4_unicode_ci, 'assessment_created' COLLATE utf8mb4_unicode_ci, 'report_generated' COLLATE utf8mb4_unicode_ci)`, suffix),
		fmt.Sprintf(`INSERT IGNORE INTO repair_bak_behavior_footprint_%s
SELECT bf.* FROM behavior_footprint bf
INNER JOIN repair_plan_task_time_scope s ON s.assessment_id = bf.assessment_id
WHERE bf.org_id = s.org_id
  AND bf.deleted_at IS NULL
  AND bf.event_name COLLATE utf8mb4_unicode_ci IN ('answersheet_submitted' COLLATE utf8mb4_unicode_ci, 'assessment_created' COLLATE utf8mb4_unicode_ci, 'report_generated' COLLATE utf8mb4_unicode_ci)`, suffix),
	}
	for i, statement := range statements {
		log.Printf("backup mysql step %d/%d", i+1, len(statements))
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func backupMongo(ctx context.Context, db *mongo.Database, rows []scopeRow, suffix string) error {
	answerIDs := make([]int64, 0, len(rows))
	assessmentIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		answerIDs = append(answerIDs, int64(r.AnswerSheetID))
		assessmentIDs = append(assessmentIDs, int64(r.AssessmentID))
	}
	if err := backupMongoCollection(ctx, db.Collection("answersheets"), db.Collection("repair_bak_answersheets_"+suffix), bson.M{"domain_id": bson.M{"$in": answerIDs}}); err != nil {
		return err
	}
	return backupMongoCollection(ctx, db.Collection("interpret_reports"), db.Collection("repair_bak_interpret_reports_"+suffix), bson.M{"domain_id": bson.M{"$in": assessmentIDs}})
}

func backupMongoCollection(ctx context.Context, src, dst *mongo.Collection, filter bson.M) (err error) {
	cur, err := src.Find(ctx, filter)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := cur.Close(ctx); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	var docs []interface{}
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return err
		}
		docs = append(docs, doc)
	}
	if err := cur.Err(); err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	_, err = dst.InsertMany(ctx, docs)
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

	statements := []string{
		`UPDATE assessment_task t INNER JOIN repair_plan_task_time_scope s ON s.task_id = t.id
SET t.created_at = s.new_task_created_at, t.open_at = s.new_open_at, t.expire_at = s.new_expire_at, t.completed_at = s.new_completed_at, t.updated_at = NOW(3), t.version = t.version + 1`,
		`UPDATE assessment a INNER JOIN repair_plan_task_time_scope s ON s.assessment_id = a.id
SET a.created_at = s.new_assessment_created_at, a.submitted_at = s.new_submitted_at,
    a.interpreted_at = CASE WHEN a.interpreted_at IS NULL THEN NULL ELSE s.new_interpreted_at END,
    a.updated_at = NOW(3), a.version = a.version + 1`,
		`UPDATE assessment_score sc INNER JOIN repair_plan_task_time_scope s ON s.assessment_id = sc.assessment_id
SET sc.created_at = s.new_assessment_created_at, sc.updated_at = NOW(3), sc.version = sc.version + 1
WHERE sc.deleted_at IS NULL`,
		`UPDATE assessment_episode e INNER JOIN repair_plan_task_time_scope s ON s.answer_sheet_id = e.answersheet_id
SET e.submitted_at = s.new_submitted_at,
    e.assessment_created_at = CASE WHEN e.assessment_created_at IS NULL THEN NULL ELSE s.new_assessment_created_at END,
    e.report_generated_at = CASE WHEN e.report_generated_at IS NULL THEN NULL ELSE s.new_report_generated_at END,
    e.updated_at = NOW(3)
WHERE e.deleted_at IS NULL`,
		`UPDATE behavior_footprint bf INNER JOIN repair_plan_task_time_scope s ON s.answer_sheet_id = bf.answersheet_id
SET bf.occurred_at = CASE bf.event_name COLLATE utf8mb4_unicode_ci
    WHEN 'answersheet_submitted' COLLATE utf8mb4_unicode_ci THEN s.new_submitted_at
    WHEN 'assessment_created' COLLATE utf8mb4_unicode_ci THEN s.new_assessment_created_at
    WHEN 'report_generated' COLLATE utf8mb4_unicode_ci THEN s.new_report_generated_at
    ELSE bf.occurred_at END,
    bf.updated_at = NOW(3)
WHERE bf.org_id = s.org_id
  AND bf.deleted_at IS NULL
  AND bf.event_name COLLATE utf8mb4_unicode_ci IN ('answersheet_submitted' COLLATE utf8mb4_unicode_ci, 'assessment_created' COLLATE utf8mb4_unicode_ci, 'report_generated' COLLATE utf8mb4_unicode_ci)
  AND (bf.event_name COLLATE utf8mb4_unicode_ci <> 'report_generated' COLLATE utf8mb4_unicode_ci OR s.new_report_generated_at IS NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := repairPlanStatistics(ctx, tx, cfg); err != nil {
		return err
	}
	if err := repairAnalyticsProjection(ctx, tx); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func repairPlanStatistics(ctx context.Context, tx *sql.Tx, cfg config) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM statistics_plan WHERE org_id = ? AND plan_id = ?", cfg.orgID, cfg.planID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO statistics_plan (
  org_id, plan_id, total_tasks, completed_tasks, pending_tasks,
  expired_tasks, enrolled_testees, active_testees, last_updated_at
)
SELECT
  t.org_id, t.plan_id, COUNT(*),
  SUM(CASE WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci THEN 1 ELSE 0 END),
  SUM(CASE WHEN t.status COLLATE utf8mb4_unicode_ci IN ('pending' COLLATE utf8mb4_unicode_ci, 'opened' COLLATE utf8mb4_unicode_ci) THEN 1 ELSE 0 END),
  SUM(CASE WHEN t.status COLLATE utf8mb4_unicode_ci = 'expired' COLLATE utf8mb4_unicode_ci THEN 1 ELSE 0 END),
  COUNT(DISTINCT t.testee_id),
  COUNT(DISTINCT CASE WHEN t.status COLLATE utf8mb4_unicode_ci = 'completed' COLLATE utf8mb4_unicode_ci THEN t.testee_id END),
  NOW()
FROM assessment_task t
WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL
GROUP BY t.org_id, t.plan_id`, cfg.orgID, cfg.planID)
	return err
}

func repairAnalyticsProjection(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`DROP TEMPORARY TABLE IF EXISTS repair_plan_task_projection_delta_raw`,
		`DROP TEMPORARY TABLE IF EXISTS repair_plan_task_projection_delta`,
		`CREATE TEMPORARY TABLE repair_plan_task_projection_delta_raw (
  org_id BIGINT NOT NULL,
  clinician_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  entry_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  stat_date DATE NOT NULL,
  answersheet_submitted_delta BIGINT NOT NULL DEFAULT 0,
  assessment_created_delta BIGINT NOT NULL DEFAULT 0,
  report_generated_delta BIGINT NOT NULL DEFAULT 0,
  episode_completed_delta BIGINT NOT NULL DEFAULT 0
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`INSERT INTO repair_plan_task_projection_delta_raw
SELECT org_id, clinician_id, entry_id, DATE(old_episode_submitted_at), -1, 0, 0, 0
FROM repair_plan_task_time_scope WHERE old_episode_submitted_at IS NOT NULL`,
		`INSERT INTO repair_plan_task_projection_delta_raw
SELECT org_id, clinician_id, entry_id, DATE(new_submitted_at), 1, 0, 0, 0
FROM repair_plan_task_time_scope`,
		`INSERT INTO repair_plan_task_projection_delta_raw
SELECT org_id, clinician_id, entry_id, DATE(old_episode_assessment_created_at), 0, -1, 0, 0
FROM repair_plan_task_time_scope WHERE old_episode_assessment_created_at IS NOT NULL`,
		`INSERT INTO repair_plan_task_projection_delta_raw
SELECT org_id, clinician_id, entry_id, DATE(new_assessment_created_at), 0, 1, 0, 0
FROM repair_plan_task_time_scope WHERE old_episode_assessment_created_at IS NOT NULL`,
		`INSERT INTO repair_plan_task_projection_delta_raw
SELECT org_id, clinician_id, entry_id, DATE(old_episode_report_generated_at), 0, 0, -1, -1
FROM repair_plan_task_time_scope WHERE old_episode_report_generated_at IS NOT NULL`,
		`INSERT INTO repair_plan_task_projection_delta_raw
SELECT org_id, clinician_id, entry_id, DATE(new_report_generated_at), 0, 0, 1, 1
FROM repair_plan_task_time_scope WHERE new_report_generated_at IS NOT NULL`,
		`CREATE TEMPORARY TABLE repair_plan_task_projection_delta DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci AS
SELECT org_id, clinician_id, entry_id, stat_date,
       SUM(answersheet_submitted_delta) AS answersheet_submitted_delta,
       SUM(assessment_created_delta) AS assessment_created_delta,
       SUM(report_generated_delta) AS report_generated_delta,
       SUM(episode_completed_delta) AS episode_completed_delta
FROM repair_plan_task_projection_delta_raw
GROUP BY org_id, clinician_id, entry_id, stat_date`,
		`INSERT INTO analytics_projection_org_daily (
  org_id, stat_date, entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count, created_at, updated_at
)
SELECT org_id, stat_date, 0, 0, 0, 0, 0, answersheet_submitted_delta, assessment_created_delta, report_generated_delta, episode_completed_delta, 0, NOW(3), NOW(3)
FROM repair_plan_task_projection_delta
ON DUPLICATE KEY UPDATE
  answersheet_submitted_count = answersheet_submitted_count + VALUES(answersheet_submitted_count),
  assessment_created_count = assessment_created_count + VALUES(assessment_created_count),
  report_generated_count = report_generated_count + VALUES(report_generated_count),
  episode_completed_count = episode_completed_count + VALUES(episode_completed_count),
  updated_at = NOW(3)`,
		`INSERT INTO analytics_projection_clinician_daily (
  org_id, clinician_id, stat_date, entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count, created_at, updated_at
)
SELECT org_id, clinician_id, stat_date, 0, 0, 0, 0, 0, answersheet_submitted_delta, assessment_created_delta, report_generated_delta, episode_completed_delta, 0, NOW(3), NOW(3)
FROM repair_plan_task_projection_delta WHERE clinician_id <> 0
ON DUPLICATE KEY UPDATE
  answersheet_submitted_count = answersheet_submitted_count + VALUES(answersheet_submitted_count),
  assessment_created_count = assessment_created_count + VALUES(assessment_created_count),
  report_generated_count = report_generated_count + VALUES(report_generated_count),
  episode_completed_count = episode_completed_count + VALUES(episode_completed_count),
  updated_at = NOW(3)`,
		`INSERT INTO analytics_projection_entry_daily (
  org_id, entry_id, clinician_id, stat_date, entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count, created_at, updated_at
)
SELECT org_id, entry_id, clinician_id, stat_date, 0, 0, 0, 0, 0, answersheet_submitted_delta, assessment_created_delta, report_generated_delta, episode_completed_delta, 0, NOW(3), NOW(3)
FROM repair_plan_task_projection_delta WHERE entry_id <> 0
ON DUPLICATE KEY UPDATE
  clinician_id = VALUES(clinician_id),
  answersheet_submitted_count = answersheet_submitted_count + VALUES(answersheet_submitted_count),
  assessment_created_count = assessment_created_count + VALUES(assessment_created_count),
  report_generated_count = report_generated_count + VALUES(report_generated_count),
  episode_completed_count = episode_completed_count + VALUES(episode_completed_count),
  updated_at = NOW(3)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func repairMongo(ctx context.Context, db *mongo.Database, rows []scopeRow) error {
	answersheets := db.Collection("answersheets")
	reports := db.Collection("interpret_reports")
	for _, r := range rows {
		if _, err := answersheets.UpdateOne(ctx,
			bson.M{"domain_id": int64(r.AnswerSheetID)},
			bson.M{"$set": bson.M{
				"filled_at":  r.NewSubmittedAt,
				"created_at": r.NewSubmittedAt,
				"updated_at": r.NewSubmittedAt,
			}},
		); err != nil {
			return fmt.Errorf("update answersheet %d: %w", r.AnswerSheetID, err)
		}
		if r.NewReportGeneratedAt.Valid {
			if _, err := reports.UpdateOne(ctx,
				bson.M{"domain_id": int64(r.AssessmentID)},
				bson.M{"$set": bson.M{
					"created_at": r.NewReportGeneratedAt.Time,
					"updated_at": r.NewReportGeneratedAt.Time,
				}},
			); err != nil {
				return fmt.Errorf("update interpret report %d: %w", r.AssessmentID, err)
			}
		}
	}
	return nil
}

func validateBackupSuffix(s string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(s) {
		return fmt.Errorf("must match ^[A-Za-z0-9_]+$")
	}
	return nil
}

func nullInt64ToAny(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullTimeToAny(v sql.NullTime) any {
	if !v.Valid {
		return nil
	}
	return v.Time
}

func formatNullTime(v sql.NullTime) string {
	if !v.Valid {
		return "<null>"
	}
	return v.Time.Format(time.DateTime)
}
