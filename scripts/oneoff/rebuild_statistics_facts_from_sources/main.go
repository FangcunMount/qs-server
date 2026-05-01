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

const generatedEventIDPrefix = "oneoff:fact_rebuild:"

type config struct {
	mysqlDSN       string
	orgID          int64
	allOrgs        bool
	startDateRaw   string
	endDateRaw     string
	timeout        time.Duration
	apply          bool
	resetWindow    bool
	attributionDay int
}

type statementResult struct {
	Name     string
	Affected int64
}

func main() {
	cfg := parseFlags()
	startDate := mustParseDate("start-date", cfg.startDateRaw)
	endDate := parseOptionalDate("end-date", cfg.endDateRaw)

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

	counts, err := previewCounts(ctx, db, cfg, startDate, endDate)
	if err != nil {
		log.Fatalf("preview scope: %v", err)
	}
	log.Printf("scope: %s start=%s end=%s apply=%v reset_window=%v attribution_days=%d",
		scopeDescription(cfg), formatDay(startDate), formatOptionalDay(endDate), cfg.apply, cfg.resetWindow, cfg.attributionDay)
	for _, item := range counts {
		log.Printf("candidate %-32s %d", item.Name, item.Affected)
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to write behavior_footprint and assessment_episode")
		return
	}

	results, err := rebuildFacts(ctx, db, cfg, startDate, endDate)
	if err != nil {
		log.Fatalf("rebuild facts: %v", err)
	}
	for _, item := range results {
		log.Printf("applied %-34s affected_rows=%d", item.Name, item.Affected)
	}
	log.Print("statistics fact rebuild completed")
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to rebuild; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "rebuild all organizations")
	flag.StringVar(&cfg.startDateRaw, "start-date", "2025-01-01", "inclusive lower bound, format YYYY-MM-DD")
	flag.StringVar(&cfg.endDateRaw, "end-date", "", "optional exclusive upper bound, format YYYY-MM-DD")
	flag.DurationVar(&cfg.timeout, "timeout", 4*time.Hour, "overall script timeout, e.g. 30m, 4h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.resetWindow, "reset-window", false, "before rebuilding, delete behavior_footprint and assessment_episode rows in the selected time window")
	flag.IntVar(&cfg.attributionDay, "attribution-days", 30, "look-back window for attributing assessments to clinician_relation rows")
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
	if cfg.attributionDay <= 0 {
		log.Fatal("--attribution-days must be greater than 0")
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

func previewCounts(ctx context.Context, db *sql.DB, cfg config, startDate time.Time, endDate *time.Time) ([]statementResult, error) {
	queries := []struct {
		name  string
		query string
		args  []any
	}{
		countQuery("testee_profile_created", testeeScopeSQL(cfg, "t.created_at", startDate, endDate), nil),
		countQuery("intake_confirmed", relationScopeSQL(cfg, "cr.bound_at", startDate, endDate, " AND cr.source_type = 'assessment_entry' AND cr.source_id IS NOT NULL"), nil),
		countQuery("care_relationship_established", relationScopeSQL(cfg, "cr.bound_at", startDate, endDate, " AND cr.relation_type IN ('primary', 'attending', 'collaborator')"), nil),
		countQuery("care_relationship_transferred", relationScopeSQL(cfg, "cr.bound_at", startDate, endDate, " AND cr.source_type = 'transfer'"), nil),
		countQuery("answersheet_submitted", assessmentEventScopeSQL(cfg, "a.submit_at", startDate, endDate, " AND a.answer_sheet_id <> 0"), nil),
		countQuery("assessment_created", assessmentEventScopeSQL(cfg, "a.created_at", startDate, endDate, ""), nil),
		countQuery("report_generated", assessmentEventScopeSQL(cfg, "a.report_at", startDate, endDate, " AND a.report_at IS NOT NULL"), nil),
		countQuery("assessment_episode", assessmentEventScopeSQL(cfg, "a.submit_at", startDate, endDate, " AND a.answer_sheet_id <> 0"), nil),
	}

	results := make([]statementResult, 0, len(queries))
	for _, item := range queries {
		var count int64
		if err := db.QueryRowContext(ctx, item.query, item.args...).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		results = append(results, statementResult{Name: item.name, Affected: count})
	}
	return results, nil
}

func countQuery(name string, query string, args []any) struct {
	name  string
	query string
	args  []any
} {
	return struct {
		name  string
		query string
		args  []any
	}{name: name, query: query, args: args}
}

func rebuildFacts(ctx context.Context, db *sql.DB, cfg config, startDate time.Time, endDate *time.Time) ([]statementResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	results := make([]statementResult, 0, 10)
	if cfg.resetWindow {
		items, err := resetFactWindow(ctx, tx, cfg, startDate, endDate)
		if err != nil {
			return nil, err
		}
		results = append(results, items...)
	}

	statements := []struct {
		name  string
		query string
		args  []any
	}{
		buildTesteeProfileCreatedInsert(cfg, startDate, endDate),
		buildIntakeConfirmedInsert(cfg, startDate, endDate),
		buildCareRelationshipEstablishedInsert(cfg, startDate, endDate),
		buildCareRelationshipTransferredInsert(cfg, startDate, endDate),
		buildAnswerSheetSubmittedInsert(cfg, startDate, endDate),
		buildAssessmentCreatedInsert(cfg, startDate, endDate),
		buildReportGeneratedInsert(cfg, startDate, endDate),
		buildAssessmentEpisodeInsert(cfg, startDate, endDate),
	}
	for _, stmt := range statements {
		res, err := tx.ExecContext(ctx, stmt.query, stmt.args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.name, err)
		}
		affected, _ := res.RowsAffected()
		results = append(results, statementResult{Name: stmt.name, Affected: affected})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func resetFactWindow(ctx context.Context, tx *sql.Tx, cfg config, startDate time.Time, endDate *time.Time) ([]statementResult, error) {
	results := make([]statementResult, 0, 2)
	for _, stmt := range []struct {
		name  string
		query string
		args  []any
	}{
		buildBehaviorFootprintDelete(cfg, startDate, endDate),
		buildAssessmentEpisodeDelete(cfg, startDate, endDate),
	} {
		res, err := tx.ExecContext(ctx, stmt.query, stmt.args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.name, err)
		}
		affected, _ := res.RowsAffected()
		results = append(results, statementResult{Name: stmt.name, Affected: affected})
	}
	return results, nil
}

func buildBehaviorFootprintDelete(cfg config, startDate time.Time, endDate *time.Time) statement {
	query := "DELETE FROM behavior_footprint WHERE occurred_at >= ?"
	args := []any{startDate}
	if endDate != nil {
		query += " AND occurred_at < ?"
		args = append(args, *endDate)
	}
	if !cfg.allOrgs {
		query += " AND org_id = ?"
		args = append(args, cfg.orgID)
	}
	return statement{name: "reset_behavior_footprint_window", query: query, args: args}
}

func buildAssessmentEpisodeDelete(cfg config, startDate time.Time, endDate *time.Time) statement {
	query := "DELETE FROM assessment_episode WHERE submitted_at >= ?"
	args := []any{startDate}
	if endDate != nil {
		query += " AND submitted_at < ?"
		args = append(args, *endDate)
	}
	if !cfg.allOrgs {
		query += " AND org_id = ?"
		args = append(args, cfg.orgID)
	}
	return statement{name: "reset_assessment_episode_window", query: query, args: args}
}

type statement struct {
	name  string
	query string
	args  []any
}

func buildTesteeProfileCreatedInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := dateWhere("t.created_at", startDate, endDate)
	if !cfg.allOrgs {
		where += " AND t.org_id = ?"
		args = append(args, cfg.orgID)
	}
	query := `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('` + generatedEventIDPrefix + `testee_profile_created:', t.id),
  t.org_id, 'testee', t.id,
  CASE WHEN cr.clinician_id IS NULL THEN 'system' ELSE 'clinician' END,
  COALESCE(cr.clinician_id, 0),
  CASE WHEN cr.source_type = 'assessment_entry' THEN COALESCE(cr.source_id, 0) ELSE 0 END,
  COALESCE(cr.clinician_id, 0),
  0,
  t.id,
  0, 0, 0,
  'testee_profile_created',
  t.created_at,
  JSON_OBJECT('source_table', 'testee', 'source_id', t.id, 'rebuilt_by', 'rebuild_statistics_facts_from_sources')
FROM testee t
LEFT JOIN clinician_relation cr ON cr.id = (
  SELECT cr2.id
  FROM clinician_relation cr2
  WHERE cr2.org_id = t.org_id
    AND cr2.testee_id = t.id
    AND cr2.deleted_at IS NULL
    AND cr2.bound_at >= DATE_SUB(t.created_at, INTERVAL 1 DAY)
    AND cr2.bound_at <= DATE_ADD(t.created_at, INTERVAL 1 DAY)
  ORDER BY ABS(TIMESTAMPDIFF(SECOND, cr2.bound_at, t.created_at)), cr2.id
  LIMIT 1
)
WHERE t.deleted_at IS NULL AND ` + where + behaviorFootprintUpsertSQL()
	return statement{name: "insert_testee_profile_created", query: query, args: args}
}

func buildIntakeConfirmedInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := relationEventWhere(cfg, "cr.bound_at", startDate, endDate, "cr.source_type = 'assessment_entry' AND cr.source_id IS NOT NULL")
	query := relationFootprintInsertSQL("intake_confirmed", "intake_confirmed", false, where)
	return statement{name: "insert_intake_confirmed", query: query, args: args}
}

func buildCareRelationshipEstablishedInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := relationEventWhere(cfg, "cr.bound_at", startDate, endDate, "cr.relation_type IN ('primary', 'attending', 'collaborator')")
	query := relationFootprintInsertSQL("care_relationship_established", "care_relationship_established", false, where)
	return statement{name: "insert_care_relationship_established", query: query, args: args}
}

func buildCareRelationshipTransferredInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := relationEventWhere(cfg, "cr.bound_at", startDate, endDate, "cr.source_type = 'transfer'")
	query := relationFootprintInsertSQL("care_relationship_transferred", "care_relationship_transferred", true, where)
	return statement{name: "insert_care_relationship_transferred", query: query, args: args}
}

func relationFootprintInsertSQL(idPart, eventName string, transferred bool, where string) string {
	sourceClinician := "0"
	if transferred {
		sourceClinician = "COALESCE(cr.source_id, 0)"
	}
	return `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('` + generatedEventIDPrefix + idPart + `:', cr.id),
  cr.org_id, 'testee', cr.testee_id,
  'clinician', cr.clinician_id,
  CASE WHEN cr.source_type = 'assessment_entry' THEN COALESCE(cr.source_id, 0) ELSE 0 END,
  cr.clinician_id,
  ` + sourceClinician + `,
  cr.testee_id,
  0, 0, 0,
  '` + eventName + `',
  cr.bound_at,
  JSON_OBJECT(
    'source_table', 'clinician_relation',
    'source_id', cr.id,
    'relation_type', cr.relation_type,
    'source_type', cr.source_type,
    'rebuilt_by', 'rebuild_statistics_facts_from_sources'
  )
FROM clinician_relation cr
WHERE cr.deleted_at IS NULL AND ` + where + behaviorFootprintUpsertSQL()
}

func buildAnswerSheetSubmittedInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := assessmentOuterWhere("a.submit_at", startDate, endDate, "a.answer_sheet_id <> 0")
	query, prefixArgs := assessmentFootprintInsertSQL(cfg, "answersheet_submitted", "answersheet", "a.answer_sheet_id", "testee", "a.testee_id", "a.submit_at", where)
	return statement{name: "insert_answersheet_submitted", query: query, args: append(prefixArgs, args...)}
}

func buildAssessmentCreatedInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := assessmentOuterWhere("a.created_at", startDate, endDate, "")
	query, prefixArgs := assessmentFootprintInsertSQL(cfg, "assessment_created", "assessment", "a.id", "testee", "a.testee_id", "a.created_at", where)
	return statement{name: "insert_assessment_created", query: query, args: append(prefixArgs, args...)}
}

func buildReportGeneratedInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	where, args := assessmentOuterWhere("a.report_at", startDate, endDate, "a.report_at IS NOT NULL")
	query, prefixArgs := assessmentFootprintInsertSQL(cfg, "report_generated", "report", "a.id", "assessment", "a.id", "a.report_at", where)
	return statement{name: "insert_report_generated", query: query, args: append(prefixArgs, args...)}
}

func assessmentFootprintInsertSQL(cfg config, eventName, subjectType, subjectIDExpr, actorType, actorIDExpr, occurredAtExpr, where string) (string, []any) {
	source, args := assessmentSourceSQL(cfg)
	eventIDExpr := "a.id"
	reportIDExpr := "0"
	if eventName == "answersheet_submitted" {
		eventIDExpr = "a.answer_sheet_id"
	}
	if eventName == "report_generated" {
		reportIDExpr = "a.id"
	}
	query := `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('` + generatedEventIDPrefix + eventName + `:', ` + eventIDExpr + `),
  a.org_id,
  '` + subjectType + `',
  ` + subjectIDExpr + `,
  '` + actorType + `',
  ` + actorIDExpr + `,
  CASE WHEN cr.source_type = 'assessment_entry' THEN COALESCE(cr.source_id, 0) ELSE 0 END,
  COALESCE(cr.clinician_id, 0),
  0,
  a.testee_id,
  a.answer_sheet_id,
  a.id,
  ` + reportIDExpr + `,
  '` + eventName + `',
  ` + occurredAtExpr + `,
  JSON_OBJECT('source_table', 'assessment', 'source_id', a.id, 'rebuilt_by', 'rebuild_statistics_facts_from_sources')
FROM (` + source + `) a
` + assessmentRelationJoinSQL(cfg) + `
WHERE ` + where + behaviorFootprintUpsertSQL()
	return query, args
}

