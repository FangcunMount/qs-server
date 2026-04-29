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
	redis "github.com/redis/go-redis/v9"
)

// rebuild_statistics rebuilds statistics-side derived data from repaired source facts.
//
// It intentionally does not modify behavior_footprint, assessment_episode,
// assessment, assessment_plan, or assessment_task.
//
// Typical usage:
//
//	go run scripts/oneoff/rebuild_statistics.go \
//	  --mysql-dsn 'app_user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true' \
//	  --org-id 1
//
// Re-run with --apply after reviewing the dry-run output.
type config struct {
	mysqlDSN         string
	orgID            int64
	allOrgs          bool
	cutoffDate       string
	cutoff           time.Time
	backupSuffix     string
	timeout          time.Duration
	apply            bool
	skipBackup       bool
	clearPending     bool
	redisAddr        string
	redisPassword    string
	redisDB          int
	redisKeyPattern  string
	skipCacheCleanup bool
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

	if err := printDryRunSummary(ctx, conn, cfg); err != nil {
		log.Fatalf("dry-run summary: %v", err)
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to rebuild statistics data")
		return
	}

	if !cfg.skipBackup {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
		if err := backupStatisticsTables(ctx, conn, cfg); err != nil {
			log.Fatalf("backup statistics tables: %v", err)
		}
	}

	if cfg.clearPending {
		if err := clearAnalyticsPendingEvents(ctx, conn, cfg); err != nil {
			log.Fatalf("clear analytics pending events: %v", err)
		}
	}
	if err := rebuildAnalyticsProjections(ctx, conn); err != nil {
		log.Fatalf("rebuild analytics projections: %v", err)
	}
	if err := rebuildTraditionalStatistics(ctx, conn); err != nil {
		log.Fatalf("rebuild traditional statistics: %v", err)
	}
	if !cfg.skipCacheCleanup && strings.TrimSpace(cfg.redisAddr) != "" {
		if err := cleanupRedisStatsCache(ctx, cfg); err != nil {
			log.Fatalf("cleanup redis stats cache: %v", err)
		}
	}

	log.Print("statistics rebuild completed")
}

