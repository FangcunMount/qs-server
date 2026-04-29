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

	"github.com/go-sql-driver/mysql"
)

// repair_statistics_facts rebuilds the source facts used by journey statistics:
// behavior_footprint and assessment_episode.
//
// The script is intentionally source-table driven. By default it derives intake
// facts from clinician_relation, because those rows are the current relationship
// source of truth after historical data repair. Use --intake-source=log to replay
// the old assessment_entry_intake_log based migration semantics exactly.
//
// Typical usage:
//
//	go run scripts/oneoff/repair_statistics_facts/repair_statistics_facts.go \
//	  --mysql-dsn 'user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true' \
//	  --org-id 1
//
// Re-run with --apply after reviewing the dry-run output. Run the statistics
// rebuild script afterwards so analytics projections and read models consume
// the repaired facts.
type config struct {
	mysqlDSN          string
	orgID             int64
	allOrgs           bool
	cutoffDate        string
	cutoff            time.Time
	backupSuffix      string
	timeout           time.Duration
	apply             bool
	skipBackup        bool
	intakeSource      string
	repairEpisodes    bool
	repairFootprints  bool
	replaceEpisodes   bool
	replaceFootprints bool
}

type tableCount struct {
	Name  string
	Count int64
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

	if err := prepareSession(ctx, conn, cfg); err != nil {
		log.Fatalf("prepare mysql session: %v", err)
	}
	if err := prepareSourceTables(ctx, conn, cfg); err != nil {
		log.Fatalf("prepare source facts: %v", err)
	}
	if err := printDryRunSummary(ctx, conn, cfg); err != nil {
		log.Fatalf("dry-run summary: %v", err)
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to repair behavior_footprint and assessment_episode")
		return
	}

	if !cfg.skipBackup {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
		if err := backupFactTables(ctx, conn, cfg); err != nil {
			log.Fatalf("backup fact tables: %v", err)
		}
	}

	if cfg.repairEpisodes {
		if err := repairEpisodes(ctx, conn, cfg); err != nil {
			log.Fatalf("repair assessment_episode: %v", err)
		}
	}
	if cfg.repairFootprints {
		if err := repairFootprints(ctx, conn, cfg); err != nil {
			log.Fatalf("repair behavior_footprint: %v", err)
		}
	}
	log.Print("statistics facts repair completed")
}

func parseFlags() config {
	var cfg config
	defaultCutoff := time.Now().In(time.Local).AddDate(0, 0, 1).Format("2006-01-02")
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to repair; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "repair all organizations")
	flag.StringVar(&cfg.cutoffDate, "cutoff-date", defaultCutoff, "exclusive upper date bound, format YYYY-MM-DD; default is tomorrow local date")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout, e.g. 30m, 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup table creation before applying changes")
	flag.StringVar(&cfg.intakeSource, "intake-source", "relation", "source for intake facts and episode attribution: relation or log")
	flag.BoolVar(&cfg.repairEpisodes, "repair-episodes", true, "repair assessment_episode from assessment and intake source")
	flag.BoolVar(&cfg.repairFootprints, "repair-footprints", true, "repair behavior_footprint from entry, relation/log, and assessment facts")
	flag.BoolVar(&cfg.replaceEpisodes, "replace-episodes", true, "soft-delete scoped assessment_episode rows no longer generated from source facts")
	flag.BoolVar(&cfg.replaceFootprints, "replace-footprints", true, "soft-delete scoped rebuildable behavior_footprint rows no longer generated from source facts")
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
	cfg.intakeSource = strings.TrimSpace(strings.ToLower(cfg.intakeSource))
	if cfg.intakeSource != "relation" && cfg.intakeSource != "log" {
		log.Fatal("--intake-source must be relation or log")
	}
	if !cfg.repairEpisodes && !cfg.repairFootprints {
		log.Fatal("nothing to repair: both --repair-episodes and --repair-footprints are false")
	}

	cutoff, err := time.ParseInLocation("2006-01-02", cfg.cutoffDate, time.Local)
	if err != nil {
		log.Fatalf("invalid --cutoff-date: %v", err)
	}
	cfg.cutoff = cutoff
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
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params["multiStatements"] = "true"
	db, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func prepareSession(ctx context.Context, conn *sql.Conn, cfg config) error {
	if _, err := conn.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "SET @qs_repair_org_id := ?, @qs_repair_cutoff := ?", scopeOrgID(cfg), cfg.cutoff.Format("2006-01-02")); err != nil {
		return err
	}
	return nil
}