func buildAssessmentEpisodeInsert(cfg config, startDate time.Time, endDate *time.Time) statement {
	source, args := assessmentSourceSQL(cfg)
	where, whereArgs := assessmentOuterWhere("a.submit_at", startDate, endDate, "a.answer_sheet_id <> 0")
	query := `
INSERT INTO assessment_episode (
  episode_id, org_id, entry_id, clinician_id, testee_id, answersheet_id,
  assessment_id, report_id, attributed_intake_at, submitted_at,
  assessment_created_at, report_generated_at, failed_at, status, failure_reason
)
SELECT
  a.answer_sheet_id,
  a.org_id,
  CASE WHEN cr.source_type = 'assessment_entry' THEN cr.source_id ELSE NULL END,
  cr.clinician_id,
  a.testee_id,
  a.answer_sheet_id,
  a.id,
  CASE WHEN a.report_at IS NOT NULL THEN a.id ELSE NULL END,
  cr.bound_at,
  a.submit_at,
  a.created_at,
  a.report_at,
  a.fail_at,
  CASE
    WHEN a.fail_at IS NOT NULL THEN 'failed'
    WHEN a.report_at IS NOT NULL THEN 'completed'
    ELSE 'active'
  END,
  COALESCE(a.failure_reason, '')
FROM (` + source + `) a
` + assessmentRelationJoinSQL(cfg) + `
WHERE ` + where + `
ON DUPLICATE KEY UPDATE
  org_id = VALUES(org_id),
  entry_id = VALUES(entry_id),
  clinician_id = VALUES(clinician_id),
  testee_id = VALUES(testee_id),
  assessment_id = VALUES(assessment_id),
  report_id = VALUES(report_id),
  attributed_intake_at = VALUES(attributed_intake_at),
  submitted_at = VALUES(submitted_at),
  assessment_created_at = VALUES(assessment_created_at),
  report_generated_at = VALUES(report_generated_at),
  failed_at = VALUES(failed_at),
  status = VALUES(status),
  failure_reason = VALUES(failure_reason),
  deleted_at = NULL,
  updated_at = NOW(3)`
	return statement{name: "insert_assessment_episode", query: query, args: append(args, whereArgs...)}
}

func assessmentSourceSQL(cfg config) (string, []any) {
	where := "base.deleted_at IS NULL"
	args := make([]any, 0, 1)
	if !cfg.allOrgs {
		where += " AND base.org_id = ?"
		args = append(args, cfg.orgID)
	}
	return `
SELECT
  base.id,
  base.org_id,
  base.testee_id,
  base.answer_sheet_id,
  base.created_at,
  COALESCE(base.submitted_at, task.task_completed_at, base.created_at) AS submit_at,
  COALESCE(base.interpreted_at, CASE WHEN base.status = 'interpreted' THEN score.score_created_at ELSE NULL END) AS report_at,
  CASE
    WHEN base.failed_at IS NOT NULL THEN base.failed_at
    WHEN base.status = 'failed' THEN base.updated_at
    ELSE NULL
  END AS fail_at,
  base.failure_reason
FROM assessment base
LEFT JOIN (
  SELECT assessment_id, MIN(completed_at) AS task_completed_at
  FROM assessment_task
  WHERE deleted_at IS NULL AND completed_at IS NOT NULL
  GROUP BY assessment_id
) task ON task.assessment_id = base.id
LEFT JOIN (
  SELECT assessment_id, MIN(created_at) AS score_created_at
  FROM assessment_score
  WHERE deleted_at IS NULL
  GROUP BY assessment_id
) score ON score.assessment_id = base.id
WHERE ` + where, args
}

func assessmentRelationJoinSQL(cfg config) string {
	days := strconv.Itoa(cfg.attributionDay)
	return `
LEFT JOIN clinician_relation cr ON cr.id = (
  SELECT cr2.id
  FROM clinician_relation cr2
  WHERE cr2.org_id = a.org_id
    AND cr2.testee_id = a.testee_id
    AND cr2.deleted_at IS NULL
    AND cr2.bound_at <= a.submit_at
    AND cr2.bound_at >= DATE_SUB(a.submit_at, INTERVAL ` + days + ` DAY)
    AND cr2.relation_type IN ('assigned', 'primary', 'attending', 'collaborator')
  ORDER BY cr2.bound_at DESC, cr2.id DESC
  LIMIT 1
)`
}