func parseFlags() config {
	var cfg config
	defaultCutoff := time.Now().In(time.Local).AddDate(0, 0, 1).Format("2006-01-02")
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to rebuild; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "rebuild all organizations")
	flag.StringVar(&cfg.cutoffDate, "cutoff-date", defaultCutoff, "exclusive upper date bound, format YYYY-MM-DD; default is tomorrow local date")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout, e.g. 30m, 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup table creation before applying changes")
	flag.BoolVar(&cfg.clearPending, "clear-pending", false, "mark matching analytics pending checkpoints completed and delete matching analytics_pending_event rows")
	flag.StringVar(&cfg.redisAddr, "redis-addr", "", "optional Redis address for stats query cache cleanup, e.g. 127.0.0.1:6379")
	flag.StringVar(&cfg.redisPassword, "redis-password", "", "optional Redis password")
	flag.IntVar(&cfg.redisDB, "redis-db", 0, "optional Redis DB")
	flag.StringVar(&cfg.redisKeyPattern, "redis-key-pattern", "*stats:query*", "Redis SCAN pattern used when --redis-addr is set")
	flag.BoolVar(&cfg.skipCacheCleanup, "skip-cache-cleanup", false, "skip Redis stats query cache cleanup")
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
	if _, err := conn.ExecContext(ctx, "SET @qs_rebuild_org_id := ?, @qs_rebuild_cutoff := ?", scopeOrgID(cfg), cfg.cutoff.Format("2006-01-02")); err != nil {
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

func printDryRunSummary(ctx context.Context, conn *sql.Conn, cfg config) error {
	log.Printf("scope: %s cutoff_date(exclusive): %s apply=%v backup=%v clear_pending=%v",
		scopeDescription(cfg), cfg.cutoff.Format("2006-01-02"), cfg.apply, !cfg.skipBackup, cfg.clearPending)

	counts, err := loadCounts(ctx, conn, []string{
		"SELECT COUNT(*) FROM analytics_projection_org_daily WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id",
		"SELECT COUNT(*) FROM analytics_projection_clinician_daily WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id",
		"SELECT COUNT(*) FROM analytics_projection_entry_daily WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id",
		"SELECT COUNT(*) FROM statistics_daily WHERE statistic_type IN ('questionnaire', 'system') AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
		"SELECT COUNT(*) FROM statistics_accumulated WHERE statistic_type IN ('questionnaire', 'system') AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
		"SELECT COUNT(*) FROM statistics_plan WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id",
	}, []string{
		"existing analytics_projection_org_daily",
		"existing analytics_projection_clinician_daily",
		"existing analytics_projection_entry_daily",
		"existing statistics_daily(questionnaire/system)",
		"existing statistics_accumulated(questionnaire/system)",
		"existing statistics_plan",
	})
	if err != nil {
		return err
	}
	for _, item := range counts {
		log.Printf("%s: %d", item.Name, item.Count)
	}

	sourceCounts, err := loadCounts(ctx, conn, []string{
		"SELECT COUNT(*) FROM behavior_footprint WHERE deleted_at IS NULL AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
		"SELECT COUNT(*) FROM assessment_episode WHERE deleted_at IS NULL AND submitted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
		"SELECT COUNT(*) FROM assessment WHERE deleted_at IS NULL AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
		"SELECT COUNT(*) FROM assessment_plan WHERE deleted_at IS NULL AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
		"SELECT COUNT(*) FROM assessment_task WHERE deleted_at IS NULL AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)",
	}, []string{
		"source behavior_footprint",
		"source assessment_episode",
		"source assessment",
		"source assessment_plan",
		"source assessment_task",
	})
	if err != nil {
		return err
	}
	for _, item := range sourceCounts {
		log.Printf("%s: %d", item.Name, item.Count)
	}

	if cfg.clearPending {
		pending, err := countPendingEvents(ctx, conn, cfg)
		if err != nil {
			return err
		}
		log.Printf("matching analytics_pending_event rows to clear: %d", pending)
	}
	return nil
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

func validateBackupSuffix(suffix string) error {
	if matched := regexp.MustCompile(`^[0-9A-Za-z_]+$`).MatchString(suffix); !matched {
		return fmt.Errorf("must contain only letters, numbers, or underscore")
	}
	return nil
}

func backupStatisticsTables(ctx context.Context, conn *sql.Conn, cfg config) error {
	tables := []string{
		"analytics_projection_org_daily",
		"analytics_projection_clinician_daily",
		"analytics_projection_entry_daily",
		"statistics_daily",
		"statistics_accumulated",
		"statistics_plan",
	}
	for _, table := range tables {
		backup := fmt.Sprintf("%s_backup_%s", table, cfg.backupSuffix)
		log.Printf("backup %s -> %s", table, backup)
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE `%s`", backup, table)); err != nil {
			return err
		}
		where := "@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id"
		if table == "statistics_daily" {
			where = "statistic_type IN ('questionnaire', 'system') AND (" + where + ")"
		}
		if table == "statistics_accumulated" {
			where = "statistic_type IN ('questionnaire', 'system') AND (" + where + ")"
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("INSERT IGNORE INTO `%s` SELECT * FROM `%s` WHERE %s", backup, table, where)); err != nil {
			return err
		}
	}

	if cfg.clearPending {
		if err := backupPendingTables(ctx, conn, cfg); err != nil {
			return err
		}
	}
	return nil
}

func backupPendingTables(ctx context.Context, conn *sql.Conn, cfg config) error {
	pendingBackup := fmt.Sprintf("analytics_pending_event_backup_%s", cfg.backupSuffix)
	checkpointBackup := fmt.Sprintf("analytics_projector_checkpoint_backup_%s", cfg.backupSuffix)
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE analytics_pending_event", pendingBackup)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE analytics_projector_checkpoint", checkpointBackup)); err != nil {
		return err
	}

	predicate := pendingPredicate(cfg, "p")
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("INSERT IGNORE INTO `%s` SELECT p.* FROM analytics_pending_event p WHERE %s", pendingBackup, predicate)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`
INSERT IGNORE INTO %[1]s
SELECT c.*
FROM analytics_projector_checkpoint c
JOIN analytics_pending_event p ON p.event_id = c.event_id
WHERE %[2]s`, "`"+checkpointBackup+"`", predicate)); err != nil {
		return err
	}
	return nil
}

