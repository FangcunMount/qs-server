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

// repair_testee_profile_created_footprints reconciles behavior_footprint
// testee_profile_created rows with the testee table.
//
// Product semantics: "new testee profile" means an active testee row was
// created. The behavior footprint must therefore have one active
// testee_profile_created row per active testee in the selected scope.
//
// Typical usage:
//
//	go run scripts/oneoff/repair_testee_profile_created_footprints/repair_testee_profile_created_footprints.go \
//	  --mysql-dsn 'app_user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true' \
//	  --org-id 1
//
// Re-run with --apply after reviewing the dry-run output. Run the statistics
// rebuild script afterwards so analytics projections consume the repaired
// behavior facts.
type config struct {
	mysqlDSN              string
	orgID                 int64
	allOrgs               bool
	createdStartValue     string
	createdStart          time.Time
	hasCreatedStart       bool
	cutoffDate            string
	cutoff                time.Time
	relationWindowSeconds int
	backupSuffix          string
	timeout               time.Duration
	previewLimit          int
	apply                 bool
	skipBackup            bool
}

type repairSummary struct {
	SourceTestees                int64
	SourceWithAttribution        int64
	TargetActiveFootprints       int64
	NonCanonicalToSoftDelete     int64
	CanonicalMissing             int64
	CanonicalActiveExisting      int64
	CanonicalDeletedExisting     int64
	ActiveFootprintsForSource    int64
	ActiveSourceTesteeDuplicates int64
	SourceMinOccurredAt          sql.NullTime
	SourceMaxOccurredAt          sql.NullTime
}

type sourcePreviewRow struct {
	OrgID               int64
	TesteeID            uint64
	OccurredAt          time.Time
	ClinicianID         uint64
	EntryID             uint64
	ExistingFootprintID sql.NullString
	RelationID          sql.NullInt64
}

type targetPreviewRow struct {
	ID          string
	OrgID       int64
	TesteeID    uint64
	OccurredAt  time.Time
	ClinicianID uint64
	EntryID     uint64
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
	if err := prepareRepairScope(ctx, conn); err != nil {
		log.Fatalf("prepare repair scope: %v", err)
	}

	summary, err := loadRepairSummary(ctx, conn)
	if err != nil {
		log.Fatalf("load repair summary: %v", err)
	}
	printSummary(cfg, summary)
	if summary.SourceTestees == 0 && summary.TargetActiveFootprints == 0 {
		log.Print("scope is empty; nothing to repair")
		return
	}

	if !cfg.apply {
		if err := printPreview(ctx, conn, cfg.previewLimit); err != nil {
			log.Fatalf("print preview: %v", err)
		}
		log.Print("dry-run only; re-run with --apply to reconcile testee_profile_created behavior footprints")
		return
	}

	if !cfg.skipBackup {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
		if err := backupBehaviorFootprints(ctx, conn, cfg.backupSuffix); err != nil {
			log.Fatalf("backup behavior_footprint: %v", err)
		}
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("begin repair transaction: %v", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	softDeleted, err := softDeleteNonCanonicalFootprints(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		committed = true
		log.Fatalf("soft-delete non-canonical footprints: %v", err)
	}
	upsertAffected, err := upsertCanonicalFootprints(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		committed = true
		log.Fatalf("upsert canonical footprints: %v", err)
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		committed = true
		log.Fatalf("commit repair transaction: %v", err)
	}
	committed = true

	log.Printf("repair completed: source_testees=%d soft_deleted_noncanonical=%d canonical_upsert_rows_affected=%d",
		summary.SourceTestees, softDeleted, upsertAffected)
	log.Print("run scripts/oneoff/rebuild_statistic/rebuild_statistics.go afterwards to refresh analytics_projection_* and cached statistics")
}

func parseFlags() config {
	var cfg config
	defaultCutoff := time.Now().In(time.Local).AddDate(0, 0, 1).Format("2006-01-02")
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to repair; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "repair all organizations")
	flag.StringVar(&cfg.createdStartValue, "created-start", "", "optional inclusive testee.created_at lower bound, format YYYY-MM-DD or YYYY-MM-DD HH:MM:SS")
	flag.StringVar(&cfg.cutoffDate, "cutoff-date", defaultCutoff, "exclusive testee.created_at upper date bound, format YYYY-MM-DD; default is tomorrow local date")
	flag.IntVar(&cfg.relationWindowSeconds, "relation-window-seconds", 5, "seconds used to recover entry/clinician attribution from creator assessment_entry relations")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup table")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout, e.g. 30m, 2h")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of rows to preview in dry-run")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup table creation before applying changes")
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
	if cfg.relationWindowSeconds < 0 {
		log.Fatal("--relation-window-seconds must be >= 0")
	}
	if cfg.previewLimit < 0 {
		log.Fatal("--preview-limit must be >= 0")
	}

	cutoff, err := time.ParseInLocation("2006-01-02", cfg.cutoffDate, time.Local)
	if err != nil {
		log.Fatalf("invalid --cutoff-date: %v", err)
	}
	cfg.cutoff = cutoff

	if strings.TrimSpace(cfg.createdStartValue) != "" {
		createdStart, err := parseTimeBound(cfg.createdStartValue)
		if err != nil {
			log.Fatalf("invalid --created-start: %v", err)
		}
		cfg.createdStart = createdStart
		cfg.hasCreatedStart = true
	}
	return cfg
}