func scopeOrgID(cfg config) int64 {
	if cfg.allOrgs {
		return 0
	}
	return cfg.orgID
}

func scopeDescription(cfg config) string {
	if cfg.allOrgs {
		return "all orgs"
	}
	return fmt.Sprintf("org_id=%d", cfg.orgID)
}

func prepareSourceTables(ctx context.Context, conn *sql.Conn, cfg config) error {
	statements := []string{
		"DROP TEMPORARY TABLE IF EXISTS repair_stats_fact_episode_source",
		"DROP TEMPORARY TABLE IF EXISTS repair_stats_fact_attribution_source",
		"DROP TEMPORARY TABLE IF EXISTS repair_stats_fact_behavior_source",
		createEpisodeSourceTableSQL,
		createAttributionSourceTableSQL,
		createBehaviorSourceTableSQL,
		insertEpisodeSourceSQL,
	}
	for _, stmt := range statements {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	if cfg.intakeSource == "relation" {
		if _, err := conn.ExecContext(ctx, insertAttributionFromRelationSQL); err != nil {
			return err
		}
	} else {
		if _, err := conn.ExecContext(ctx, insertAttributionFromLogSQL); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, updateEpisodeSourceAttributionSQL); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, insertBehaviorFromResolveLogSQL); err != nil {
		return err
	}
	if cfg.intakeSource == "relation" {
		for _, stmt := range []string{
			insertBehaviorIntakeFromRelationSQL,
			insertBehaviorTesteeCreatedFromRelationSQL,
			insertBehaviorCareEstablishedFromRelationSQL,
		} {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
	} else {
		for _, stmt := range []string{
			insertBehaviorIntakeFromLogSQL,
			insertBehaviorTesteeCreatedFromLogSQL,
			insertBehaviorCareEstablishedFromLogSQL,
		} {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
	}
	for _, stmt := range []string{
		insertBehaviorAnswerSheetSubmittedSQL,
		insertBehaviorAssessmentCreatedSQL,
		insertBehaviorReportGeneratedSQL,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func printDryRunSummary(ctx context.Context, conn *sql.Conn, cfg config) error {
	log.Printf("scope: %s cutoff_date(exclusive): %s apply=%v backup=%v intake_source=%s replace_episodes=%v replace_footprints=%v",
		scopeDescription(cfg), cfg.cutoff.Format("2006-01-02"), cfg.apply, !cfg.skipBackup, cfg.intakeSource, cfg.replaceEpisodes, cfg.replaceFootprints)

	counts, err := loadCounts(ctx, conn, []string{
		"SELECT COUNT(*) FROM testee WHERE deleted_at IS NULL AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		"SELECT COUNT(*) FROM clinician WHERE deleted_at IS NULL AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		"SELECT COUNT(*) FROM clinician_relation WHERE deleted_at IS NULL AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		"SELECT COUNT(*) FROM assessment_entry WHERE deleted_at IS NULL AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		"SELECT COUNT(*) FROM assessment WHERE deleted_at IS NULL AND answer_sheet_id <> 0 AND COALESCE(submitted_at, created_at) < @qs_repair_cutoff AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		"SELECT COUNT(*) FROM assessment_entry_resolve_log WHERE deleted_at IS NULL AND resolved_at < @qs_repair_cutoff AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		sourceIntakeCountSQL(cfg),
	}, []string{
		"source testee",
		"source clinician",
		"source clinician_relation",
		"source assessment_entry",
		"source assessment(answer_sheet)",
		"source assessment_entry_resolve_log",
		"source intake facts",
	})
	if err != nil {
		return err
	}
	for _, item := range counts {
		log.Printf("%s: %d", item.Name, item.Count)
	}

	factCounts, err := loadCounts(ctx, conn, []string{
		"SELECT COUNT(*) FROM assessment_episode WHERE deleted_at IS NULL AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id) AND submitted_at < @qs_repair_cutoff",
		"SELECT COUNT(*) FROM repair_stats_fact_episode_source",
		"SELECT COUNT(*) FROM repair_stats_fact_episode_source s LEFT JOIN assessment_episode e ON e.answersheet_id = s.answersheet_id AND e.deleted_at IS NULL WHERE e.answersheet_id IS NULL",
		episodeMismatchCountSQL,
		"SELECT COUNT(*) FROM assessment_episode e LEFT JOIN repair_stats_fact_episode_source s ON s.episode_id = e.episode_id WHERE e.deleted_at IS NULL AND (@qs_repair_org_id = 0 OR e.org_id = @qs_repair_org_id) AND e.submitted_at < @qs_repair_cutoff AND s.episode_id IS NULL",
		"SELECT COUNT(*) FROM behavior_footprint WHERE deleted_at IS NULL AND event_name IN ('entry_opened', 'intake_confirmed', 'testee_profile_created', 'care_relationship_established', 'answersheet_submitted', 'assessment_created', 'report_generated') AND occurred_at < @qs_repair_cutoff AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)",
		"SELECT COUNT(*) FROM repair_stats_fact_behavior_source",
		"SELECT COUNT(*) FROM repair_stats_fact_behavior_source s LEFT JOIN behavior_footprint bf ON bf.id = s.id AND bf.deleted_at IS NULL WHERE bf.id IS NULL",
		behaviorMismatchCountSQL,
		"SELECT COUNT(*) FROM behavior_footprint bf LEFT JOIN repair_stats_fact_behavior_source s ON s.id = bf.id WHERE bf.deleted_at IS NULL AND bf.event_name IN ('entry_opened', 'intake_confirmed', 'testee_profile_created', 'care_relationship_established', 'answersheet_submitted', 'assessment_created', 'report_generated') AND bf.occurred_at < @qs_repair_cutoff AND (@qs_repair_org_id = 0 OR bf.org_id = @qs_repair_org_id) AND s.id IS NULL",
	}, []string{
		"existing assessment_episode",
		"expected assessment_episode",
		"missing assessment_episode",
		"mismatched assessment_episode",
		"stale assessment_episode(if replace)",
		"existing rebuildable behavior_footprint",
		"expected canonical behavior_footprint",
		"missing canonical behavior_footprint",
		"mismatched canonical behavior_footprint",
		"noncanonical behavior_footprint(if replace)",
	})
	if err != nil {
		return err
	}
	for _, item := range factCounts {
		log.Printf("%s: %d", item.Name, item.Count)
	}

	if err := printGroupedCounts(ctx, conn, "expected behavior_footprint by event", "SELECT event_name, COUNT(*) FROM repair_stats_fact_behavior_source GROUP BY event_name ORDER BY event_name"); err != nil {
		return err
	}
	if err := printGroupedCounts(ctx, conn, "existing behavior_footprint by event", "SELECT event_name, COUNT(*) FROM behavior_footprint WHERE deleted_at IS NULL AND event_name IN ('entry_opened', 'intake_confirmed', 'testee_profile_created', 'care_relationship_established', 'answersheet_submitted', 'assessment_created', 'report_generated') AND occurred_at < @qs_repair_cutoff AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id) GROUP BY event_name ORDER BY event_name"); err != nil {
		return err
	}
	return nil
}

func sourceIntakeCountSQL(cfg config) string {
	if cfg.intakeSource == "relation" {
		return `SELECT COUNT(*)
FROM clinician_relation r
WHERE r.deleted_at IS NULL
  AND r.relation_type COLLATE utf8mb4_unicode_ci = 'creator' COLLATE utf8mb4_unicode_ci
  AND r.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
  AND r.source_id IS NOT NULL
  AND r.bound_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR r.org_id = @qs_repair_org_id)`
	}
	return `SELECT COUNT(*)
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND l.intake_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR l.org_id = @qs_repair_org_id)`
}

func loadCounts(ctx context.Context, conn *sql.Conn, queries []string, names []string) ([]tableCount, error) {
	result := make([]tableCount, 0, len(queries))
	for i, query := range queries {
		var count int64
		if err := conn.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", names[i], err)
		}
		result = append(result, tableCount{Name: names[i], Count: count})
	}
	return result, nil
}

func printGroupedCounts(ctx context.Context, conn *sql.Conn, label string, query string) error {
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return err
		}
		log.Printf("%s: %s=%d", label, name, count)
	}
	return rows.Err()
}

func validateBackupSuffix(suffix string) error {
	if matched := regexp.MustCompile(`^[0-9A-Za-z_]+$`).MatchString(suffix); !matched {
		return fmt.Errorf("must contain only letters, numbers, or underscore")
	}
	return nil
}

func backupFactTables(ctx context.Context, conn *sql.Conn, cfg config) error {
	tables := []string{"assessment_episode", "behavior_footprint"}
	for _, table := range tables {
		backup := fmt.Sprintf("%s_backup_%s", table, cfg.backupSuffix)
		log.Printf("backup %s -> %s", table, backup)
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE `%s`", backup, table)); err != nil {
			return err
		}
		where := "@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id"
		if table == "behavior_footprint" {
			where = "(@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id) AND event_name IN ('entry_opened', 'intake_confirmed', 'testee_profile_created', 'care_relationship_established', 'answersheet_submitted', 'assessment_created', 'report_generated')"
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("INSERT IGNORE INTO `%s` SELECT * FROM `%s` WHERE %s", backup, table, where)); err != nil {
			return err
		}
	}
	return nil
}