func pendingPredicate(cfg config, alias string) string {
	if cfg.allOrgs {
		return alias + ".deleted_at IS NULL"
	}
	return alias + ".deleted_at IS NULL AND JSON_VALID(" + alias + ".payload_json) AND CAST(JSON_UNQUOTE(JSON_EXTRACT(" + alias + ".payload_json, '$.org_id')) AS UNSIGNED) = @qs_rebuild_org_id"
}

func countPendingEvents(ctx context.Context, conn *sql.Conn, cfg config) (int64, error) {
	var count int64
	query := "SELECT COUNT(*) FROM analytics_pending_event p WHERE " + pendingPredicate(cfg, "p")
	if err := conn.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func clearAnalyticsPendingEvents(ctx context.Context, conn *sql.Conn, cfg config) error {
	log.Print("clear analytics pending events")
	predicate := pendingPredicate(cfg, "p")
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`
UPDATE analytics_projector_checkpoint c
JOIN analytics_pending_event p ON p.event_id = c.event_id
SET c.status = 'completed', c.updated_at = NOW(3)
WHERE %s`, predicate)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "DELETE p FROM analytics_pending_event p WHERE "+predicate); err != nil {
		return err
	}
	return nil
}

func rebuildAnalyticsProjections(ctx context.Context, conn *sql.Conn) error {
	log.Print("delete analytics projections")
	if _, err := conn.ExecContext(ctx, "DELETE FROM analytics_projection_org_daily WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM analytics_projection_clinician_daily WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM analytics_projection_entry_daily WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id"); err != nil {
		return err
	}

	log.Print("insert analytics_projection_org_daily")
	if _, err := conn.ExecContext(ctx, rebuildProjectionOrgDailySQL); err != nil {
		return err
	}
	log.Print("insert analytics_projection_clinician_daily")
	if _, err := conn.ExecContext(ctx, rebuildProjectionClinicianDailySQL); err != nil {
		return err
	}
	log.Print("insert analytics_projection_entry_daily")
	if _, err := conn.ExecContext(ctx, rebuildProjectionEntryDailySQL); err != nil {
		return err
	}
	return nil
}

func rebuildTraditionalStatistics(ctx context.Context, conn *sql.Conn) error {
	log.Print("delete statistics_daily")
	if _, err := conn.ExecContext(ctx, "DELETE FROM statistics_daily WHERE statistic_type IN ('questionnaire', 'system') AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)"); err != nil {
		return err
	}
	log.Print("insert statistics_daily questionnaire")
	if _, err := conn.ExecContext(ctx, rebuildDailyQuestionnaireSQL); err != nil {
		return err
	}
	log.Print("insert statistics_daily system")
	if _, err := conn.ExecContext(ctx, rebuildDailySystemSQL); err != nil {
		return err
	}

	log.Print("delete statistics_accumulated")
	if _, err := conn.ExecContext(ctx, "DELETE FROM statistics_accumulated WHERE statistic_type IN ('questionnaire', 'system') AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)"); err != nil {
		return err
	}
	log.Print("insert statistics_accumulated questionnaire")
	if _, err := conn.ExecContext(ctx, rebuildAccumulatedQuestionnaireSQL); err != nil {
		return err
	}
	log.Print("insert statistics_accumulated system")
	if _, err := conn.ExecContext(ctx, rebuildAccumulatedSystemSQL); err != nil {
		return err
	}

	log.Print("delete statistics_plan")
	if _, err := conn.ExecContext(ctx, "DELETE FROM statistics_plan WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id"); err != nil {
		return err
	}
	log.Print("insert statistics_plan")
	if _, err := conn.ExecContext(ctx, rebuildPlanSQL); err != nil {
		return err
	}
	return nil
}

