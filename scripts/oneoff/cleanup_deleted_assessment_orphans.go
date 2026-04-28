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
	mysqlDSN               string
	mongoURI               string
	mongoDatabase          string
	orgID                  int64
	allOrgs                bool
	assessmentDeletedStart string
	assessmentDeletedEnd   string
	backupSuffix           string
	timeout                time.Duration
	apply                  bool
	skipBackup             bool
	previewLimit           int
	batchSize              int
}

type deletedAssessmentRow struct {
	AssessmentID  uint64
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
	DeletedAt     time.Time
	CreatedAt     time.Time
}

type cleanupCounts struct {
	BehaviorFootprints int64
	AssessmentEpisodes int64
	MongoAnswersheets  int64
	MongoReports       int64
}

// cleanup_deleted_assessment_orphans scans assessments already soft-deleted in MySQL
// and soft-deletes still-active derived rows/documents that point to those assessments.
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

	totalAssessments, err := prepareDeletedAssessmentQueue(ctx, conn, cfg)
	if err != nil {
		log.Fatalf("prepare deleted assessment queue: %v", err)
	}
	log.Printf("scope: %s deleted_start=%q deleted_end=%q candidate_deleted_assessments=%d apply=%v backup=%v",
		scopeDescription(cfg), cfg.assessmentDeletedStart, cfg.assessmentDeletedEnd, totalAssessments, cfg.apply, !cfg.skipBackup)
	if totalAssessments == 0 {
		log.Print("scope is empty; nothing to clean")
		return
	}

	if !cfg.apply {
		rows, err := loadQueuedAssessments(ctx, conn, cfg)
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
		rows, err := loadQueuedAssessments(ctx, conn, cfg)
		if err != nil {
			log.Fatalf("load queued assessments: %v", err)
		}
		if len(rows) == 0 {
			printProgressBar(totalProcessed, totalAssessments)
			log.Printf("cleanup completed: deleted_assessments_scanned=%d behavior_footprints=%d assessment_episodes=%d mongo_answersheets=%d mongo_reports=%d",
				totalProcessed, totalCounts.BehaviorFootprints, totalCounts.AssessmentEpisodes, totalCounts.MongoAnswersheets, totalCounts.MongoReports)
			return
		}

		batchNumber++
		batchCfg := cfg
		batchCfg.backupSuffix = backupSuffixForBatch(cfg.backupSuffix, batchNumber)
		log.Printf("processing batch: number=%d assessments=%d backup_suffix=%s", batchNumber, len(rows), batchCfg.backupSuffix)

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
		printProgressBar(totalProcessed, totalAssessments)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", "", "MongoDB URI")
	flag.StringVar(&cfg.mongoDatabase, "mongo-db", "", "MongoDB database name")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to clean; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "clean all organizations")
	flag.StringVar(&cfg.assessmentDeletedStart, "assessment-deleted-start", "", "optional inclusive assessment.deleted_at lower bound, format 2006-01-02 15:04:05")
	flag.StringVar(&cfg.assessmentDeletedEnd, "assessment-deleted-end", "", "optional exclusive assessment.deleted_at upper bound, format 2006-01-02 15:04:05")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "suffix for backup tables/collections")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall script timeout, e.g. 30m, 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup table/collection creation before applying changes")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of deleted assessments to preview in dry-run")
	flag.IntVar(&cfg.batchSize, "batch-size", 1000, "number of deleted assessments to scan per batch")
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

func prepareDeletedAssessmentQueue(ctx context.Context, conn *sql.Conn, cfg config) (int, error) {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS cleanup_deleted_assessment_queue`); err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE cleanup_deleted_assessment_queue (
  assessment_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  answer_sheet_id BIGINT UNSIGNED NOT NULL,
  assessment_deleted_at DATETIME(3) NOT NULL,
  assessment_created_at DATETIME(3) NOT NULL,
  processed TINYINT(1) NOT NULL DEFAULT 0,
  KEY idx_cleanup_queue_processed (processed, assessment_deleted_at, assessment_id),
  KEY idx_cleanup_queue_org (org_id),
  KEY idx_cleanup_queue_answer_sheet (answer_sheet_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return 0, err
	}

	query := `
INSERT INTO cleanup_deleted_assessment_queue (
  assessment_id, org_id, testee_id, answer_sheet_id, assessment_deleted_at, assessment_created_at
)
SELECT id, org_id, testee_id, answer_sheet_id, deleted_at, created_at
FROM assessment
WHERE deleted_at IS NOT NULL`
	args := make([]any, 0, 4)
	if !cfg.allOrgs {
		query += " AND org_id = ?"
		args = append(args, cfg.orgID)
	}
	if cfg.assessmentDeletedStart != "" {
		query += " AND deleted_at >= ?"
		args = append(args, cfg.assessmentDeletedStart)
	}
	if cfg.assessmentDeletedEnd != "" {
		query += " AND deleted_at < ?"
		args = append(args, cfg.assessmentDeletedEnd)
	}
	query += " ORDER BY deleted_at, id"
	if _, err := conn.ExecContext(ctx, query, args...); err != nil {
		return 0, err
	}

	var count int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM cleanup_deleted_assessment_queue`).Scan(&count)
	return count, err
}