func repairEpisodes(ctx context.Context, conn *sql.Conn, cfg config) error {
	if cfg.replaceEpisodes {
		log.Print("soft-delete stale assessment_episode")
		if _, err := conn.ExecContext(ctx, replaceEpisodesSQL); err != nil {
			return err
		}
	}
	log.Print("upsert assessment_episode")
	if _, err := conn.ExecContext(ctx, upsertEpisodesSQL); err != nil {
		return err
	}
	return nil
}

func repairFootprints(ctx context.Context, conn *sql.Conn, cfg config) error {
	if cfg.replaceFootprints {
		log.Print("soft-delete noncanonical behavior_footprint")
		if _, err := conn.ExecContext(ctx, replaceFootprintsSQL); err != nil {
			return err
		}
	}
	log.Print("upsert behavior_footprint")
	if _, err := conn.ExecContext(ctx, upsertFootprintsSQL); err != nil {
		return err
	}
	return nil
}

const createEpisodeSourceTableSQL = `
CREATE TEMPORARY TABLE repair_stats_fact_episode_source (
  episode_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  entry_id BIGINT UNSIGNED NULL,
  clinician_id BIGINT UNSIGNED NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  answersheet_id BIGINT UNSIGNED NOT NULL,
  assessment_id BIGINT UNSIGNED NULL,
  report_id BIGINT UNSIGNED NULL,
  attributed_intake_at DATETIME(3) NULL,
  submitted_at DATETIME(3) NOT NULL,
  assessment_created_at DATETIME(3) NULL,
  report_generated_at DATETIME(3) NULL,
  failed_at DATETIME(3) NULL,
  status VARCHAR(32) NOT NULL,
  failure_reason TEXT NULL,
  PRIMARY KEY (episode_id),
  UNIQUE KEY uk_repair_episode_answersheet (answersheet_id),
  KEY idx_repair_episode_org_submitted (org_id, submitted_at),
  KEY idx_repair_episode_assessment (org_id, assessment_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const createAttributionSourceTableSQL = `