func cleanupRedisStatsCache(ctx context.Context, cfg config) error {
	log.Printf("cleanup redis stats query cache addr=%s pattern=%s", cfg.redisAddr, cfg.redisKeyPattern)
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.redisAddr,
		Password: cfg.redisPassword,
		DB:       cfg.redisDB,
	})
	defer func() { _ = client.Close() }()
	if err := client.Ping(ctx).Err(); err != nil {
		return err
	}

	var cursor uint64
	var deleted int64
	for {
		keys, next, err := client.Scan(ctx, cursor, cfg.redisKeyPattern, 500).Result()
		if err != nil {
			return err
		}
		cursor = next
		if len(keys) > 0 {
			n, err := client.Del(ctx, keys...).Result()
			if err != nil {
				return err
			}
			deleted += n
		}
		if cursor == 0 {
			break
		}
	}
	log.Printf("redis stats query cache keys deleted: %d", deleted)
	return nil
}

const rebuildProjectionOrgDailySQL = `
INSERT INTO analytics_projection_org_daily (
  org_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count,
  created_at, updated_at
)
SELECT
  agg.org_id, agg.stat_date,
  SUM(agg.entry_opened_count),
  SUM(agg.intake_confirmed_count),
  SUM(agg.testee_profile_created_count),
  SUM(agg.care_relationship_established_count),
  SUM(agg.care_relationship_transferred_count),
  SUM(agg.answersheet_submitted_count),
  SUM(agg.assessment_created_count),
  SUM(agg.report_generated_count),
  SUM(agg.episode_completed_count),
  SUM(agg.episode_failed_count),
  NOW(3), NOW(3)
FROM (
  SELECT org_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'entry_opened' AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'intake_confirmed' AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'testee_profile_created' AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'care_relationship_established' AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'care_relationship_transferred' AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND submitted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND assessment_created_at IS NOT NULL AND assessment_created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND report_generated_at IS NOT NULL AND report_generated_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM assessment_episode WHERE deleted_at IS NULL AND status = 'failed' AND failed_at IS NOT NULL AND failed_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
) agg
GROUP BY agg.org_id, agg.stat_date
ON DUPLICATE KEY UPDATE
  entry_opened_count = VALUES(entry_opened_count),
  intake_confirmed_count = VALUES(intake_confirmed_count),
  testee_profile_created_count = VALUES(testee_profile_created_count),
  care_relationship_established_count = VALUES(care_relationship_established_count),
  care_relationship_transferred_count = VALUES(care_relationship_transferred_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  episode_completed_count = VALUES(episode_completed_count),
  episode_failed_count = VALUES(episode_failed_count),
  deleted_at = NULL,
  updated_at = NOW(3)`

const rebuildProjectionClinicianDailySQL = `
INSERT INTO analytics_projection_clinician_daily (
  org_id, clinician_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count,
  created_at, updated_at
)
SELECT
  agg.org_id, agg.clinician_id, agg.stat_date,
  SUM(agg.entry_opened_count),
  SUM(agg.intake_confirmed_count),
  SUM(agg.testee_profile_created_count),
  SUM(agg.care_relationship_established_count),
  SUM(agg.care_relationship_transferred_count),
  SUM(agg.answersheet_submitted_count),
  SUM(agg.assessment_created_count),
  SUM(agg.report_generated_count),
  SUM(agg.episode_completed_count),
  SUM(agg.episode_failed_count),
  NOW(3), NOW(3)
FROM (
  SELECT org_id, clinician_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'entry_opened' AND clinician_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'intake_confirmed' AND clinician_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'testee_profile_created' AND clinician_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'care_relationship_established' AND clinician_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'care_relationship_transferred' AND clinician_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND submitted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND assessment_created_at IS NOT NULL AND assessment_created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND report_generated_at IS NOT NULL AND report_generated_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, clinician_id, DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM assessment_episode WHERE deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND status = 'failed' AND failed_at IS NOT NULL AND failed_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
) agg
GROUP BY agg.org_id, agg.clinician_id, agg.stat_date
ON DUPLICATE KEY UPDATE
  entry_opened_count = VALUES(entry_opened_count),
  intake_confirmed_count = VALUES(intake_confirmed_count),
  testee_profile_created_count = VALUES(testee_profile_created_count),
  care_relationship_established_count = VALUES(care_relationship_established_count),
  care_relationship_transferred_count = VALUES(care_relationship_transferred_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  episode_completed_count = VALUES(episode_completed_count),
  episode_failed_count = VALUES(episode_failed_count),
  deleted_at = NULL,
  updated_at = NOW(3)`