func loadQueuedAssessments(ctx context.Context, conn *sql.Conn, cfg config) (rows []deletedAssessmentRow, err error) {
	rs, err := conn.QueryContext(ctx, fmt.Sprintf(`
SELECT assessment_id, org_id, testee_id, answer_sheet_id, assessment_deleted_at, assessment_created_at
FROM cleanup_deleted_assessment_queue
WHERE processed = 0
ORDER BY assessment_deleted_at, assessment_id
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
		var row deletedAssessmentRow
		if err := rs.Scan(&row.AssessmentID, &row.OrgID, &row.TesteeID, &row.AnswerSheetID, &row.DeletedAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, rs.Err()
}

func prepareCleanupScope(ctx context.Context, conn *sql.Conn, rows []deletedAssessmentRow) (err error) {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS cleanup_deleted_assessment_scope`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE cleanup_deleted_assessment_scope (
  assessment_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  org_id BIGINT NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  answer_sheet_id BIGINT UNSIGNED NOT NULL,
  assessment_deleted_at DATETIME(3) NOT NULL,
  assessment_created_at DATETIME(3) NOT NULL,
  KEY idx_cleanup_scope_org_assessment (org_id, assessment_id),
  KEY idx_cleanup_scope_org_answersheet (org_id, answer_sheet_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	stmt, err := conn.PrepareContext(ctx, `
INSERT INTO cleanup_deleted_assessment_scope (
  assessment_id, org_id, testee_id, answer_sheet_id, assessment_deleted_at, assessment_created_at
) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stmt.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	for _, row := range rows {
		if _, err := stmt.ExecContext(ctx, row.AssessmentID, row.OrgID, row.TesteeID, row.AnswerSheetID, row.DeletedAt, row.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func processBatch(ctx context.Context, conn *sql.Conn, mongoDB *mongo.Database, rows []deletedAssessmentRow, cfg config) (cleanupCounts, error) {
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
		fmt.Sprintf("CREATE TABLE cleanup_bak_deleted_assessment_%s LIKE assessment", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO cleanup_bak_deleted_assessment_%s
SELECT a.* FROM assessment a
INNER JOIN cleanup_deleted_assessment_scope s ON s.assessment_id = a.id`, suffix),
		fmt.Sprintf("CREATE TABLE cleanup_bak_behavior_footprint_%s LIKE behavior_footprint", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO cleanup_bak_behavior_footprint_%s
SELECT bf.* FROM behavior_footprint bf
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.org_id = bf.org_id
 AND (bf.assessment_id = s.assessment_id OR bf.answersheet_id = s.answer_sheet_id OR bf.report_id = s.assessment_id)
WHERE bf.deleted_at IS NULL`, suffix),
		fmt.Sprintf("CREATE TABLE cleanup_bak_assessment_episode_%s LIKE assessment_episode", suffix),
		fmt.Sprintf(`INSERT IGNORE INTO cleanup_bak_assessment_episode_%s
SELECT e.* FROM assessment_episode e
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.org_id = e.org_id
 AND (e.assessment_id = s.assessment_id OR e.answersheet_id = s.answer_sheet_id OR e.report_id = s.assessment_id)
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

func backupMongo(ctx context.Context, db *mongo.Database, rows []deletedAssessmentRow, suffix string) error {
	answerIDs, err := answerSheetDomainIDs(rows)
	if err != nil {
		return err
	}
	assessmentIDs, err := assessmentDomainIDs(rows)
	if err != nil {
		return err
	}
	if err := backupMongoCollection(ctx, db.Collection("answersheets"), db.Collection("cleanup_bak_answersheets_"+suffix), bson.M{
		"domain_id":  bson.M{"$in": answerIDs},
		"deleted_at": nil,
	}); err != nil {
		return err
	}
	return backupMongoCollection(ctx, db.Collection("interpret_reports"), db.Collection("cleanup_bak_interpret_reports_"+suffix), bson.M{
		"domain_id":  bson.M{"$in": assessmentIDs},
		"deleted_at": nil,
	})
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
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.org_id = bf.org_id
 AND (bf.assessment_id = s.assessment_id OR bf.answersheet_id = s.answer_sheet_id OR bf.report_id = s.assessment_id)
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
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.org_id = e.org_id
 AND (e.assessment_id = s.assessment_id OR e.answersheet_id = s.answer_sheet_id OR e.report_id = s.assessment_id)
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

func cleanupMongo(ctx context.Context, db *mongo.Database, rows []deletedAssessmentRow) (cleanupCounts, error) {
	now := time.Now()
	answerIDs, err := answerSheetDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}
	assessmentIDs, err := assessmentDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}

	update := bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}}
	answerResult, err := db.Collection("answersheets").UpdateMany(ctx, bson.M{
		"domain_id":  bson.M{"$in": answerIDs},
		"deleted_at": nil,
	}, update)
	if err != nil {
		return cleanupCounts{}, err
	}
	reportResult, err := db.Collection("interpret_reports").UpdateMany(ctx, bson.M{
		"domain_id":  bson.M{"$in": assessmentIDs},
		"deleted_at": nil,
	}, update)
	if err != nil {
		return cleanupCounts{}, err
	}
	return cleanupCounts{
		MongoAnswersheets: answerResult.ModifiedCount,
		MongoReports:      reportResult.ModifiedCount,
	}, nil
}

func countMySQLOrphansInScope(ctx context.Context, conn *sql.Conn) (cleanupCounts, error) {
	var counts cleanupCounts
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT bf.id)
FROM behavior_footprint bf
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.org_id = bf.org_id
 AND (bf.assessment_id = s.assessment_id OR bf.answersheet_id = s.answer_sheet_id OR bf.report_id = s.assessment_id)
WHERE bf.deleted_at IS NULL`).Scan(&counts.BehaviorFootprints); err != nil {
		return counts, err
	}
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT e.episode_id)
FROM assessment_episode e
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.org_id = e.org_id
 AND (e.assessment_id = s.assessment_id OR e.answersheet_id = s.answer_sheet_id OR e.report_id = s.assessment_id)
WHERE e.deleted_at IS NULL`).Scan(&counts.AssessmentEpisodes); err != nil {
		return counts, err
	}
	return counts, nil
}

func countMongoOrphans(ctx context.Context, db *mongo.Database, rows []deletedAssessmentRow) (cleanupCounts, error) {
	answerIDs, err := answerSheetDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}
	assessmentIDs, err := assessmentDomainIDs(rows)
	if err != nil {
		return cleanupCounts{}, err
	}
	answerCount, err := db.Collection("answersheets").CountDocuments(ctx, bson.M{
		"domain_id":  bson.M{"$in": answerIDs},
		"deleted_at": nil,
	})
	if err != nil {
		return cleanupCounts{}, err
	}
	reportCount, err := db.Collection("interpret_reports").CountDocuments(ctx, bson.M{
		"domain_id":  bson.M{"$in": assessmentIDs},
		"deleted_at": nil,
	})
	if err != nil {
		return cleanupCounts{}, err
	}
	return cleanupCounts{MongoAnswersheets: answerCount, MongoReports: reportCount}, nil
}