CREATE TEMPORARY TABLE repair_stats_fact_attribution_source (
  answersheet_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  entry_id BIGINT UNSIGNED NOT NULL,
  clinician_id BIGINT UNSIGNED NOT NULL,
  intake_at DATETIME(3) NOT NULL,
  PRIMARY KEY (answersheet_id),
  KEY idx_repair_attr_org_entry (org_id, entry_id),
  KEY idx_repair_attr_org_clinician (org_id, clinician_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const createBehaviorSourceTableSQL = `
CREATE TEMPORARY TABLE repair_stats_fact_behavior_source (
  id VARCHAR(128) NOT NULL,
  org_id BIGINT NOT NULL,
  subject_type VARCHAR(64) NOT NULL,
  subject_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  actor_type VARCHAR(64) NOT NULL,
  actor_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  entry_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  clinician_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  source_clinician_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  testee_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  answersheet_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  assessment_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  report_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  event_name VARCHAR(64) NOT NULL,
  occurred_at DATETIME(3) NOT NULL,
  properties_json JSON NULL,
  PRIMARY KEY (id),
  KEY idx_repair_behavior_org_event_time (org_id, event_name, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const insertEpisodeSourceSQL = `
INSERT INTO repair_stats_fact_episode_source (
  episode_id, org_id, entry_id, clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, attributed_intake_at, submitted_at,
  assessment_created_at, report_generated_at, failed_at, status, failure_reason
)
SELECT
  a.answer_sheet_id,
  a.org_id,
  NULL,
  NULL,
  a.testee_id,
  a.answer_sheet_id,
  a.id,
  COALESCE(existing.report_id, existing_report.report_id),
  NULL,
  COALESCE(a.submitted_at, a.created_at),
  a.created_at,
  CASE WHEN a.interpreted_at IS NOT NULL THEN a.interpreted_at ELSE NULL END,
  a.failed_at,
  CASE
    WHEN a.failed_at IS NOT NULL OR a.status COLLATE utf8mb4_unicode_ci = 'failed' COLLATE utf8mb4_unicode_ci THEN 'failed'
    WHEN a.interpreted_at IS NOT NULL OR a.status COLLATE utf8mb4_unicode_ci = 'interpreted' COLLATE utf8mb4_unicode_ci THEN 'completed'
    ELSE 'active'
  END,
  COALESCE(a.failure_reason, '')
FROM assessment a
LEFT JOIN assessment_episode existing
  ON existing.org_id = a.org_id
 AND existing.answersheet_id = a.answer_sheet_id
 AND existing.deleted_at IS NULL
LEFT JOIN (
  SELECT org_id, assessment_id, MAX(NULLIF(report_id, 0)) AS report_id
  FROM behavior_footprint
  WHERE deleted_at IS NULL
    AND event_name = 'report_generated'
    AND assessment_id <> 0
    AND (@qs_repair_org_id = 0 OR org_id = @qs_repair_org_id)
  GROUP BY org_id, assessment_id
) existing_report
  ON existing_report.org_id = a.org_id
 AND existing_report.assessment_id = a.id
WHERE a.deleted_at IS NULL
  AND a.answer_sheet_id <> 0
  AND COALESCE(a.submitted_at, a.created_at) < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR a.org_id = @qs_repair_org_id)`

const insertAttributionFromRelationSQL = `
INSERT INTO repair_stats_fact_attribution_source (
  answersheet_id, org_id, entry_id, clinician_id, intake_at
)
SELECT ranked.answersheet_id, ranked.org_id, ranked.entry_id, ranked.clinician_id, ranked.intake_at
FROM (
  SELECT
    a.answer_sheet_id AS answersheet_id,
    a.org_id,
    r.source_id AS entry_id,
    r.clinician_id,
    r.bound_at AS intake_at,
    ROW_NUMBER() OVER (
      PARTITION BY a.answer_sheet_id
      ORDER BY r.bound_at DESC, r.id DESC
    ) AS rn
  FROM assessment a
  JOIN clinician_relation r
    ON r.org_id = a.org_id
   AND r.testee_id = a.testee_id
   AND r.deleted_at IS NULL
   AND r.relation_type COLLATE utf8mb4_unicode_ci = 'creator' COLLATE utf8mb4_unicode_ci
   AND r.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
   AND r.source_id IS NOT NULL
   AND r.bound_at <= COALESCE(a.submitted_at, a.created_at)
   AND r.bound_at >= DATE_SUB(COALESCE(a.submitted_at, a.created_at), INTERVAL 30 DAY)
  WHERE a.deleted_at IS NULL
    AND a.answer_sheet_id <> 0
    AND COALESCE(a.submitted_at, a.created_at) < @qs_repair_cutoff
    AND (@qs_repair_org_id = 0 OR a.org_id = @qs_repair_org_id)
) ranked
WHERE ranked.rn = 1`

const insertAttributionFromLogSQL = `
INSERT INTO repair_stats_fact_attribution_source (
  answersheet_id, org_id, entry_id, clinician_id, intake_at
)
SELECT ranked.answersheet_id, ranked.org_id, ranked.entry_id, ranked.clinician_id, ranked.intake_at
FROM (
  SELECT
    a.answer_sheet_id AS answersheet_id,
    a.org_id,
    l.entry_id,
    l.clinician_id,
    l.intake_at,
    ROW_NUMBER() OVER (
      PARTITION BY a.answer_sheet_id
      ORDER BY l.intake_at DESC, l.id DESC
    ) AS rn
  FROM assessment a
  JOIN assessment_entry_intake_log l
    ON l.org_id = a.org_id
   AND l.testee_id = a.testee_id
   AND l.deleted_at IS NULL
   AND l.intake_at <= COALESCE(a.submitted_at, a.created_at)
   AND l.intake_at >= DATE_SUB(COALESCE(a.submitted_at, a.created_at), INTERVAL 30 DAY)
  WHERE a.deleted_at IS NULL
    AND a.answer_sheet_id <> 0
    AND COALESCE(a.submitted_at, a.created_at) < @qs_repair_cutoff
    AND (@qs_repair_org_id = 0 OR a.org_id = @qs_repair_org_id)
) ranked
WHERE ranked.rn = 1`

const updateEpisodeSourceAttributionSQL = `
UPDATE repair_stats_fact_episode_source e
JOIN repair_stats_fact_attribution_source a
  ON a.answersheet_id = e.answersheet_id
SET
  e.entry_id = a.entry_id,
  e.clinician_id = a.clinician_id,
  e.attributed_intake_at = a.intake_at`

const insertBehaviorFromResolveLogSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:entry_opened:', l.id),
  l.org_id,
  'assessment_entry',
  l.entry_id,
  'assessment_entry',
  l.entry_id,
  l.entry_id,
  l.clinician_id,
  0,
  0,
  0,
  0,
  0,
  'entry_opened',
  l.resolved_at,
  JSON_OBJECT('repair_source', 'assessment_entry_resolve_log', 'source_id', l.id)
FROM assessment_entry_resolve_log l
WHERE l.deleted_at IS NULL
  AND l.resolved_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR l.org_id = @qs_repair_org_id)`

const insertBehaviorIntakeFromRelationSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('repair:intake_confirmed:clinician_relation:', r.id),
  r.org_id,
  'testee',
  r.testee_id,
  'clinician',
  r.clinician_id,
  r.source_id,
  r.clinician_id,
  0,
  r.testee_id,
  0,
  0,
  0,
  'intake_confirmed',
  r.bound_at,
  JSON_OBJECT('repair_source', 'clinician_relation', 'source_id', r.id)
FROM clinician_relation r
WHERE r.deleted_at IS NULL
  AND r.relation_type COLLATE utf8mb4_unicode_ci = 'creator' COLLATE utf8mb4_unicode_ci
  AND r.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
  AND r.source_id IS NOT NULL
  AND r.bound_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR r.org_id = @qs_repair_org_id)`

const insertBehaviorTesteeCreatedFromRelationSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('repair:testee_profile_created:clinician_relation:', r.id),
  r.org_id,
  'testee',
  r.testee_id,
  'clinician',
  r.clinician_id,
  r.source_id,
  r.clinician_id,
  0,
  r.testee_id,
  0,
  0,
  0,
  'testee_profile_created',
  r.bound_at,
  JSON_OBJECT('repair_source', 'clinician_relation', 'source_id', r.id)
FROM clinician_relation r
JOIN testee t
  ON t.id = r.testee_id
 AND t.org_id = r.org_id
 AND t.deleted_at IS NULL
WHERE r.deleted_at IS NULL
  AND r.relation_type COLLATE utf8mb4_unicode_ci = 'creator' COLLATE utf8mb4_unicode_ci
  AND r.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
  AND r.source_id IS NOT NULL
  AND r.bound_at < @qs_repair_cutoff
  AND ABS(TIMESTAMPDIFF(SECOND, t.created_at, r.bound_at)) <= 5
  AND (@qs_repair_org_id = 0 OR r.org_id = @qs_repair_org_id)`

const insertBehaviorCareEstablishedFromRelationSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('repair:care_relationship_established:clinician_relation:', r.id),
  r.org_id,
  'testee',
  r.testee_id,
  'clinician',
  r.clinician_id,
  r.source_id,
  r.clinician_id,
  0,
  r.testee_id,
  0,
  0,
  0,
  'care_relationship_established',
  r.bound_at,
  JSON_OBJECT('repair_source', 'clinician_relation', 'source_id', r.id)
FROM clinician_relation r
WHERE r.deleted_at IS NULL
  AND r.relation_type COLLATE utf8mb4_unicode_ci = 'creator' COLLATE utf8mb4_unicode_ci
  AND r.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
  AND r.source_id IS NOT NULL
  AND r.bound_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR r.org_id = @qs_repair_org_id)
  AND EXISTS (
    SELECT 1
    FROM clinician_relation ar
    WHERE ar.org_id = r.org_id
      AND ar.clinician_id = r.clinician_id
      AND ar.testee_id = r.testee_id
      AND ar.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
      AND ar.source_id = r.source_id
      AND ar.relation_type COLLATE utf8mb4_unicode_ci IN (
        'assigned' COLLATE utf8mb4_unicode_ci,
        'primary' COLLATE utf8mb4_unicode_ci,
        'attending' COLLATE utf8mb4_unicode_ci,
        'collaborator' COLLATE utf8mb4_unicode_ci
      )
      AND ar.deleted_at IS NULL
      AND ABS(TIMESTAMPDIFF(SECOND, ar.bound_at, r.bound_at)) <= 5
  )`

const insertBehaviorIntakeFromLogSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:intake_confirmed:', l.id),
  l.org_id,
  'testee',
  l.testee_id,
  'clinician',
  l.clinician_id,
  l.entry_id,
  l.clinician_id,
  0,
  l.testee_id,
  0,
  0,
  0,
  'intake_confirmed',
  l.intake_at,
  JSON_OBJECT('repair_source', 'assessment_entry_intake_log', 'source_id', l.id)
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND l.intake_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR l.org_id = @qs_repair_org_id)`

const insertBehaviorTesteeCreatedFromLogSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:testee_profile_created:', l.id),
  l.org_id,
  'testee',
  l.testee_id,
  'clinician',
  l.clinician_id,
  l.entry_id,
  l.clinician_id,
  0,
  l.testee_id,
  0,
  0,
  0,
  'testee_profile_created',
  l.intake_at,
  JSON_OBJECT('repair_source', 'assessment_entry_intake_log', 'source_id', l.id)
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND l.testee_created = 1
  AND l.intake_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR l.org_id = @qs_repair_org_id)`

const insertBehaviorCareEstablishedFromLogSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:care_relationship_established:', l.id),
  l.org_id,
  'testee',
  l.testee_id,
  'clinician',
  l.clinician_id,
  l.entry_id,
  l.clinician_id,
  0,
  l.testee_id,
  0,
  0,
  0,
  'care_relationship_established',
  l.intake_at,
  JSON_OBJECT('repair_source', 'assessment_entry_intake_log', 'source_id', l.id)
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND l.assignment_created = 1
  AND l.intake_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR l.org_id = @qs_repair_org_id)`

const insertBehaviorAnswerSheetSubmittedSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:answersheet_submitted:', e.answersheet_id),
  e.org_id,
  'answersheet',
  e.answersheet_id,
  'testee',
  e.testee_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  e.testee_id,
  e.answersheet_id,
  COALESCE(e.assessment_id, 0),
  COALESCE(e.report_id, 0),
  'answersheet_submitted',
  e.submitted_at,
  JSON_OBJECT('repair_source', 'assessment', 'episode_id', e.episode_id)