const rebuildProjectionEntryDailySQL = `
INSERT INTO analytics_projection_entry_daily (
  org_id, entry_id, clinician_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count,
  created_at, updated_at
)
SELECT
  agg.org_id, agg.entry_id, MAX(agg.clinician_id), agg.stat_date,
  SUM(agg.entry_opened_count),
  SUM(agg.intake_confirmed_count),
  SUM(agg.testee_profile_created_count),
  SUM(agg.care_relationship_established_count),
  SUM(agg.care_relationship_transferred_count),
  SUM(agg.answersheet_submitted_count),
  SUM(agg.assessment_created_count),
  SUM(agg.report_generated_count),
  SUM(agg.episode_completed_count),
  SUM(agg.episode_failed_count),
  NOW(3), NOW(3)
FROM (
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'entry_opened' AND entry_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'intake_confirmed' AND entry_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'testee_profile_created' AND entry_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE deleted_at IS NULL AND event_name = 'care_relationship_established' AND entry_id <> 0 AND occurred_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND entry_id IS NOT NULL AND submitted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND entry_id IS NOT NULL AND assessment_created_at IS NOT NULL AND assessment_created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM assessment_episode WHERE deleted_at IS NULL AND entry_id IS NOT NULL AND report_generated_at IS NOT NULL AND report_generated_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM assessment_episode WHERE deleted_at IS NULL AND entry_id IS NOT NULL AND status = 'failed' AND failed_at IS NOT NULL AND failed_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
) agg
GROUP BY agg.org_id, agg.entry_id, agg.stat_date
ON DUPLICATE KEY UPDATE
  clinician_id = VALUES(clinician_id),
  entry_opened_count = VALUES(entry_opened_count),
  intake_confirmed_count = VALUES(intake_confirmed_count),
  testee_profile_created_count = VALUES(testee_profile_created_count),
  care_relationship_established_count = VALUES(care_relationship_established_count),
  care_relationship_transferred_count = VALUES(care_relationship_transferred_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  episode_completed_count = VALUES(episode_completed_count),
  episode_failed_count = VALUES(episode_failed_count),
  deleted_at = NULL,
  updated_at = NOW(3)`

const rebuildDailyQuestionnaireSQL = `
INSERT INTO statistics_daily (
  org_id, statistic_type, statistic_key, stat_date, submission_count, completion_count
)
SELECT agg.org_id, 'questionnaire', agg.statistic_key, agg.stat_date,
       SUM(agg.submission_count), SUM(agg.completion_count)
FROM (
  SELECT org_id, questionnaire_code AS statistic_key, DATE(created_at) AS stat_date, 1 AS submission_count, 0 AS completion_count
  FROM assessment
  WHERE deleted_at IS NULL AND questionnaire_code <> '' AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, questionnaire_code AS statistic_key, DATE(interpreted_at) AS stat_date, 0 AS submission_count, 1 AS completion_count
  FROM assessment
  WHERE deleted_at IS NULL AND questionnaire_code <> '' AND interpreted_at IS NOT NULL AND interpreted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
) agg
GROUP BY agg.org_id, agg.statistic_key, agg.stat_date
ON DUPLICATE KEY UPDATE
  submission_count = VALUES(submission_count),
  completion_count = VALUES(completion_count),
  deleted_at = NULL,
  updated_at = NOW()`

const rebuildDailySystemSQL = `
INSERT INTO statistics_daily (
  org_id, statistic_type, statistic_key, stat_date, submission_count, completion_count
)
SELECT agg.org_id, 'system', 'system', agg.stat_date,
       SUM(agg.submission_count), SUM(agg.completion_count)
FROM (
  SELECT org_id, DATE(created_at) AS stat_date, 1 AS submission_count, 0 AS completion_count
  FROM assessment
  WHERE deleted_at IS NULL AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  UNION ALL
  SELECT org_id, DATE(interpreted_at) AS stat_date, 0 AS submission_count, 1 AS completion_count
  FROM assessment
  WHERE deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
) agg
GROUP BY agg.org_id, agg.stat_date
ON DUPLICATE KEY UPDATE
  submission_count = VALUES(submission_count),
  completion_count = VALUES(completion_count),
  deleted_at = NULL,
  updated_at = NOW()`

