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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mysqlDSN           string
	mongoURI           string
	mongoDatabase      string
	orgID              int64
	allOrgs            bool
	sourceCreatedStart string
	sourceCreatedEnd   string
	backupSuffix       string
	timeout            time.Duration
	apply              bool
	skipBackup         bool
	previewLimit       int
	batchSize          int
}

type orphanRefRow struct {
	QueueID         uint64
	SourceTable     string
	SourceStringID  string
	SourceUintID    uint64
	OrgID           int64
	TesteeID        uint64
	AssessmentID    uint64
	AnswerSheetID   uint64
	ReportID        uint64
	SourceCreatedAt sql.NullTime
}

type cleanupCounts struct {
	BehaviorFootprints int64
	AssessmentEpisodes int64
	MongoAnswersheets  int64
	MongoReports       int64
}

type queueSummary struct {
	TotalRefs             int
	BehaviorFootprintRefs int
	AssessmentEpisodeRefs int
	MongoAnswerSheetIDs   int
	MongoReportIDs        int
}

// cleanup_deleted_assessment_orphans scans assessment-related runtime rows whose
// assessment foreign keys no longer resolve to the physically deleted assessment row.
//
// It uses MySQL behavior_footprint and assessment_episode as the source of truth
// for orphan references, then soft-deletes matching MySQL rows plus MongoDB
// answersheets / interpret_reports derived from those orphan references.
//
// Typical usage:
//
//	go run scripts/oneoff/cleanup_deleted_assessment_orphans.go \
//	  --mysql-dsn 'app_user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true' \
//	  --mongo-uri 'mongodb://127.0.0.1:27017' \
//	  --mongo-db qs \
//	  --org-id 1
//
// Re-run with --apply after reviewing the dry-run output.
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

	summary, err := prepareOrphanRefQueue(ctx, conn, cfg)
	if err != nil {
		log.Fatalf("prepare orphan ref queue: %v", err)
	}
	log.Printf("scope: %s source_created_start=%q source_created_end=%q apply=%v backup=%v",
		scopeDescription(cfg), cfg.sourceCreatedStart, cfg.sourceCreatedEnd, cfg.apply, !cfg.skipBackup)
	log.Printf("candidate orphan refs: total=%d behavior_footprint=%d assessment_episode=%d derived_mongo_answersheet_ids=%d derived_mongo_report_ids=%d",
		summary.TotalRefs, summary.BehaviorFootprintRefs, summary.AssessmentEpisodeRefs, summary.MongoAnswerSheetIDs, summary.MongoReportIDs)
	if summary.TotalRefs == 0 {
		log.Print("scope is empty; nothing to clean")
		return
	}

	if !cfg.apply {
		rows, err := loadQueuedOrphanRefs(ctx, conn, cfg)
		if err != nil {
			log.Fatalf("load preview scope: %v", err)
		}
		if err := prepareCleanupScope(ctx, conn, rows); err != nil {
			log.Fatalf("prepare preview cleanup scope: %v", err)
		}
		mysqlCounts, err := countMySQLOrphansInScope(ctx, conn)
		if err != nil {
			log.Fatalf("count mysql orphans: %v", err)
		}
		mongoCounts, err := countMongoOrphans(ctx, mongoDB, rows)
		if err != nil {
			log.Fatalf("count mongo orphans: %v", err)
		}
		printPreview(rows, cfg.previewLimit, addCounts(mysqlCounts, mongoCounts))
		log.Print("dry-run only; re-run with --apply to soft-delete orphan MySQL rows and MongoDB documents")
		return
	}

	if !cfg.skipBackup {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
	}

	totalProcessed := 0
	batchNumber := 0
	totalCounts := cleanupCounts{}
	for {
		rows, err := loadQueuedOrphanRefs(ctx, conn, cfg)
		if err != nil {
			log.Fatalf("load queued orphan refs: %v", err)
		}
		if len(rows) == 0 {
			printProgressBar(totalProcessed, summary.TotalRefs)
			log.Printf("cleanup completed: orphan_refs_scanned=%d behavior_footprints=%d assessment_episodes=%d mongo_answersheets=%d mongo_reports=%d",
				totalProcessed, totalCounts.BehaviorFootprints, totalCounts.AssessmentEpisodes, totalCounts.MongoAnswersheets, totalCounts.MongoReports)
			return
		}

		batchNumber++
		batchCfg := cfg
		batchCfg.backupSuffix = backupSuffixForBatch(cfg.backupSuffix, batchNumber)
		log.Printf("processing batch: number=%d orphan_refs=%d backup_suffix=%s", batchNumber, len(rows), batchCfg.backupSuffix)

		counts, err := processBatch(ctx, conn, mongoDB, rows, batchCfg)
		if err != nil {
			log.Fatalf("process batch %d: %v", batchNumber, err)
		}
		if err := markQueueProcessed(ctx, conn); err != nil {
			log.Fatalf("mark queue processed for batch %d: %v", batchNumber, err)
		}

		totalProcessed += len(rows)
		totalCounts = addCounts(totalCounts, counts)
		log.Printf("batch cleaned: behavior_footprints=%d assessment_episodes=%d mongo_answersheets=%d mongo_reports=%d",
			counts.BehaviorFootprints, counts.AssessmentEpisodes, counts.MongoAnswersheets, counts.MongoReports)
		printProgressBar(totalProcessed, summary.TotalRefs)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", "", "MongoDB URI")
	flag.StringVar(&cfg.mongoDatabase, "mongo-db", "", "MongoDB database name")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to clean; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "clean all organizations")
	flag.StringVar(&cfg.sourceCreatedStart, "source-created-start", "", "optional inclusive behavior_footprint/assessment_episode created_at lower bound, format 2006-01-02 15:04:05")
	flag.StringVar(&cfg.sourceCreatedEnd, "source-created-end", "", "optional exclusive behavior_footprint/assessment_episode created_at upper bound, format 2006-01-02 15:04:05")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables/collections")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout, e.g. 30m, 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup table/collection creation before applying changes")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of orphan refs to preview in dry-run")
	flag.IntVar(&cfg.batchSize, "batch-size", 1000, "number of orphan refs to scan per batch")
	flag.Parse()

	required := map[string]string{
		"--mysql-dsn": cfg.mysqlDSN,
		"--mongo-uri": cfg.mongoURI,
		"--mongo-db":  cfg.mongoDatabase,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			log.Fatalf("%s is required", name)
		}
	}
	if cfg.allOrgs && cfg.orgID > 0 {
		log.Fatal("--org-id and --all-orgs are mutually exclusive")
	}
	if !cfg.allOrgs && cfg.orgID <= 0 {
		log.Fatal("one of --org-id or --all-orgs is required")
	}
	if cfg.batchSize <= 0 {
		log.Fatal("--batch-size must be greater than 0")
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
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func scopeDescription(cfg config) string {
	if cfg.allOrgs {
		return "all orgs"
	}
	return fmt.Sprintf("org_id=%d", cfg.orgID)
}

func prepareOrphanRefQueue(ctx context.Context, conn *sql.Conn, cfg config) (queueSummary, error) {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS cleanup_assessment_orphan_queue`); err != nil {
		return queueSummary{}, err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE cleanup_assessment_orphan_queue (
  queue_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_table VARCHAR(64) NOT NULL,
  source_string_id VARCHAR(128) NOT NULL,
  source_uint_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  assessment_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  answer_sheet_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  report_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  source_created_at DATETIME(3) NULL,
  processed TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE KEY uk_cleanup_source (source_table, source_string_id),
  KEY idx_cleanup_processed (processed, queue_id),
  KEY idx_cleanup_assessment (assessment_id),
  KEY idx_cleanup_answer_sheet (answer_sheet_id),
  KEY idx_cleanup_report (report_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return queueSummary{}, err
	}

	if err := insertBehaviorFootprintOrphans(ctx, conn, cfg); err != nil {
		return queueSummary{}, err
	}
	if err := insertAssessmentEpisodeOrphans(ctx, conn, cfg); err != nil {
		return queueSummary{}, err
	}
	return loadQueueSummary(ctx, conn)
}

func insertBehaviorFootprintOrphans(ctx context.Context, conn *sql.Conn, cfg config) error {
	query := `
INSERT IGNORE INTO cleanup_assessment_orphan_queue (
  source_table, source_string_id, source_uint_id, org_id, testee_id,
  assessment_id, answer_sheet_id, report_id, source_created_at
)
SELECT
  'behavior_footprint', bf.id, 0, bf.org_id, bf.testee_id,
  bf.assessment_id, bf.answersheet_id, bf.report_id, bf.created_at
FROM behavior_footprint bf
LEFT JOIN assessment a_by_assessment
  ON bf.assessment_id <> 0
 AND a_by_assessment.id = bf.assessment_id
LEFT JOIN assessment a_by_answersheet
  ON bf.answersheet_id <> 0
 AND a_by_answersheet.answer_sheet_id = bf.answersheet_id
LEFT JOIN assessment a_by_report
  ON bf.report_id <> 0
 AND a_by_report.id = bf.report_id
WHERE bf.deleted_at IS NULL
  AND (bf.assessment_id <> 0 OR bf.answersheet_id <> 0 OR bf.report_id <> 0)
  AND a_by_assessment.id IS NULL
  AND a_by_answersheet.id IS NULL
  AND a_by_report.id IS NULL`
	args := make([]any, 0, 4)
	query, args = appendSourceFilters(query, args, "bf", cfg)
	_, err := conn.ExecContext(ctx, query, args...)
	return err
}

func insertAssessmentEpisodeOrphans(ctx context.Context, conn *sql.Conn, cfg config) error {
	query := `
INSERT IGNORE INTO cleanup_assessment_orphan_queue (
  source_table, source_string_id, source_uint_id, org_id, testee_id,
  assessment_id, answer_sheet_id, report_id, source_created_at
)
SELECT
  'assessment_episode', CAST(e.episode_id AS CHAR), e.episode_id, e.org_id, e.testee_id,
  COALESCE(e.assessment_id, 0), e.answersheet_id, COALESCE(e.report_id, 0), e.created_at
FROM assessment_episode e
LEFT JOIN assessment a_by_assessment
  ON e.assessment_id IS NOT NULL
 AND e.assessment_id <> 0
 AND a_by_assessment.id = e.assessment_id
LEFT JOIN assessment a_by_answersheet
  ON e.answersheet_id <> 0
 AND a_by_answersheet.answer_sheet_id = e.answersheet_id
LEFT JOIN assessment a_by_report
  ON e.report_id IS NOT NULL
 AND e.report_id <> 0
 AND a_by_report.id = e.report_id
WHERE e.deleted_at IS NULL
  AND (COALESCE(e.assessment_id, 0) <> 0 OR e.answersheet_id <> 0 OR COALESCE(e.report_id, 0) <> 0)
  AND a_by_assessment.id IS NULL
  AND a_by_answersheet.id IS NULL
  AND a_by_report.id IS NULL`
	args := make([]any, 0, 4)
	query, args = appendSourceFilters(query, args, "e", cfg)
	_, err := conn.ExecContext(ctx, query, args...)
	return err
}

func appendSourceFilters(query string, args []any, alias string, cfg config) (string, []any) {
	if !cfg.allOrgs {
		query += fmt.Sprintf(" AND %s.org_id = ?", alias)
		args = append(args, cfg.orgID)
	}
	if cfg.sourceCreatedStart != "" {
		query += fmt.Sprintf(" AND %s.created_at >= ?", alias)
		args = append(args, cfg.sourceCreatedStart)
	}
	if cfg.sourceCreatedEnd != "" {
		query += fmt.Sprintf(" AND %s.created_at < ?", alias)
		args = append(args, cfg.sourceCreatedEnd)
	}
	return query, args
}

func loadQueueSummary(ctx context.Context, conn *sql.Conn) (queueSummary, error) {
	var s queueSummary
	if err := conn.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(CASE WHEN source_table = 'behavior_footprint' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN source_table = 'assessment_episode' THEN 1 ELSE 0 END), 0),
  COUNT(DISTINCT CASE WHEN answer_sheet_id <> 0 THEN answer_sheet_id END)
FROM cleanup_assessment_orphan_queue`).Scan(&s.TotalRefs, &s.BehaviorFootprintRefs, &s.AssessmentEpisodeRefs, &s.MongoAnswerSheetIDs); err != nil {
		return s, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*) FROM (
  SELECT assessment_id AS report_domain_id
  FROM cleanup_assessment_orphan_queue
  WHERE assessment_id <> 0
  UNION
  SELECT report_id AS report_domain_id
  FROM cleanup_assessment_orphan_queue
  WHERE report_id <> 0
) report_ids`).Scan(&s.MongoReportIDs); err != nil {
		return s, err
	}
	return s, nil
}

func loadQueuedOrphanRefs(ctx context.Context, conn *sql.Conn, cfg config) (rows []orphanRefRow, err error) {
	rs, err := conn.QueryContext(ctx, fmt.Sprintf(`
SELECT
  queue_id, source_table, source_string_id, source_uint_id, org_id, testee_id,
  assessment_id, answer_sheet_id, report_id, source_created_at
FROM cleanup_assessment_orphan_queue
WHERE processed = 0
ORDER BY queue_id
LIMIT %d`, cfg.batchSize))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rs.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	for rs.Next() {
		var row orphanRefRow
		if err := rs.Scan(
			&row.QueueID, &row.SourceTable, &row.SourceStringID, &row.SourceUintID,
			&row.OrgID, &row.TesteeID, &row.AssessmentID, &row.AnswerSheetID, &row.ReportID, &row.SourceCreatedAt,
		); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, rs.Err()
}

func prepareCleanupScope(ctx context.Context, conn *sql.Conn, rows []orphanRefRow) (err error) {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS cleanup_assessment_orphan_scope`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE cleanup_assessment_orphan_scope (
  queue_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  source_table VARCHAR(64) NOT NULL,
  source_string_id VARCHAR(128) NOT NULL,
  source_uint_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  assessment_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  answer_sheet_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  report_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  source_created_at DATETIME(3) NULL,
  KEY idx_cleanup_scope_source (source_table, source_string_id),
  KEY idx_cleanup_scope_source_uint (source_table, source_uint_id),
  KEY idx_cleanup_scope_answer_sheet (answer_sheet_id),
  KEY idx_cleanup_scope_report (report_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	stmt, err := conn.PrepareContext(ctx, `
INSERT INTO cleanup_assessment_orphan_scope (
  queue_id, source_table, source_string_id, source_uint_id, org_id, testee_id,
  assessment_id, answer_sheet_id, report_id, source_created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stmt.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	for _, row := range rows {
		if _, err := stmt.ExecContext(ctx,
			row.QueueID, row.SourceTable, row.SourceStringID, row.SourceUintID, row.OrgID, row.TesteeID,
			row.AssessmentID, row.AnswerSheetID, row.ReportID, nullTimeToAny(row.SourceCreatedAt),
		); err != nil {
			return err
		}
	}
	return nil
}

func processBatch(ctx context.Context, conn *sql.Conn, mongoDB *mongo.Database, rows []orphanRefRow, cfg config) (cleanupCounts, error) {
	if err := prepareCleanupScope(ctx, conn, rows); err != nil {
		return cleanupCounts{}, fmt.Errorf("prepare cleanup scope: %w", err)
	}
	if !cfg.skipBackup {
		if err := backupMySQL(ctx, conn, cfg.backupSuffix); err != nil {
			return cleanupCounts{}, fmt.Errorf("backup mysql: %w", err)
		}
		if err := backupMongo(ctx, mongoDB, rows, cfg.backupSuffix); err != nil {
			return cleanupCounts{}, fmt.Errorf("backup mongo: %w", err)
		}
	}
	mysqlCounts, err := cleanupMySQL(ctx, conn)
	if err != nil {
		return cleanupCounts{}, fmt.Errorf("cleanup mysql: %w", err)
	}
	mongoCounts, err := cleanupMongo(ctx, mongoDB, rows)
	if err != nil {
		return cleanupCounts{}, fmt.Errorf("cleanup mongo: %w", err)
	}
	return addCounts(mysqlCounts, mongoCounts), nil
}

func backupMySQL(ctx context.Context, conn *sql.Conn, suffix string) error {
	statements := []string{
		fmt.Sprintf("CREATE TABLE cleanup_bak_behavior_footprint_%s LIKE behavior_footprint", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO cleanup_bak_behavior_footprint_%s
SELECT bf.* FROM behavior_footprint bf
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.source_table = 'behavior_footprint'
 AND s.source_string_id = bf.id
WHERE bf.deleted_at IS NULL`, suffix),
		fmt.Sprintf("CREATE TABLE cleanup_bak_assessment_episode_%s LIKE assessment_episode", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO cleanup_bak_assessment_episode_%s
SELECT e.* FROM assessment_episode e
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.source_table = 'assessment_episode'
 AND s.source_uint_id = e.episode_id
WHERE e.deleted_at IS NULL`, suffix),
	}
	for i, statement := range statements {
		log.Printf("backup mysql step %d/%d", i+1, len(statements))
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func backupMongo(ctx context.Context, db *mongo.Database, rows []orphanRefRow, suffix string) error {
	answerIDs, err := answerSheetDomainIDs(rows)
	if err != nil {
		return err
	}
	reportIDs, err := reportDomainIDs(rows)
	if err != nil {
		return err
	}
	if err := backupMongoCollection(ctx, db.Collection("answersheets"), db.Collection("cleanup_bak_answersheets_"+suffix), answerIDs); err != nil {
		return err
	}
	return backupMongoCollection(ctx, db.Collection("interpret_reports"), db.Collection("cleanup_bak_interpret_reports_"+suffix), reportIDs)
}

func backupMongoCollection(ctx context.Context, src, dst *mongo.Collection, domainIDs []int64) (err error) {
	if len(domainIDs) == 0 {
		return nil
	}
	cur, err := src.Find(ctx, bson.M{
		"domain_id":  bson.M{"$in": domainIDs},
		"deleted_at": nil,
	})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := cur.Close(ctx); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	docs := make([]interface{}, 0)
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

func cleanupMySQL(ctx context.Context, conn *sql.Conn) (counts cleanupCounts, err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return counts, err
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("rollback mysql cleanup: %v", rollbackErr)
			}
		}
	}()

	result, err := tx.ExecContext(ctx, `
UPDATE behavior_footprint bf
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.source_table = 'behavior_footprint'
 AND s.source_string_id = bf.id
SET bf.deleted_at = NOW(3), bf.updated_at = NOW(3)
WHERE bf.deleted_at IS NULL`)
	if err != nil {
		return counts, err
	}
	counts.BehaviorFootprints, err = result.RowsAffected()
	if err != nil {
		return counts, err
	}

	result, err = tx.ExecContext(ctx, `
UPDATE assessment_episode e
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.source_table = 'assessment_episode'
 AND s.source_uint_id = e.episode_id
SET e.deleted_at = NOW(3), e.updated_at = NOW(3)
WHERE e.deleted_at IS NULL`)
	if err != nil {
		return counts, err
	}
	counts.AssessmentEpisodes, err = result.RowsAffected()
	if err != nil {
		return counts, err
	}

	err = tx.Commit()
	return counts, err
}

func cleanupMongo(ctx context.Context, db *mongo.Database, rows []orphanRefRow) (cleanupCounts, error) {
	now := time.Now()
	answerIDs, err := answerSheetDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}
	reportIDs, err := reportDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}

	counts := cleanupCounts{}
	if len(answerIDs) > 0 {
		answerResult, err := db.Collection("answersheets").UpdateMany(ctx, bson.M{
			"domain_id":  bson.M{"$in": answerIDs},
			"deleted_at": nil,
		}, bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}})
		if err != nil {
			return cleanupCounts{}, err
		}
		counts.MongoAnswersheets = answerResult.ModifiedCount
	}
	if len(reportIDs) > 0 {
		reportResult, err := db.Collection("interpret_reports").UpdateMany(ctx, bson.M{
			"domain_id":  bson.M{"$in": reportIDs},
			"deleted_at": nil,
		}, bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}})
		if err != nil {
			return cleanupCounts{}, err
		}
		counts.MongoReports = reportResult.ModifiedCount
	}
	return counts, nil
}

func countMySQLOrphansInScope(ctx context.Context, conn *sql.Conn) (cleanupCounts, error) {
	var counts cleanupCounts
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM behavior_footprint bf
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.source_table = 'behavior_footprint'
 AND s.source_string_id = bf.id
WHERE bf.deleted_at IS NULL`).Scan(&counts.BehaviorFootprints); err != nil {
		return counts, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM assessment_episode e
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.source_table = 'assessment_episode'
 AND s.source_uint_id = e.episode_id
WHERE e.deleted_at IS NULL`).Scan(&counts.AssessmentEpisodes); err != nil {
		return counts, err
	}
	return counts, nil
}

func countMongoOrphans(ctx context.Context, db *mongo.Database, rows []orphanRefRow) (cleanupCounts, error) {
	answerIDs, err := answerSheetDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}
	reportIDs, err := reportDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}

	counts := cleanupCounts{}
	if len(answerIDs) > 0 {
		counts.MongoAnswersheets, err = db.Collection("answersheets").CountDocuments(ctx, bson.M{
			"domain_id":  bson.M{"$in": answerIDs},
			"deleted_at": nil,
		})
		if err != nil {
			return cleanupCounts{}, err
		}
	}
	if len(reportIDs) > 0 {
		counts.MongoReports, err = db.Collection("interpret_reports").CountDocuments(ctx, bson.M{
			"domain_id":  bson.M{"$in": reportIDs},
			"deleted_at": nil,
		})
		if err != nil {
			return cleanupCounts{}, err
		}
	}
	return counts, nil
}

func markQueueProcessed(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `
UPDATE cleanup_assessment_orphan_queue q
INNER JOIN cleanup_assessment_orphan_scope s
  ON s.queue_id = q.queue_id
SET q.processed = 1
WHERE q.processed = 0`)
	return err
}

func answerSheetDomainIDs(rows []orphanRefRow) ([]int64, error) {
	seen := make(map[uint64]struct{}, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.AnswerSheetID == 0 {
			continue
		}
		if _, ok := seen[row.AnswerSheetID]; ok {
			continue
		}
		seen[row.AnswerSheetID] = struct{}{}
		id, err := uint64ToInt64(row.AnswerSheetID)
		if err != nil {
			return nil, fmt.Errorf("answersheet domain_id %d: %w", row.AnswerSheetID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func reportDomainIDs(rows []orphanRefRow) ([]int64, error) {
	seen := make(map[uint64]struct{}, len(rows)*2)
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		for _, candidate := range []uint64{row.AssessmentID, row.ReportID} {
			if candidate == 0 {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			id, err := uint64ToInt64(candidate)
			if err != nil {
				return nil, fmt.Errorf("report domain_id %d: %w", candidate, err)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func uint64ToInt64(v uint64) (int64, error) {
	const maxInt64 = uint64(9223372036854775807)
	if v > maxInt64 {
		return 0, fmt.Errorf("exceeds int64 max")
	}
	return int64(v), nil
}

func addCounts(a, b cleanupCounts) cleanupCounts {
	return cleanupCounts{
		BehaviorFootprints: a.BehaviorFootprints + b.BehaviorFootprints,
		AssessmentEpisodes: a.AssessmentEpisodes + b.AssessmentEpisodes,
		MongoAnswersheets:  a.MongoAnswersheets + b.MongoAnswersheets,
		MongoReports:       a.MongoReports + b.MongoReports,
	}
}

func backupSuffixForBatch(base string, batchNumber int) string {
	if batchNumber <= 1 {
		return base
	}
	return fmt.Sprintf("%s_%04d", base, batchNumber)
}

func validateBackupSuffix(s string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(s) {
		return fmt.Errorf("must match ^[A-Za-z0-9_]+$")
	}
	return nil
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

func printPreview(rows []orphanRefRow, limit int, counts cleanupCounts) {
	log.Printf("preview batch orphan_refs=%d active_orphans_in_preview: behavior_footprints=%d assessment_episodes=%d mongo_answersheets=%d mongo_reports=%d",
		len(rows), counts.BehaviorFootprints, counts.AssessmentEpisodes, counts.MongoAnswersheets, counts.MongoReports)
	if limit > len(rows) {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		row := rows[i]
		log.Printf("preview source=%s source_id=%s org=%d testee=%d assessment=%d answersheet=%d report=%d created_at=%s",
			row.SourceTable, row.SourceStringID, row.OrgID, row.TesteeID, row.AssessmentID, row.AnswerSheetID, row.ReportID, formatNullTime(row.SourceCreatedAt))
	}
}

func printProgressBar(current, total int) {
	if total <= 0 {
		return
	}
	if current > total {
		current = total
	}
	percentage := float64(current) / float64(total) * 100
	filled := int(percentage / 2)
	empty := 50 - filled

	bar := "["
	for i := 0; i < filled; i++ {
		bar += "="
	}
	for i := 0; i < empty; i++ {
		bar += " "
	}
	bar += "]"

	log.Printf("%s %.1f%% (%d/%d)", bar, percentage, current, total)
}