FROM repair_stats_fact_episode_source e
WHERE e.submitted_at < @qs_repair_cutoff`

const insertBehaviorAssessmentCreatedSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:assessment_created:', e.assessment_id),
  e.org_id,
  'assessment',
  e.assessment_id,
  'testee',
  e.testee_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  e.testee_id,
  e.answersheet_id,
  e.assessment_id,
  COALESCE(e.report_id, 0),
  'assessment_created',
  e.assessment_created_at,
  JSON_OBJECT('repair_source', 'assessment', 'episode_id', e.episode_id)
FROM repair_stats_fact_episode_source e
WHERE e.assessment_id IS NOT NULL
  AND e.assessment_created_at IS NOT NULL
  AND e.assessment_created_at < @qs_repair_cutoff`

const insertBehaviorReportGeneratedSQL = `
INSERT INTO repair_stats_fact_behavior_source (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('legacy:report_generated:', e.assessment_id),
  e.org_id,
  'report',
  COALESCE(e.report_id, 0),
  'assessment',
  e.assessment_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  e.testee_id,
  e.answersheet_id,
  e.assessment_id,
  COALESCE(e.report_id, 0),
  'report_generated',
  e.report_generated_at,
  JSON_OBJECT('repair_source', 'assessment', 'episode_id', e.episode_id)
FROM repair_stats_fact_episode_source e
WHERE e.assessment_id IS NOT NULL
  AND e.report_generated_at IS NOT NULL
  AND e.report_generated_at < @qs_repair_cutoff`

const episodeMismatchCountSQL = `
SELECT COUNT(*)
FROM repair_stats_fact_episode_source s
JOIN assessment_episode e
  ON e.answersheet_id = s.answersheet_id
 AND e.deleted_at IS NULL
WHERE e.org_id <> s.org_id
   OR IFNULL(e.entry_id, 0) <> IFNULL(s.entry_id, 0)
   OR IFNULL(e.clinician_id, 0) <> IFNULL(s.clinician_id, 0)
   OR e.testee_id <> s.testee_id
   OR IFNULL(e.assessment_id, 0) <> IFNULL(s.assessment_id, 0)
   OR IFNULL(e.report_id, 0) <> IFNULL(s.report_id, 0)
   OR (e.attributed_intake_at IS NULL) <> (s.attributed_intake_at IS NULL)
   OR (e.attributed_intake_at IS NOT NULL AND s.attributed_intake_at IS NOT NULL AND TIMESTAMPDIFF(MICROSECOND, e.attributed_intake_at, s.attributed_intake_at) <> 0)
   OR TIMESTAMPDIFF(MICROSECOND, e.submitted_at, s.submitted_at) <> 0
   OR (e.assessment_created_at IS NULL) <> (s.assessment_created_at IS NULL)
   OR (e.assessment_created_at IS NOT NULL AND s.assessment_created_at IS NOT NULL AND TIMESTAMPDIFF(MICROSECOND, e.assessment_created_at, s.assessment_created_at) <> 0)
   OR (e.report_generated_at IS NULL) <> (s.report_generated_at IS NULL)
   OR (e.report_generated_at IS NOT NULL AND s.report_generated_at IS NOT NULL AND TIMESTAMPDIFF(MICROSECOND, e.report_generated_at, s.report_generated_at) <> 0)
   OR (e.failed_at IS NULL) <> (s.failed_at IS NULL)
   OR (e.failed_at IS NOT NULL AND s.failed_at IS NOT NULL AND TIMESTAMPDIFF(MICROSECOND, e.failed_at, s.failed_at) <> 0)
   OR e.status COLLATE utf8mb4_unicode_ci <> s.status COLLATE utf8mb4_unicode_ci
   OR COALESCE(e.failure_reason, '') COLLATE utf8mb4_unicode_ci <> COALESCE(s.failure_reason, '') COLLATE utf8mb4_unicode_ci`