const rebuildAccumulatedQuestionnaireSQL = `
INSERT INTO statistics_accumulated (
  org_id, statistic_type, statistic_key,
  total_submissions, total_completions,
  last7d_submissions, last15d_submissions, last30d_submissions,
  distribution, first_occurred_at, last_occurred_at
)
SELECT
  d.org_id,
  'questionnaire',
  d.statistic_key,
  d.total_submissions,
  d.total_completions,
  d.last7d_submissions,
  d.last15d_submissions,
  d.last30d_submissions,
  JSON_OBJECT('origin', COALESCE(o.origin_json, JSON_OBJECT())),
  b.first_occurred_at,
  b.last_occurred_at
FROM (
  SELECT
    org_id,
    statistic_key,
    SUM(submission_count) AS total_submissions,
    SUM(completion_count) AS total_completions,
    SUM(CASE WHEN stat_date >= DATE_SUB(DATE(@qs_rebuild_cutoff), INTERVAL 7 DAY) THEN submission_count ELSE 0 END) AS last7d_submissions,
    SUM(CASE WHEN stat_date >= DATE_SUB(DATE(@qs_rebuild_cutoff), INTERVAL 15 DAY) THEN submission_count ELSE 0 END) AS last15d_submissions,
    SUM(CASE WHEN stat_date >= DATE_SUB(DATE(@qs_rebuild_cutoff), INTERVAL 30 DAY) THEN submission_count ELSE 0 END) AS last30d_submissions
  FROM statistics_daily
  WHERE statistic_type = 'questionnaire' AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  GROUP BY org_id, statistic_key
) d
LEFT JOIN (
  SELECT questionnaire_code, org_id, JSON_OBJECTAGG(origin_type, cnt) AS origin_json
  FROM (
    SELECT
      org_id,
      questionnaire_code COLLATE utf8mb4_unicode_ci AS questionnaire_code,
      COALESCE(NULLIF(origin_type COLLATE utf8mb4_unicode_ci, ''), 'unknown') AS origin_type,
      COUNT(*) AS cnt
    FROM assessment
    WHERE deleted_at IS NULL AND questionnaire_code <> '' AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
    GROUP BY org_id, questionnaire_code COLLATE utf8mb4_unicode_ci, COALESCE(NULLIF(origin_type COLLATE utf8mb4_unicode_ci, ''), 'unknown')
  ) x
  GROUP BY org_id, questionnaire_code
) o ON o.org_id = d.org_id AND o.questionnaire_code COLLATE utf8mb4_unicode_ci = d.statistic_key COLLATE utf8mb4_unicode_ci
LEFT JOIN (
  SELECT org_id, questionnaire_code COLLATE utf8mb4_unicode_ci AS questionnaire_code, MIN(created_at) AS first_occurred_at, MAX(created_at) AS last_occurred_at
  FROM assessment
  WHERE deleted_at IS NULL AND questionnaire_code <> '' AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  GROUP BY org_id, questionnaire_code COLLATE utf8mb4_unicode_ci
) b ON b.org_id = d.org_id AND b.questionnaire_code COLLATE utf8mb4_unicode_ci = d.statistic_key COLLATE utf8mb4_unicode_ci
ON DUPLICATE KEY UPDATE
  total_submissions = VALUES(total_submissions),
  total_completions = VALUES(total_completions),
  last7d_submissions = VALUES(last7d_submissions),
  last15d_submissions = VALUES(last15d_submissions),
  last30d_submissions = VALUES(last30d_submissions),
  distribution = VALUES(distribution),
  first_occurred_at = VALUES(first_occurred_at),
  last_occurred_at = VALUES(last_occurred_at),
  last_updated_at = NOW(),
  deleted_at = NULL`