func parseTimeBound(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or YYYY-MM-DD HH:MM:SS, got %q", value)
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
	var createdStart interface{}
	if cfg.hasCreatedStart {
		createdStart = cfg.createdStart.Format("2006-01-02 15:04:05.000")
	}
	_, err := conn.ExecContext(
		ctx,
		"SET @qs_repair_org_id := ?, @qs_repair_cutoff := ?, @qs_repair_created_start := ?, @qs_repair_relation_window_seconds := ?",
		scopeOrgID(cfg),
		cfg.cutoff.Format("2006-01-02"),
		createdStart,
		cfg.relationWindowSeconds,
	)
	return err
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

func prepareRepairScope(ctx context.Context, conn *sql.Conn) error {
	statements := []string{
		"DROP TEMPORARY TABLE IF EXISTS repair_testee_profile_created_source",
		"DROP TEMPORARY TABLE IF EXISTS repair_testee_profile_created_existing",
		"DROP TEMPORARY TABLE IF EXISTS repair_testee_profile_created_relation",
		"DROP TEMPORARY TABLE IF EXISTS repair_testee_profile_created_target",
		createSourceTableSQL,
		insertSourceSQL,
		createExistingTableSQL,
		insertExistingTableSQL,
		updateSourceFromExistingSQL,
		createRelationTableSQL,
		insertRelationTableSQL,
		updateSourceFromRelationSQL,
		createTargetTableSQL,
		insertTargetByOccurredAtSQL,
		insertTargetBySourceTesteeSQL,
	}
	for _, stmt := range statements {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func loadRepairSummary(ctx context.Context, conn *sql.Conn) (repairSummary, error) {
	var summary repairSummary
	if err := conn.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS source_testees,
  COALESCE(SUM(CASE WHEN clinician_id <> 0 OR entry_id <> 0 THEN 1 ELSE 0 END), 0) AS source_with_attribution,
  MIN(occurred_at) AS min_occurred_at,
  MAX(occurred_at) AS max_occurred_at
FROM repair_testee_profile_created_source`).Scan(
		&summary.SourceTestees,
		&summary.SourceWithAttribution,
		&summary.SourceMinOccurredAt,
		&summary.SourceMaxOccurredAt,
	); err != nil {
		return summary, err
	}

	counts, err := loadCounts(ctx, conn, []string{
		"SELECT COUNT(*) FROM repair_testee_profile_created_target",
		"SELECT COUNT(*) FROM repair_testee_profile_created_target t LEFT JOIN repair_testee_profile_created_source s ON s.id = t.id WHERE s.id IS NULL",
		"SELECT COUNT(*) FROM repair_testee_profile_created_source s LEFT JOIN behavior_footprint bf ON bf.id = s.id WHERE bf.id IS NULL",
		"SELECT COUNT(*) FROM repair_testee_profile_created_source s JOIN behavior_footprint bf ON bf.id = s.id WHERE bf.deleted_at IS NULL",
		"SELECT COUNT(*) FROM repair_testee_profile_created_source s JOIN behavior_footprint bf ON bf.id = s.id WHERE bf.deleted_at IS NOT NULL",
		"SELECT COUNT(*) FROM behavior_footprint bf JOIN repair_testee_profile_created_source s ON s.org_id = bf.org_id AND s.testee_id = bf.testee_id WHERE bf.deleted_at IS NULL AND bf.event_name = 'testee_profile_created'",
		"SELECT COUNT(*) FROM (SELECT bf.org_id, bf.testee_id FROM behavior_footprint bf JOIN repair_testee_profile_created_source s ON s.org_id = bf.org_id AND s.testee_id = bf.testee_id WHERE bf.deleted_at IS NULL AND bf.event_name = 'testee_profile_created' GROUP BY bf.org_id, bf.testee_id HAVING COUNT(*) > 1) duplicate_source_testees",
	})
	if err != nil {
		return summary, err
	}
	summary.TargetActiveFootprints = counts[0]
	summary.NonCanonicalToSoftDelete = counts[1]
	summary.CanonicalMissing = counts[2]
	summary.CanonicalActiveExisting = counts[3]
	summary.CanonicalDeletedExisting = counts[4]
	summary.ActiveFootprintsForSource = counts[5]
	summary.ActiveSourceTesteeDuplicates = counts[6]
	return summary, nil
}

func loadCounts(ctx context.Context, conn *sql.Conn, queries []string) ([]int64, error) {
	counts := make([]int64, 0, len(queries))
	for _, query := range queries {
		var count int64
		if err := conn.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, nil
}

func printSummary(cfg config, summary repairSummary) {
	log.Printf("scope: %s created_start=%q cutoff_date(exclusive)=%s apply=%v backup=%v",
		scopeDescription(cfg), cfg.createdStartValue, cfg.cutoff.Format("2006-01-02"), cfg.apply, !cfg.skipBackup)
	if summary.SourceMinOccurredAt.Valid && summary.SourceMaxOccurredAt.Valid {
		log.Printf("source testee.created_at range: min=%s max=%s",
			summary.SourceMinOccurredAt.Time.Format("2006-01-02 15:04:05.000"),
			summary.SourceMaxOccurredAt.Time.Format("2006-01-02 15:04:05.000"))
	}
	log.Printf("source active testees: total=%d with_entry_or_clinician_attribution=%d",
		summary.SourceTestees, summary.SourceWithAttribution)
	log.Printf("existing active testee_profile_created footprints in repair target: total=%d active_for_source_testees=%d duplicate_source_testees=%d",
		summary.TargetActiveFootprints, summary.ActiveFootprintsForSource, summary.ActiveSourceTesteeDuplicates)
	log.Printf("planned reconciliation: canonical_missing=%d canonical_active_existing=%d canonical_deleted_existing=%d noncanonical_active_to_soft_delete=%d",
		summary.CanonicalMissing, summary.CanonicalActiveExisting, summary.CanonicalDeletedExisting, summary.NonCanonicalToSoftDelete)
}

func printPreview(ctx context.Context, conn *sql.Conn, limit int) error {
	if limit == 0 {
		return nil
	}
	sourceRows, err := loadSourcePreview(ctx, conn, limit)
	if err != nil {
		return err
	}
	if len(sourceRows) > 0 {
		log.Printf("source preview, latest %d rows:", len(sourceRows))
		for _, row := range sourceRows {
			log.Printf("  org_id=%d testee_id=%d occurred_at=%s clinician_id=%d entry_id=%d existing_footprint_id=%s relation_id=%s",
				row.OrgID,
				row.TesteeID,
				row.OccurredAt.Format("2006-01-02 15:04:05.000"),
				row.ClinicianID,
				row.EntryID,
				nullString(row.ExistingFootprintID),
				nullInt(row.RelationID),
			)
		}
	}

	targetRows, err := loadTargetPreview(ctx, conn, limit)
	if err != nil {
		return err
	}
	if len(targetRows) > 0 {
		log.Printf("non-canonical active footprint preview, latest %d rows to soft-delete:", len(targetRows))
		for _, row := range targetRows {
			log.Printf("  id=%s org_id=%d testee_id=%d occurred_at=%s clinician_id=%d entry_id=%d",
				row.ID,
				row.OrgID,
				row.TesteeID,
				row.OccurredAt.Format("2006-01-02 15:04:05.000"),
				row.ClinicianID,
				row.EntryID,
			)
		}
	}
	return nil
}

func loadSourcePreview(ctx context.Context, conn *sql.Conn, limit int) ([]sourcePreviewRow, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT org_id, testee_id, occurred_at, clinician_id, entry_id, existing_footprint_id, relation_id
FROM repair_testee_profile_created_source
ORDER BY occurred_at DESC, testee_id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []sourcePreviewRow
	for rows.Next() {
		var row sourcePreviewRow
		if err := rows.Scan(
			&row.OrgID,
			&row.TesteeID,
			&row.OccurredAt,
			&row.ClinicianID,
			&row.EntryID,
			&row.ExistingFootprintID,
			&row.RelationID,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func loadTargetPreview(ctx context.Context, conn *sql.Conn, limit int) ([]targetPreviewRow, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT bf.id, bf.org_id, bf.testee_id, bf.occurred_at, bf.clinician_id, bf.entry_id
FROM behavior_footprint bf
JOIN repair_testee_profile_created_target t ON t.id = bf.id
LEFT JOIN repair_testee_profile_created_source s ON s.id = bf.id
WHERE s.id IS NULL
ORDER BY bf.occurred_at DESC, bf.id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []targetPreviewRow
	for rows.Next() {
		var row targetPreviewRow
		if err := rows.Scan(
			&row.ID,
			&row.OrgID,
			&row.TesteeID,
			&row.OccurredAt,
			&row.ClinicianID,
			&row.EntryID,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func backupBehaviorFootprints(ctx context.Context, conn *sql.Conn, suffix string) error {
	tableName := "repair_bak_bf_tpc_" + suffix
	query := fmt.Sprintf(`
CREATE TABLE %s AS
SELECT bf.*
FROM behavior_footprint bf
LEFT JOIN repair_testee_profile_created_target t ON t.id = bf.id
LEFT JOIN repair_testee_profile_created_source s ON s.id = bf.id
WHERE t.id IS NOT NULL OR s.id IS NOT NULL`, tableName)
	result, err := conn.ExecContext(ctx, query)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	log.Printf("backup table created: %s rows=%d", tableName, count)
	return nil
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func softDeleteNonCanonicalFootprints(ctx context.Context, execer sqlExecer) (int64, error) {
	result, err := execer.ExecContext(ctx, softDeleteNonCanonicalSQL)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func upsertCanonicalFootprints(ctx context.Context, execer sqlExecer) (int64, error) {
	result, err := execer.ExecContext(ctx, upsertCanonicalSQL)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func validateBackupSuffix(suffix string) error {
	if suffix == "" {
		return fmt.Errorf("backup suffix is empty")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(suffix) {
		return fmt.Errorf("backup suffix must contain only letters, numbers, and underscores")
	}
	return nil
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return "NULL"
	}
	return value.String
}

func nullInt(value sql.NullInt64) string {
	if !value.Valid {
		return "NULL"
	}
	return fmt.Sprintf("%d", value.Int64)
}

const createSourceTableSQL = `
CREATE TEMPORARY TABLE repair_testee_profile_created_source (
  id VARCHAR(128) NOT NULL,
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  occurred_at DATETIME(3) NOT NULL,
  entry_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  clinician_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  source_clinician_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  existing_footprint_id VARCHAR(128) NULL,
  relation_id BIGINT UNSIGNED NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uniq_source_org_testee (org_id, testee_id),
  KEY idx_source_org_time (org_id, occurred_at)
) ENGINE=InnoDB`

const insertSourceSQL = `
INSERT INTO repair_testee_profile_created_source (
  id, org_id, testee_id, occurred_at
)
SELECT
  CONCAT('repair:testee_profile_created:testee:', t.id),
  t.org_id,
  t.id,
  t.created_at
FROM testee t
WHERE t.deleted_at IS NULL
  AND t.created_at < @qs_repair_cutoff
  AND (@qs_repair_created_start IS NULL OR t.created_at >= @qs_repair_created_start)
  AND (@qs_repair_org_id = 0 OR t.org_id = @qs_repair_org_id)`

const createExistingTableSQL = `
CREATE TEMPORARY TABLE repair_testee_profile_created_existing (
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  footprint_id VARCHAR(128) NOT NULL,
  PRIMARY KEY (org_id, testee_id)
) ENGINE=InnoDB`

const insertExistingTableSQL = `
INSERT INTO repair_testee_profile_created_existing (
  org_id, testee_id, footprint_id
)
SELECT
  bf.org_id,
  bf.testee_id,
  COALESCE(
    MIN(CASE WHEN bf.entry_id <> 0 OR bf.clinician_id <> 0 THEN bf.id ELSE NULL END),
    MIN(bf.id)
  ) AS footprint_id
FROM behavior_footprint bf
JOIN repair_testee_profile_created_source s
  ON s.org_id = bf.org_id
 AND s.testee_id = bf.testee_id
WHERE bf.deleted_at IS NULL
  AND bf.event_name = 'testee_profile_created'
GROUP BY bf.org_id, bf.testee_id`

const updateSourceFromExistingSQL = `
UPDATE repair_testee_profile_created_source s
JOIN repair_testee_profile_created_existing e
  ON e.org_id = s.org_id
 AND e.testee_id = s.testee_id
JOIN behavior_footprint bf
  ON bf.id = e.footprint_id
SET
  s.existing_footprint_id = bf.id,
  s.entry_id = bf.entry_id,
  s.clinician_id = bf.clinician_id,
  s.source_clinician_id = bf.source_clinician_id`

const createRelationTableSQL = `
CREATE TEMPORARY TABLE repair_testee_profile_created_relation (
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  relation_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (org_id, testee_id)
) ENGINE=InnoDB`

const insertRelationTableSQL = `
INSERT INTO repair_testee_profile_created_relation (
  org_id, testee_id, relation_id
)
SELECT
  s.org_id,
  s.testee_id,
  MIN(r.id) AS relation_id
FROM repair_testee_profile_created_source s
JOIN clinician_relation r
  ON r.org_id = s.org_id
 AND r.testee_id = s.testee_id
WHERE s.clinician_id = 0
  AND s.entry_id = 0
  AND r.deleted_at IS NULL
  AND r.relation_type COLLATE utf8mb4_unicode_ci = 'creator' COLLATE utf8mb4_unicode_ci
  AND r.source_type COLLATE utf8mb4_unicode_ci = 'assessment_entry' COLLATE utf8mb4_unicode_ci
  AND r.source_id IS NOT NULL
  AND ABS(TIMESTAMPDIFF(SECOND, s.occurred_at, r.bound_at)) <= @qs_repair_relation_window_seconds
GROUP BY s.org_id, s.testee_id`

const updateSourceFromRelationSQL = `
UPDATE repair_testee_profile_created_source s
JOIN repair_testee_profile_created_relation rr
  ON rr.org_id = s.org_id
 AND rr.testee_id = s.testee_id
JOIN clinician_relation r
  ON r.id = rr.relation_id
SET
  s.relation_id = r.id,
  s.entry_id = COALESCE(r.source_id, 0),
  s.clinician_id = r.clinician_id`

const createTargetTableSQL = `
CREATE TEMPORARY TABLE repair_testee_profile_created_target (
  id VARCHAR(128) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB`

const insertTargetByOccurredAtSQL = `
INSERT IGNORE INTO repair_testee_profile_created_target (id)
SELECT bf.id
FROM behavior_footprint bf
WHERE bf.deleted_at IS NULL
  AND bf.event_name = 'testee_profile_created'
  AND bf.occurred_at < @qs_repair_cutoff
  AND (@qs_repair_created_start IS NULL OR bf.occurred_at >= @qs_repair_created_start)
  AND (@qs_repair_org_id = 0 OR bf.org_id = @qs_repair_org_id)`

const insertTargetBySourceTesteeSQL = `
INSERT IGNORE INTO repair_testee_profile_created_target (id)
SELECT bf.id
FROM behavior_footprint bf
JOIN repair_testee_profile_created_source s
  ON s.org_id = bf.org_id
 AND s.testee_id = bf.testee_id
WHERE bf.deleted_at IS NULL
  AND bf.event_name = 'testee_profile_created'`

const softDeleteNonCanonicalSQL = `
UPDATE behavior_footprint bf
JOIN repair_testee_profile_created_target t
  ON t.id = bf.id
LEFT JOIN repair_testee_profile_created_source s
  ON s.id = bf.id
SET
  bf.deleted_at = NOW(3),
  bf.updated_at = NOW(3)
WHERE s.id IS NULL
  AND bf.deleted_at IS NULL`

const upsertCanonicalSQL = `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at, deleted_at
)
SELECT
  s.id,
  s.org_id,
  'testee',
  s.testee_id,
  CASE WHEN s.clinician_id <> 0 THEN 'clinician' ELSE 'system' END,
  CASE WHEN s.clinician_id <> 0 THEN s.clinician_id ELSE 0 END,
  s.entry_id,
  s.clinician_id,
  s.source_clinician_id,
  s.testee_id,
  0,
  0,
  0,
  'testee_profile_created',
  s.occurred_at,
  JSON_OBJECT(
    'repair_source', 'testee',
    'testee_id', s.testee_id,
    'existing_footprint_id', s.existing_footprint_id,
    'relation_id', s.relation_id
  ),
  NOW(3),
  NOW(3),
  NULL
FROM repair_testee_profile_created_source s
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