func behaviorFootprintUpsertSQL() string {
	return `
ON DUPLICATE KEY UPDATE
  org_id = VALUES(org_id),
  subject_type = VALUES(subject_type),
  subject_id = VALUES(subject_id),
  actor_type = VALUES(actor_type),
  actor_id = VALUES(actor_id),
  entry_id = VALUES(entry_id),
  clinician_id = VALUES(clinician_id),
  source_clinician_id = VALUES(source_clinician_id),
  testee_id = VALUES(testee_id),
  answersheet_id = VALUES(answersheet_id),
  assessment_id = VALUES(assessment_id),
  report_id = VALUES(report_id),
  event_name = VALUES(event_name),
  occurred_at = VALUES(occurred_at),
  properties_json = VALUES(properties_json),
  deleted_at = NULL,
  updated_at = NOW(3)`
}

func testeeScopeSQL(cfg config, dateExpr string, startDate time.Time, endDate *time.Time) string {
	where, args := dateWhere(dateExpr, startDate, endDate)
	if !cfg.allOrgs {
		where += " AND t.org_id = " + strconv.FormatInt(cfg.orgID, 10)
	}
	return "SELECT COUNT(*) FROM testee t WHERE t.deleted_at IS NULL AND " + interpolateDateArgs(where, args)
}

func relationScopeSQL(cfg config, dateExpr string, startDate time.Time, endDate *time.Time, extra string) string {
	where, args := dateWhere(dateExpr, startDate, endDate)
	if !cfg.allOrgs {
		where += " AND cr.org_id = " + strconv.FormatInt(cfg.orgID, 10)
	}
	return "SELECT COUNT(*) FROM clinician_relation cr WHERE cr.deleted_at IS NULL AND " + interpolateDateArgs(where+extra, args)
}

func assessmentEventScopeSQL(cfg config, dateExpr string, startDate time.Time, endDate *time.Time, extra string) string {
	source, _ := assessmentSourceSQL(config{allOrgs: true, attributionDay: cfg.attributionDay})
	where, args := assessmentOuterWhere(dateExpr, startDate, endDate, strings.TrimPrefix(strings.TrimSpace(extra), "AND "))
	if !cfg.allOrgs {
		where += " AND a.org_id = " + strconv.FormatInt(cfg.orgID, 10)
	}
	return "SELECT COUNT(*) FROM (" + source + ") a WHERE " + interpolateDateArgs(where, args)
}

func relationEventWhere(cfg config, dateExpr string, startDate time.Time, endDate *time.Time, extra string) (string, []any) {
	where, args := dateWhere(dateExpr, startDate, endDate)
	if strings.TrimSpace(extra) != "" {
		where += " AND " + extra
	}
	if !cfg.allOrgs {
		where += " AND cr.org_id = ?"
		args = append(args, cfg.orgID)
	}
	return where, args
}

func assessmentOuterWhere(dateExpr string, startDate time.Time, endDate *time.Time, extra string) (string, []any) {
	where, args := dateWhere(dateExpr, startDate, endDate)
	if strings.TrimSpace(extra) != "" {
		where += " AND " + extra
	}
	return where, args
}

func dateWhere(expr string, startDate time.Time, endDate *time.Time) (string, []any) {
	where := expr + " >= ?"
	args := []any{startDate}
	if endDate != nil {
		where += " AND " + expr + " < ?"
		args = append(args, *endDate)
	}
	return where, args
}

func interpolateDateArgs(query string, args []any) string {
	result := query
	for _, arg := range args {
		t, ok := arg.(time.Time)
		if !ok {
			continue
		}
		result = strings.Replace(result, "?", "'"+t.Format("2006-01-02 15:04:05")+"'", 1)
	}
	return result
}

func mustParseDate(name, raw string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), time.Local)
	if err != nil {
		log.Fatalf("--%s must use YYYY-MM-DD: %v", name, err)
	}
	return t
}

func parseOptionalDate(name, raw string) *time.Time {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	t := mustParseDate(name, raw)
	return &t
}

func scopeDescription(cfg config) string {
	if cfg.allOrgs {
		return "all_orgs"
	}
	return fmt.Sprintf("org_id=%d", cfg.orgID)
}

func formatDay(t time.Time) string {
	return t.Format("2006-01-02")
}

func formatOptionalDay(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatDay(*t)
}