const rebuildAccumulatedSystemSQL = `
INSERT INTO statistics_accumulated (
  org_id, statistic_type, statistic_key,
  total_submissions, total_completions,
  distribution, first_occurred_at, last_occurred_at
)
SELECT
  orgs.org_id,
  'system',
  'system',
  COALESCE(a.assessment_count, 0),
  COALESCE(c.completion_count, 0),
  JSON_OBJECT(
    'status', COALESCE(s.status_json, JSON_OBJECT()),
    'testee_count', COALESCE(t.testee_count, 0)
  ),
  b.first_occurred_at,
  b.last_occurred_at
FROM (
  SELECT DISTINCT org_id FROM assessment WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id
  UNION
  SELECT DISTINCT org_id FROM testee WHERE @qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id
) orgs
LEFT JOIN (
  SELECT org_id, COUNT(*) AS assessment_count
  FROM assessment
  WHERE deleted_at IS NULL AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  GROUP BY org_id
) a ON a.org_id = orgs.org_id
LEFT JOIN (
  SELECT org_id, COUNT(*) AS completion_count
  FROM assessment
  WHERE deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  GROUP BY org_id
) c ON c.org_id = orgs.org_id
LEFT JOIN (
  SELECT org_id, JSON_OBJECTAGG(status, cnt) AS status_json
  FROM (
    SELECT org_id, status, COUNT(*) AS cnt
    FROM assessment
    WHERE deleted_at IS NULL AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
    GROUP BY org_id, status
  ) x
  GROUP BY org_id
) s ON s.org_id = orgs.org_id
LEFT JOIN (
  SELECT org_id, COUNT(*) AS testee_count
  FROM testee
  WHERE deleted_at IS NULL AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  GROUP BY org_id
) t ON t.org_id = orgs.org_id
LEFT JOIN (
  SELECT org_id, MIN(created_at) AS first_occurred_at, MAX(created_at) AS last_occurred_at
  FROM assessment
  WHERE deleted_at IS NULL AND created_at < @qs_rebuild_cutoff AND (@qs_rebuild_org_id = 0 OR org_id = @qs_rebuild_org_id)
  GROUP BY org_id
) b ON b.org_id = orgs.org_id
ON DUPLICATE KEY UPDATE
  total_submissions = VALUES(total_submissions),
  total_completions = VALUES(total_completions),
  distribution = VALUES(distribution),
  first_occurred_at = VALUES(first_occurred_at),
  last_occurred_at = VALUES(last_occurred_at),
  last_updated_at = NOW(),
  deleted_at = NULL`

const rebuildPlanSQL = `
INSERT INTO statistics_plan (
  org_id, plan_id,
  total_tasks, completed_tasks, pending_tasks, expired_tasks,
  enrolled_testees, active_testees
)
SELECT
  p.org_id,
  p.id AS plan_id,
  COUNT(t.id) AS total_tasks,
  COALESCE(SUM(CASE WHEN t.status = 'completed' THEN 1 ELSE 0 END), 0) AS completed_tasks,
  COALESCE(SUM(CASE WHEN t.status IN ('pending', 'opened') THEN 1 ELSE 0 END), 0) AS pending_tasks,
  COALESCE(SUM(CASE WHEN t.status = 'expired' THEN 1 ELSE 0 END), 0) AS expired_tasks,
  COUNT(DISTINCT t.testee_id) AS enrolled_testees,
  COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.testee_id END) AS active_testees
FROM assessment_plan p
LEFT JOIN assessment_task t
  ON t.org_id = p.org_id
 AND t.plan_id = p.id
 AND t.deleted_at IS NULL
WHERE p.deleted_at IS NULL AND (@qs_rebuild_org_id = 0 OR p.org_id = @qs_rebuild_org_id)
GROUP BY p.org_id, p.id
ON DUPLICATE KEY UPDATE
  total_tasks = VALUES(total_tasks),
  completed_tasks = VALUES(completed_tasks),
  pending_tasks = VALUES(pending_tasks),
  expired_tasks = VALUES(expired_tasks),
  enrolled_testees = VALUES(enrolled_testees),
  active_testees = VALUES(active_testees),
  last_updated_at = NOW(),
  deleted_at = NULL`