const behaviorMismatchCountSQL = `
SELECT COUNT(*)
FROM repair_stats_fact_behavior_source s
JOIN behavior_footprint bf
  ON bf.id = s.id
 AND bf.deleted_at IS NULL
WHERE bf.org_id <> s.org_id
   OR bf.subject_type COLLATE utf8mb4_unicode_ci <> s.subject_type COLLATE utf8mb4_unicode_ci
   OR bf.subject_id <> s.subject_id
   OR bf.actor_type COLLATE utf8mb4_unicode_ci <> s.actor_type COLLATE utf8mb4_unicode_ci
   OR bf.actor_id <> s.actor_id
   OR bf.entry_id <> s.entry_id
   OR bf.clinician_id <> s.clinician_id
   OR bf.source_clinician_id <> s.source_clinician_id
   OR bf.testee_id <> s.testee_id
   OR bf.answersheet_id <> s.answersheet_id
   OR bf.assessment_id <> s.assessment_id
   OR bf.report_id <> s.report_id
   OR bf.event_name COLLATE utf8mb4_unicode_ci <> s.event_name COLLATE utf8mb4_unicode_ci
   OR TIMESTAMPDIFF(MICROSECOND, bf.occurred_at, s.occurred_at) <> 0`

const replaceEpisodesSQL = `
UPDATE assessment_episode e
LEFT JOIN repair_stats_fact_episode_source s
  ON s.episode_id = e.episode_id
SET
  e.deleted_at = NOW(3),
  e.updated_at = NOW(3)
WHERE e.deleted_at IS NULL
  AND (@qs_repair_org_id = 0 OR e.org_id = @qs_repair_org_id)
  AND e.submitted_at < @qs_repair_cutoff
  AND s.episode_id IS NULL`

const upsertEpisodesSQL = `
INSERT INTO assessment_episode (
  episode_id, org_id, entry_id, clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, attributed_intake_at, submitted_at,
  assessment_created_at, report_generated_at, failed_at, status, failure_reason,
  created_at, updated_at
)
SELECT
  episode_id, org_id, entry_id, clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, attributed_intake_at, submitted_at,
  assessment_created_at, report_generated_at, failed_at, status, failure_reason,
  NOW(3), NOW(3)
FROM repair_stats_fact_episode_source
ON DUPLICATE KEY UPDATE
  episode_id = VALUES(episode_id),
  org_id = VALUES(org_id),
  entry_id = VALUES(entry_id),
  clinician_id = VALUES(clinician_id),
  testee_id = VALUES(testee_id),
  answersheet_id = VALUES(answersheet_id),
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

const replaceFootprintsSQL = `
UPDATE behavior_footprint bf
LEFT JOIN repair_stats_fact_behavior_source s
  ON s.id = bf.id
SET
  bf.deleted_at = NOW(3),
  bf.updated_at = NOW(3)
WHERE bf.deleted_at IS NULL
  AND bf.event_name IN ('entry_opened', 'intake_confirmed', 'testee_profile_created', 'care_relationship_established', 'answersheet_submitted', 'assessment_created', 'report_generated')
  AND bf.occurred_at < @qs_repair_cutoff
  AND (@qs_repair_org_id = 0 OR bf.org_id = @qs_repair_org_id)
  AND s.id IS NULL`

const upsertFootprintsSQL = `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, NOW(3), NOW(3)
FROM repair_stats_fact_behavior_source
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