func markQueueProcessed(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `
UPDATE cleanup_deleted_assessment_queue q
INNER JOIN cleanup_deleted_assessment_scope s
  ON s.assessment_id = q.assessment_id
SET q.processed = 1
WHERE q.processed = 0`)
	return err
}

func answerSheetDomainIDs(rows []deletedAssessmentRow) ([]int64, error) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		id, err := uint64ToInt64(row.AnswerSheetID)
		if err != nil {
			return nil, fmt.Errorf("answersheet domain_id %d: %w", row.AnswerSheetID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func assessmentDomainIDs(rows []deletedAssessmentRow) ([]int64, error) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		id, err := uint64ToInt64(row.AssessmentID)
		if err != nil {
			return nil, fmt.Errorf("assessment domain_id %d: %w", row.AssessmentID, err)
		}
		ids = append(ids, id)
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

func printPreview(rows []deletedAssessmentRow, limit int, counts cleanupCounts) {
	log.Printf("preview batch deleted_assessments=%d active_orphans_in_preview: behavior_footprints=%d assessment_episodes=%d mongo_answersheets=%d mongo_reports=%d",
		len(rows), counts.BehaviorFootprints, counts.AssessmentEpisodes, counts.MongoAnswersheets, counts.MongoReports)
	if limit > len(rows) {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		row := rows[i]
		log.Printf("preview assessment=%d org=%d testee=%d answersheet=%d assessment_deleted_at=%s",
			row.AssessmentID, row.OrgID, row.TesteeID, row.AnswerSheetID, row.DeletedAt.Format(time.DateTime))
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
