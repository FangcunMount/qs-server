package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"
)

const mongoIDChunkSize = 1000
const mysqlInsertChunkSize = 1000
const defaultMySQLDeleteBatchSize = 1000
const progressBarWidth = 32

var prog progressReporter

type config struct {
	mysqlDSN                  string
	mongoURI                  string
	mongoDB                   string
	testeeIDsRaw              string
	testeeIDsFile             string
	testeeCreatedAfter        string
	allowOldTestees           bool
	deriveIDsFromFacts        bool
	scanEventPayloads         bool
	skipCounts                bool
	skipMongoOutboxEventScope bool
	backupSuffix              string
	timeout                   time.Duration
	apply                     bool
	skipBackup                bool
	previewLimit              int
	noProgress                bool
	mysqlLockWaitTimeout      int
	mysqlDeleteRetries        int
	mysqlDeleteBatchSize      int
	workers                   int
}

type namedCount struct {
	Name  string
	Count int64
}

type namedSQL struct {
	name string
	sql  string
}

type mysqlCountItem struct {
	name  string
	query string
}

type mysqlBackupItem struct {
	table     string
	selectSQL string
}

type mysqlDeleteItem struct {
	name string
	stmt string
}

type mysqlChunkedDeleteSpec struct {
	name             string
	createBatchTable string
	clearBatchTable  string
	fillBatchTable   string
	deleteBatch      string
}

type testeePreview struct {
	ID            uint64
	Name          string
	OrgID         int64
	CreatedAt     sql.NullTime
	AssessmentCnt int64
}

type scopeSummary struct {
	Testees        int64
	Assessments    int64
	AnswerSheets   int64
	Reports        int64
	EventIDs       int64
	OrgIDs         []int64
	MinTouchedDate sql.NullString
	MaxTouchedDate sql.NullString
}

func main() {
	cfg := parseFlags()
	initProgress(cfg.noProgress)
	testeeIDs, err := parseTesteeIDs(cfg.testeeIDsRaw, cfg.testeeIDsFile)
	if err != nil {
		log.Fatalf("parse testee ids: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	prog.Phase("connect databases")
	mysqlDB, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = mysqlDB.Close() }()
	mysqlDB.SetMaxOpenConns(1)
	mysqlDB.SetMaxIdleConns(1)

	conn, err := mysqlDB.Conn(ctx)
	if err != nil {
		log.Fatalf("open mysql conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SET NAMES utf8mb4"); err != nil {
		log.Fatalf("set mysql names: %v", err)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("ping mongo: %v", err)
	}
	mongoDB := mongoClient.Database(cfg.mongoDB)
	if err := verifyMongoReadAccess(ctx, mongoDB); err != nil {
		log.Fatalf("verify mongo read access: %v", err)
	}
	prog.Finish("connect databases", "")

	prog.Phase("prepare mysql scope")
	if err := prepareMySQLScope(ctx, conn, cfg, testeeIDs); err != nil {
		log.Fatalf("prepare mysql scope: %v", err)
	}
	prog.Finish("prepare mysql scope", fmt.Sprintf("testees=%d", len(testeeIDs)))

	ids, err := loadScopeIDs(ctx, conn)
	if err != nil {
		log.Fatalf("load scope ids: %v", err)
	}
	mysqlScopedIDs := ids
	prog.Phase("enrich scope from mongo")
	ids, err = enrichScopeIDsFromMongo(ctx, mongoDB, ids, cfg.workers)
	if err != nil {
		log.Fatalf("enrich scope ids from mongo: %v", err)
	}
	prog.Finish("enrich scope from mongo", fmt.Sprintf("answersheets=%d reports=%d", len(ids.AnswerSheetIDs), len(ids.ReportIDs)))

	prog.Phase("store enriched scope ids")
	if err := storeScopeIDs(ctx, conn, ids); err != nil {
		log.Fatalf("store enriched scope ids: %v", err)
	}
	prog.Finish("store enriched scope ids", "")

	if !scopeIDsEqual(mysqlScopedIDs, ids) {
		log.Print("refresh mysql outbox scope after mongo id enrichment")
		prog.Phase("refresh mysql outbox scope")
		if err := addMySQLOutboxIDsToScope(ctx, conn, cfg); err != nil {
			log.Fatalf("refresh mysql outbox scope after mongo id enrichment: %v", err)
		}
		prog.Finish("refresh mysql outbox scope", "")
	}
	if cfg.skipMongoOutboxEventScope {
		log.Print("skip mongo outbox event id scope by --skip-mongo-outbox-event-scope; mysql pending/checkpoint cleanup will not include event_ids that exist only in Mongo outbox")
	} else {
		prog.Phase("load mongo outbox event ids")
		if err := addMongoOutboxEventIDsToMySQLScope(ctx, conn, mongoDB, ids, cfg.workers); err != nil {
			log.Fatalf("load mongo outbox event ids: %v", err)
		}
	}

	if cfg.skipCounts {
		printScopeIDsSummary(ids, cfg)
		log.Print("row counts skipped by --skip-counts")
	} else {
		prog.Phase("count scoped rows")
		summary, err := loadScopeSummary(ctx, conn)
		if err != nil {
			log.Fatalf("load scope summary: %v", err)
		}
		var mysqlCounts, mongoCounts []namedCount
		countGroup, countCtx := errgroup.WithContext(ctx)
		countGroup.Go(func() error {
			var err error
			mysqlCounts, err = countMySQLRows(countCtx, conn)
			return err
		})
		countGroup.Go(func() error {
			var err error
			mongoCounts, err = countMongoRows(countCtx, mongoDB, ids, cfg.workers)
			return err
		})
		if err := countGroup.Wait(); err != nil {
			log.Fatalf("count scoped rows: %v", err)
		}
		prog.Finish("count scoped rows", "")

		printScopeSummary(summary, cfg)
		printCounts("mysql", mysqlCounts)
		printCounts("mongo", mongoCounts)
	}

	previews, err := loadTesteePreview(ctx, conn, cfg.previewLimit)
	if err != nil {
		log.Fatalf("load preview: %v", err)
	}
	printTesteePreview(previews)

	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to delete scoped perf data")
		return
	}
	if !cfg.skipBackup {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid backup suffix: %v", err)
		}
		prog.Phase("backup mysql and mongo rows")
		backupGroup, backupCtx := errgroup.WithContext(ctx)
		backupGroup.Go(func() error {
			return backupMongoRows(backupCtx, mongoDB, ids, cfg.backupSuffix, cfg.workers)
		})
		backupGroup.Go(func() error {
			return backupMySQLRows(backupCtx, conn, cfg.backupSuffix)
		})
		if err := backupGroup.Wait(); err != nil {
			log.Fatalf("backup rows: %v", err)
		}
		prog.Finish("backup mysql and mongo rows", "suffix="+cfg.backupSuffix)
		log.Printf("backup completed: suffix=%s", cfg.backupSuffix)
	} else {
		log.Print("backup skipped by --skip-backup")
	}

	prog.Phase("delete mysql rows")
	mysqlDeleted, err := deleteMySQLRows(ctx, conn, cfg.mysqlLockWaitTimeout, cfg.mysqlDeleteRetries, cfg.mysqlDeleteBatchSize)
	if err != nil {
		log.Fatalf("delete mysql rows: %v", err)
	}
	prog.Finish("delete mysql rows", "")

	prog.Phase("delete mongo rows")
	mongoDeleted, err := deleteMongoRows(ctx, mongoDB, ids, cfg.workers)
	if err != nil {
		log.Fatalf("delete mongo rows: %v", err)
	}
	prog.Finish("delete mongo rows", "")
	printCounts("mysql_deleted", mysqlDeleted)
	printCounts("mongo_deleted", mongoDeleted)
	log.Print("cleanup completed")
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, for example user:pass@tcp(host:3306)/qs?parseTime=true")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", "", "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", "", "MongoDB database name")
	flag.StringVar(&cfg.testeeIDsRaw, "testee-ids", "", "comma/space/newline separated testee IDs")
	flag.StringVar(&cfg.testeeIDsFile, "testee-ids-file", "", "file containing comma/space/newline separated testee IDs")
	flag.StringVar(&cfg.testeeCreatedAfter, "testee-created-after", "2026-05-01 00:00:00", "safety guard: selected testees must have created_at after this MySQL timestamp")
	flag.BoolVar(&cfg.allowOldTestees, "allow-old-testees", false, "bypass --testee-created-after guard")
	flag.BoolVar(&cfg.deriveIDsFromFacts, "derive-ids-from-facts", false, "also derive IDs from MySQL behavior_footprint and assessment_episode; slower on large fact tables")
	flag.BoolVar(&cfg.scanEventPayloads, "scan-event-payloads", false, "also scan MySQL outbox/pending payload_json for testee_id; expensive on large outbox tables")
	flag.BoolVar(&cfg.skipCounts, "skip-counts", false, "skip expensive row counts and affected source date window; useful when an external backup already protects an apply run")
	flag.BoolVar(&cfg.skipMongoOutboxEventScope, "skip-mongo-outbox-event-scope", false, "skip loading Mongo outbox event_id values into MySQL temp scope; Mongo outbox documents are still deleted by aggregate filters")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "backup table/collection suffix")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall timeout, for example 30m or 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply deletes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip built-in MySQL/Mongo backups before deleting")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of scoped testees to preview")
	flag.BoolVar(&cfg.noProgress, "no-progress", false, "disable terminal progress output")
	flag.IntVar(&cfg.mysqlLockWaitTimeout, "mysql-lock-wait-timeout", 300, "MySQL SESSION innodb_lock_wait_timeout seconds during delete; 0 keeps server default")
	flag.IntVar(&cfg.mysqlDeleteRetries, "mysql-delete-retries", 5, "retries per table when MySQL delete hits lock wait timeout or deadlock")
	flag.IntVar(&cfg.mysqlDeleteBatchSize, "mysql-delete-batch-size", defaultMySQLDeleteBatchSize, "rows per transaction for chunked MySQL deletes")
	flag.IntVar(&cfg.workers, "workers", 4, "parallel workers for independent Mongo scans and cross-store backup/count")
	flag.Parse()

	required := map[string]string{
		"--mysql-dsn": cfg.mysqlDSN,
		"--mongo-uri": cfg.mongoURI,
		"--mongo-db":  cfg.mongoDB,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			log.Fatalf("%s is required", name)
		}
	}
	if strings.TrimSpace(cfg.testeeIDsRaw) == "" && strings.TrimSpace(cfg.testeeIDsFile) == "" {
		log.Fatal("either --testee-ids or --testee-ids-file is required")
	}
	if cfg.previewLimit < 0 {
		log.Fatal("--preview-limit must be >= 0")
	}
	if cfg.mysqlLockWaitTimeout < 0 {
		log.Fatal("--mysql-lock-wait-timeout must be >= 0")
	}
	if cfg.mysqlDeleteRetries < 1 {
		log.Fatal("--mysql-delete-retries must be >= 1")
	}
	if cfg.workers < 1 {
		log.Fatal("--workers must be >= 1")
	}
	if !cfg.apply && cfg.skipBackup {
		log.Print("--skip-backup has no effect in dry-run mode")
	}
	return cfg
}

func parseTesteeIDs(raw, file string) ([]uint64, error) {
	parts := []string{}
	if strings.TrimSpace(raw) != "" {
		parts = append(parts, splitIDs(raw)...)
	}
	if strings.TrimSpace(file) != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		parts = append(parts, splitIDs(string(data))...)
	}
	if len(parts) == 0 {
		return nil, errors.New("empty testee id list")
	}

	seen := make(map[uint64]struct{}, len(parts))
	ids := make([]uint64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid testee id %q: %w", part, err)
		}
		if id == 0 {
			return nil, fmt.Errorf("invalid testee id %q: zero is not allowed", part)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func splitIDs(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func prepareMySQLScope(ctx context.Context, conn *sql.Conn, cfg config, testeeIDs []uint64) error {
	eventIDCollation, err := loadEventIDCollation(ctx, conn)
	if err != nil {
		return err
	}
	eventIDType := "VARCHAR(128)"
	if eventIDCollation != "" {
		eventIDType = fmt.Sprintf("VARCHAR(128) CHARACTER SET utf8mb4 COLLATE %s", eventIDCollation)
		if !prog.enabled {
			log.Printf("prepare mysql scope: event_id collation=%s", eventIDCollation)
		}
	}

	stmts := []string{
		`CREATE TEMPORARY TABLE tmp_cleanup_testee_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
		`CREATE TEMPORARY TABLE tmp_cleanup_assessment_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
		`CREATE TEMPORARY TABLE tmp_cleanup_answersheet_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
		`CREATE TEMPORARY TABLE tmp_cleanup_report_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
		fmt.Sprintf(`CREATE TEMPORARY TABLE tmp_cleanup_event_ids (event_id %s NOT NULL PRIMARY KEY)`, eventIDType),
		fmt.Sprintf(`CREATE TEMPORARY TABLE tmp_cleanup_mysql_outbox_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY, event_id %s NOT NULL, UNIQUE KEY uk_event_id (event_id))`, eventIDType),
		fmt.Sprintf(`CREATE TEMPORARY TABLE tmp_cleanup_pending_event_ids (event_id %s NOT NULL PRIMARY KEY)`, eventIDType),
	}
	for i, stmt := range stmts {
		if err := prog.RunStep("create temp tables", i+1, len(stmts), func() error {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if err := bulkInsertUint64IDs(ctx, conn, "tmp_cleanup_testee_ids", testeeIDs); err != nil {
		return fmt.Errorf("insert testee ids: %w", err)
	}

	if err := validateTesteeGuard(ctx, conn, cfg, len(testeeIDs)); err != nil {
		return err
	}

	populate := []namedSQL{
		{"assessment ids from assessment.testee_id", `INSERT IGNORE INTO tmp_cleanup_assessment_ids (id)
SELECT a.id FROM assessment a JOIN tmp_cleanup_testee_ids t ON t.id = a.testee_id`,
		},
	}
	if cfg.deriveIDsFromFacts {
		populate = append(populate,
			namedSQL{"assessment ids from behavior_footprint", `INSERT IGNORE INTO tmp_cleanup_assessment_ids (id)
SELECT DISTINCT bf.assessment_id FROM behavior_footprint bf JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id WHERE bf.assessment_id <> 0`},
			namedSQL{"assessment ids from assessment_episode", `INSERT IGNORE INTO tmp_cleanup_assessment_ids (id)
SELECT DISTINCT ae.assessment_id FROM assessment_episode ae JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id WHERE ae.assessment_id IS NOT NULL AND ae.assessment_id <> 0`},
		)
	}
	populate = append(populate,
		namedSQL{"answersheet ids from assessment scope", `INSERT IGNORE INTO tmp_cleanup_answersheet_ids (id)
SELECT DISTINCT a.answer_sheet_id FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id WHERE a.answer_sheet_id <> 0`},
	)
	if cfg.deriveIDsFromFacts {
		populate = append(populate,
			namedSQL{"answersheet ids from behavior_footprint", `INSERT IGNORE INTO tmp_cleanup_answersheet_ids (id)
SELECT DISTINCT bf.answersheet_id FROM behavior_footprint bf JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id WHERE bf.answersheet_id <> 0`},
			namedSQL{"answersheet ids from assessment_episode", `INSERT IGNORE INTO tmp_cleanup_answersheet_ids (id)
SELECT DISTINCT ae.answersheet_id FROM assessment_episode ae JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id WHERE ae.answersheet_id <> 0`},
		)
	}
	populate = append(populate,
		namedSQL{"report ids from assessment scope", `INSERT IGNORE INTO tmp_cleanup_report_ids (id)
SELECT id FROM tmp_cleanup_assessment_ids`,
		},
	)
	if cfg.deriveIDsFromFacts {
		populate = append(populate,
			namedSQL{"report ids from behavior_footprint", `INSERT IGNORE INTO tmp_cleanup_report_ids (id)
SELECT DISTINCT bf.report_id FROM behavior_footprint bf JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id WHERE bf.report_id <> 0`},
			namedSQL{"report ids from assessment_episode", `INSERT IGNORE INTO tmp_cleanup_report_ids (id)
SELECT DISTINCT ae.report_id FROM assessment_episode ae JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id WHERE ae.report_id IS NOT NULL AND ae.report_id <> 0`},
		)
	}
	for i, item := range populate {
		item := item
		if err := prog.RunStep(item.name, i+1, len(populate), func() error {
			if _, err := conn.ExecContext(ctx, item.sql); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if err := addMySQLOutboxIDsToScope(ctx, conn, cfg); err != nil {
		return err
	}
	return nil
}

func addMySQLOutboxIDsToScope(ctx context.Context, conn *sql.Conn, cfg config) error {
	statements := mysqlOutboxScopeStatements(cfg)
	for i, item := range statements {
		item := item
		if err := prog.RunStep(item.name, i+1, len(statements), func() error {
			if _, err := conn.ExecContext(ctx, item.sql); err != nil {
				return fmt.Errorf("%s: %w", item.name, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func mysqlOutboxScopeStatements(cfg config) []namedSQL {
	outboxStmts := []namedSQL{
		{"mysql outbox ids from assessment aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_assessment_ids a ON BINARY o.aggregate_id = BINARY CAST(a.id AS CHAR) WHERE o.aggregate_type = 'Assessment'`,
		},
		{"mysql outbox ids from report aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_report_ids r ON BINARY o.aggregate_id = BINARY CAST(r.id AS CHAR) WHERE o.aggregate_type = 'Report'`,
		},
		{"mysql outbox ids from answersheet aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_answersheet_ids s ON BINARY o.aggregate_id = BINARY CAST(s.id AS CHAR) WHERE o.aggregate_type = 'AnswerSheet'`,
		},
		{"mysql outbox ids from behavior testee aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_testee_ids t ON BINARY o.aggregate_id = BINARY CAST(t.id AS CHAR) WHERE o.aggregate_type = 'BehaviorFootprint'`,
		},
		{"mysql outbox ids from behavior answersheet aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_answersheet_ids s ON BINARY o.aggregate_id = BINARY CAST(s.id AS CHAR) WHERE o.aggregate_type = 'BehaviorFootprint'`,
		},
		{"mysql outbox ids from behavior assessment aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_assessment_ids a ON BINARY o.aggregate_id = BINARY CAST(a.id AS CHAR) WHERE o.aggregate_type = 'BehaviorFootprint'`,
		},
		{"mysql outbox ids from behavior report aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_report_ids r ON BINARY o.aggregate_id = BINARY CAST(r.id AS CHAR) WHERE o.aggregate_type = 'BehaviorFootprint'`,
		},
	}
	if cfg.scanEventPayloads {
		outboxStmts = append(outboxStmts,
			namedSQL{"mysql outbox ids from payload_json", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id
FROM domain_event_outbox o
JOIN tmp_cleanup_testee_ids t
  ON o.payload_json REGEXP CONCAT('"testee_id"[[:space:]]*:[[:space:]]*"?', t.id, '"?([,}[:space:]]|$)')`})
	}
	outboxStmts = append(outboxStmts,
		namedSQL{"event ids from mysql outbox", `INSERT IGNORE INTO tmp_cleanup_event_ids (event_id)
SELECT event_id FROM tmp_cleanup_mysql_outbox_ids`,
		},
		namedSQL{"analytics pending ids from event ids", `INSERT IGNORE INTO tmp_cleanup_pending_event_ids (event_id)
SELECT p.event_id FROM analytics_pending_event p JOIN tmp_cleanup_event_ids e ON BINARY e.event_id = BINARY p.event_id`,
		},
	)
	if cfg.scanEventPayloads {
		outboxStmts = append(outboxStmts,
			namedSQL{"analytics pending ids from payload_json", `INSERT IGNORE INTO tmp_cleanup_pending_event_ids (event_id)
SELECT p.event_id
FROM analytics_pending_event p
JOIN tmp_cleanup_testee_ids t
  ON p.payload_json REGEXP CONCAT('"testee_id"[[:space:]]*:[[:space:]]*"?', t.id, '"?([,}[:space:]]|$)')`})
	}
	return outboxStmts
}

func loadEventIDCollation(ctx context.Context, conn *sql.Conn) (string, error) {
	var collation sql.NullString
	if err := conn.QueryRowContext(ctx, `
SELECT COALESCE(
  MAX(CASE WHEN table_name = 'analytics_pending_event' THEN collation_name END),
  MAX(CASE WHEN table_name = 'analytics_projector_checkpoint' THEN collation_name END),
  MAX(CASE WHEN table_name = 'domain_event_outbox' THEN collation_name END)
)
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name IN ('analytics_pending_event', 'analytics_projector_checkpoint', 'domain_event_outbox')
  AND column_name = 'event_id'`).Scan(&collation); err != nil {
		return "", fmt.Errorf("load event_id collation: %w", err)
	}
	if !collation.Valid || strings.TrimSpace(collation.String) == "" {
		return "", nil
	}
	name := strings.TrimSpace(collation.String)
	ok, err := regexp.MatchString(`^[A-Za-z0-9_]+$`, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unsafe event_id collation name %q", name)
	}
	return name, nil
}

func validateTesteeGuard(ctx context.Context, conn *sql.Conn, cfg config, expected int) error {
	var existing int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`).Scan(&existing); err != nil {
		return err
	}
	if existing != expected {
		rows, err := conn.QueryContext(ctx, `SELECT x.id FROM tmp_cleanup_testee_ids x LEFT JOIN testee t ON t.id = x.id WHERE t.id IS NULL ORDER BY x.id LIMIT 20`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		missing := []string{}
		for rows.Next() {
			var id uint64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			missing = append(missing, strconv.FormatUint(id, 10))
		}
		return fmt.Errorf("some testee IDs do not exist in MySQL testee table; missing sample=%s", strings.Join(missing, ","))
	}
	if cfg.allowOldTestees {
		return nil
	}
	var oldCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id WHERE t.created_at <= ?`, cfg.testeeCreatedAfter).Scan(&oldCount); err != nil {
		return err
	}
	if oldCount == 0 {
		return nil
	}
	rows, err := conn.QueryContext(ctx, `
SELECT t.id, t.name, t.created_at
FROM testee t
JOIN tmp_cleanup_testee_ids x ON x.id = t.id
WHERE t.created_at <= ?
ORDER BY t.created_at ASC
LIMIT 20`, cfg.testeeCreatedAfter)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	samples := []string{}
	for rows.Next() {
		var id uint64
		var name string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &createdAt); err != nil {
			return err
		}
		samples = append(samples, fmt.Sprintf("%d/%s/%s", id, name, createdAt.Format("2006-01-02 15:04:05")))
	}
	return fmt.Errorf("%d testee(s) violate --testee-created-after=%q; sample=%s; use --allow-old-testees only after manual verification", oldCount, cfg.testeeCreatedAfter, strings.Join(samples, ", "))
}

func verifyMongoReadAccess(ctx context.Context, db *mongo.Database) error {
	collections := []string{
		"answersheets",
		"answersheet_submit_idempotency",
		"interpret_reports",
		"domain_event_outbox",
	}
	for _, name := range collections {
		err := db.Collection(name).FindOne(
			ctx,
			bson.M{"_id": bson.M{"$exists": false}},
			options.FindOne().SetProjection(bson.M{"_id": 1}),
		).Err()
		if err == nil || errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if isMongoUnauthorized(err) {
			return fmt.Errorf("%s find permission denied: %w; use an authenticated --mongo-uri, for example mongodb://user:password@127.0.0.1:27017/%s?directConnection=true, and add authSource=admin if the user was created in admin", name, err, db.Name())
		}
		return fmt.Errorf("%s find probe: %w", name, err)
	}
	return nil
}

func isMongoUnauthorized(err error) bool {
	var commandErr mongo.CommandError
	if errors.As(err, &commandErr) {
		if commandErr.Code == 13 || strings.EqualFold(commandErr.Name, "Unauthorized") {
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, "Unauthorized") || strings.Contains(msg, "requires authentication")
}

type scopeIDs struct {
	TesteeIDs      []uint64
	AssessmentIDs  []uint64
	AnswerSheetIDs []uint64
	ReportIDs      []uint64
}

func loadScopeIDs(ctx context.Context, conn *sql.Conn) (scopeIDs, error) {
	load := func(query string) ([]uint64, error) {
		rows, err := conn.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		var out []uint64
		for rows.Next() {
			var id uint64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	testeeIDs, err := load(`SELECT id FROM tmp_cleanup_testee_ids ORDER BY id`)
	if err != nil {
		return scopeIDs{}, err
	}
	assessmentIDs, err := load(`SELECT id FROM tmp_cleanup_assessment_ids ORDER BY id`)
	if err != nil {
		return scopeIDs{}, err
	}
	answerSheetIDs, err := load(`SELECT id FROM tmp_cleanup_answersheet_ids ORDER BY id`)
	if err != nil {
		return scopeIDs{}, err
	}
	reportIDs, err := load(`SELECT id FROM tmp_cleanup_report_ids ORDER BY id`)
	if err != nil {
		return scopeIDs{}, err
	}
	return scopeIDs{TesteeIDs: testeeIDs, AssessmentIDs: assessmentIDs, AnswerSheetIDs: answerSheetIDs, ReportIDs: reportIDs}, nil
}

func enrichScopeIDsFromMongo(ctx context.Context, db *mongo.Database, ids scopeIDs, workers int) (scopeIDs, error) {
	type mongoFieldTask struct {
		coll  string
		field string
		label string
	}
	tasks := []mongoFieldTask{
		{coll: "answersheets", field: "domain_id", label: "answersheets.domain_id"},
		{coll: "answersheet_submit_idempotency", field: "answersheet_id", label: "answersheet_submit_idempotency.answersheet_id"},
		{coll: "interpret_reports", field: "domain_id", label: "interpret_reports.domain_id"},
	}
	results := make([][]uint64, len(tasks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for i, task := range tasks {
		i, task := i, task
		group.Go(func() error {
			filter := inUint64("testee_id", ids.TesteeIDs)
			out, err := loadMongoUint64Field(groupCtx, db.Collection(task.coll), filter, task.field, task.label)
			if err != nil {
				return fmt.Errorf("load %s: %w", task.label, err)
			}
			results[i] = out
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return ids, err
	}

	ids.AnswerSheetIDs = uniqueUint64(append(append(ids.AnswerSheetIDs, results[0]...), results[1]...))
	ids.ReportIDs = uniqueUint64(append(ids.ReportIDs, results[2]...))
	return ids, nil
}

func loadMongoUint64Field(ctx context.Context, coll *mongo.Collection, filter bson.M, field, label string) ([]uint64, error) {
	if len(filter) == 0 {
		return nil, nil
	}
	prog.Indeterminate(label)
	cur, err := coll.Find(ctx, filter, options.Find().SetProjection(bson.M{field: 1}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var out []uint64
	for cur.Next(ctx) {
		var row bson.M
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		value, ok := row[field]
		if !ok {
			continue
		}
		id, ok := bsonValueToUint64(value)
		if !ok || id == 0 {
			continue
		}
		out = append(out, id)
		if len(out)%10000 == 0 {
			prog.Step(label, int64(len(out)), 0)
		}
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	out = uniqueUint64(out)
	prog.SubtaskDone(label, fmt.Sprintf("ids=%d", len(out)))
	return out, nil
}

func bsonValueToUint64(value any) (uint64, bool) {
	switch v := value.(type) {
	case int32:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case uint64:
		return v, true
	case string:
		id, err := strconv.ParseUint(v, 10, 64)
		return id, err == nil
	default:
		return 0, false
	}
}

func storeScopeIDs(ctx context.Context, conn *sql.Conn, ids scopeIDs) error {
	tables := []struct {
		name string
		ids  []uint64
	}{
		{"tmp_cleanup_assessment_ids", ids.AssessmentIDs},
		{"tmp_cleanup_answersheet_ids", ids.AnswerSheetIDs},
		{"tmp_cleanup_report_ids", ids.ReportIDs},
	}
	for i, table := range tables {
		table := table
		if err := prog.RunStep("store "+table.name, i+1, len(tables), func() error {
			if err := bulkInsertUint64IDs(ctx, conn, table.name, table.ids); err != nil {
				return fmt.Errorf("store %s: %w", table.name, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func bulkInsertUint64IDs(ctx context.Context, conn *sql.Conn, table string, ids []uint64) error {
	ids = uniqueUint64(ids)
	chunkCount := (len(ids) + mysqlInsertChunkSize - 1) / mysqlInsertChunkSize
	if chunkCount == 0 {
		return nil
	}
	chunkIndex := 0
	for start := 0; start < len(ids); start += mysqlInsertChunkSize {
		end := start + mysqlInsertChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		chunkIndex++
		prog.Step(table, int64(chunkIndex), int64(chunkCount))
		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk))
		for _, id := range chunk {
			placeholders = append(placeholders, "(?)")
			args = append(args, id)
		}
		stmt := fmt.Sprintf("INSERT IGNORE INTO %s (id) VALUES %s", table, strings.Join(placeholders, ","))
		if _, err := conn.ExecContext(ctx, stmt, args...); err != nil {
			return err
		}
	}
	return nil
}

func bulkInsertStringColumn(ctx context.Context, conn *sql.Conn, table, column string, values []string) error {
	values = uniqueStrings(values)
	chunkCount := (len(values) + mysqlInsertChunkSize - 1) / mysqlInsertChunkSize
	if chunkCount == 0 {
		return nil
	}
	chunkIndex := 0
	for start := 0; start < len(values); start += mysqlInsertChunkSize {
		end := start + mysqlInsertChunkSize
		if end > len(values) {
			end = len(values)
		}
		chunk := values[start:end]
		chunkIndex++
		prog.Step(table+"."+column, int64(chunkIndex), int64(chunkCount))
		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk))
		for _, value := range chunk {
			placeholders = append(placeholders, "(?)")
			args = append(args, value)
		}
		stmt := fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES %s", table, column, strings.Join(placeholders, ","))
		if _, err := conn.ExecContext(ctx, stmt, args...); err != nil {
			return err
		}
	}
	return nil
}

func addMongoOutboxEventIDsToMySQLScope(ctx context.Context, conn *sql.Conn, db *mongo.Database, ids scopeIDs, workers int) error {
	filters := mongoOutboxFilters(ids)
	eventIDs, err := scanMongoOutboxEventIDs(ctx, db, filters, workers)
	if err != nil {
		return err
	}
	if err := bulkInsertStringColumn(ctx, conn, "tmp_cleanup_event_ids", "event_id", eventIDs); err != nil {
		return fmt.Errorf("store mongo outbox event ids: %w", err)
	}
	prog.Indeterminate("sync analytics pending event ids")
	_, err = conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_pending_event_ids (event_id)
SELECT p.event_id FROM analytics_pending_event p JOIN tmp_cleanup_event_ids e ON BINARY e.event_id = BINARY p.event_id`)
	if err != nil {
		return err
	}
	prog.Finish("load mongo outbox event ids", fmt.Sprintf("event_ids=%d", len(eventIDs)))
	return nil
}

func scanMongoOutboxEventIDs(ctx context.Context, db *mongo.Database, filters []bson.M, workers int) ([]string, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for _, filter := range filters {
		filter := filter
		group.Go(func() error {
			cur, err := db.Collection("domain_event_outbox").Find(groupCtx, filter, options.Find().SetProjection(bson.M{"event_id": 1}))
			if err != nil {
				return err
			}
			defer func() { _ = cur.Close(groupCtx) }()
			local := make([]string, 0, 128)
			for cur.Next(groupCtx) {
				var row struct {
					EventID string `bson:"event_id"`
				}
				if err := cur.Decode(&row); err != nil {
					return err
				}
				if row.EventID == "" {
					continue
				}
				local = append(local, row.EventID)
			}
			if err := cur.Err(); err != nil {
				return err
			}
			if len(local) == 0 {
				return nil
			}
			mu.Lock()
			for _, eventID := range local {
				seen[eventID] = struct{}{}
			}
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for eventID := range seen {
		out = append(out, eventID)
	}
	sort.Strings(out)
	return out, nil
}

func loadScopeSummary(ctx context.Context, conn *sql.Conn) (scopeSummary, error) {
	s := scopeSummary{}
	queries := []struct {
		target *int64
		query  string
	}{
		{&s.Testees, `SELECT COUNT(*) FROM tmp_cleanup_testee_ids`},
		{&s.Assessments, `SELECT COUNT(*) FROM tmp_cleanup_assessment_ids`},
		{&s.AnswerSheets, `SELECT COUNT(*) FROM tmp_cleanup_answersheet_ids`},
		{&s.Reports, `SELECT COUNT(*) FROM tmp_cleanup_report_ids`},
		{&s.EventIDs, `SELECT COUNT(*) FROM tmp_cleanup_event_ids`},
	}
	for _, item := range queries {
		if err := conn.QueryRowContext(ctx, item.query).Scan(item.target); err != nil {
			return s, err
		}
	}
	rows, err := conn.QueryContext(ctx, `SELECT DISTINCT org_id FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id ORDER BY org_id`)
	if err != nil {
		return s, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var orgID int64
		if err := rows.Scan(&orgID); err != nil {
			return s, err
		}
		s.OrgIDs = append(s.OrgIDs, orgID)
	}
	if err := rows.Err(); err != nil {
		return s, err
	}

	err = conn.QueryRowContext(ctx, `
SELECT CAST(MIN(d) AS CHAR), CAST(MAX(d) AS CHAR)
FROM (
  SELECT DATE(a.created_at) AS d FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id
  UNION ALL
  SELECT DATE(bf.occurred_at) AS d FROM behavior_footprint bf JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id
  UNION ALL
  SELECT DATE(ae.submitted_at) AS d FROM assessment_episode ae JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id
  UNION ALL
  SELECT DATE(l.intake_at) AS d FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id
) touched`).Scan(&s.MinTouchedDate, &s.MaxTouchedDate)
	return s, err
}

func loadTesteePreview(ctx context.Context, conn *sql.Conn, limit int) ([]testeePreview, error) {
	if limit == 0 {
		return nil, nil
	}
	rows, err := conn.QueryContext(ctx, `
SELECT t.id, t.name, t.org_id, t.created_at, COUNT(a.id) AS assessment_cnt
FROM testee t
JOIN tmp_cleanup_testee_ids x ON x.id = t.id
LEFT JOIN assessment a ON a.testee_id = t.id
GROUP BY t.id, t.name, t.org_id, t.created_at
ORDER BY assessment_cnt DESC, t.id
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []testeePreview
	for rows.Next() {
		var row testeePreview
		if err := rows.Scan(&row.ID, &row.Name, &row.OrgID, &row.CreatedAt, &row.AssessmentCnt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func countMySQLRows(ctx context.Context, conn *sql.Conn) ([]namedCount, error) {
	items := []mysqlCountItem{
		{"testee", `SELECT COUNT(*) FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
		{"assessment", `SELECT COUNT(*) FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id`},
		{"assessment_score", `SELECT COUNT(*) FROM assessment_score s LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LEFT JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id WHERE a.id IS NOT NULL OR t.id IS NOT NULL`},
		{"assessment_task", `SELECT COUNT(*) FROM assessment_task task LEFT JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id WHERE t.id IS NOT NULL OR a.id IS NOT NULL`},
		{"clinician_relation", `SELECT COUNT(*) FROM clinician_relation r JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id`},
		{"assessment_entry_intake_log", `SELECT COUNT(*) FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id`},
		{"behavior_footprint", `SELECT COUNT(*) FROM behavior_footprint bf LEFT JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"assessment_episode", `SELECT COUNT(*) FROM assessment_episode ae LEFT JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"domain_event_outbox", `SELECT COUNT(*) FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id`},
		{"analytics_pending_event", `SELECT COUNT(*) FROM analytics_pending_event p JOIN tmp_cleanup_pending_event_ids x ON BINARY x.event_id = BINARY p.event_id`},
		{"analytics_projector_checkpoint", `SELECT COUNT(*) FROM analytics_projector_checkpoint c JOIN tmp_cleanup_event_ids x ON BINARY x.event_id = BINARY c.event_id`},
	}
	legacyItems, err := legacyStatisticsCountItems(ctx, conn)
	if err != nil {
		return nil, err
	}
	items = append(items, legacyItems...)
	out := make([]namedCount, 0, len(items))
	for i, item := range items {
		item := item
		var count int64
		if err := prog.RunStep("count mysql "+item.name, i+1, len(items), func() error {
			return conn.QueryRowContext(ctx, item.query).Scan(&count)
		}); err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		out = append(out, namedCount{Name: item.name, Count: count})
	}
	return out, nil
}

func countMongoRows(ctx context.Context, db *mongo.Database, ids scopeIDs, workers int) ([]namedCount, error) {
	items := mongoCollectionScopes(ids)
	out := make([]namedCount, len(items))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for i, item := range items {
		i, item := i, item
		group.Go(func() error {
			count, err := countMongoDocumentsByFilters(groupCtx, db.Collection(item.coll), item.filters, item.name)
			if err != nil {
				return fmt.Errorf("%s: %w", item.name, err)
			}
			out[i] = namedCount{Name: item.name, Count: count}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

func backupMySQLRows(ctx context.Context, conn *sql.Conn, suffix string) error {
	items := []mysqlBackupItem{
		{"testee", `SELECT t.* FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
		{"assessment", `SELECT a.* FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id`},
		{"assessment_score", `SELECT s.* FROM assessment_score s LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LEFT JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id WHERE a.id IS NOT NULL OR t.id IS NOT NULL`},
		{"assessment_task", `SELECT task.* FROM assessment_task task LEFT JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id WHERE t.id IS NOT NULL OR a.id IS NOT NULL`},
		{"clinician_relation", `SELECT r.* FROM clinician_relation r JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id`},
		{"assessment_entry_intake_log", `SELECT l.* FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id`},
		{"behavior_footprint", `SELECT bf.* FROM behavior_footprint bf LEFT JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"assessment_episode", `SELECT ae.* FROM assessment_episode ae LEFT JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"domain_event_outbox", `SELECT o.* FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id`},
		{"analytics_pending_event", `SELECT p.* FROM analytics_pending_event p JOIN tmp_cleanup_pending_event_ids x ON BINARY x.event_id = BINARY p.event_id`},
		{"analytics_projector_checkpoint", `SELECT c.* FROM analytics_projector_checkpoint c JOIN tmp_cleanup_event_ids x ON BINARY x.event_id = BINARY c.event_id`},
	}
	legacyItems, err := legacyStatisticsBackupItems(ctx, conn)
	if err != nil {
		return err
	}
	items = append(items, legacyItems...)
	for i, item := range items {
		item := item
		if err := prog.RunStep("backup mysql "+item.table, i+1, len(items), func() error {
			backupTable := fmt.Sprintf("cleanup_bak_perf_testee_%s_%s", item.table, suffix)
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE `%s`", backupTable, item.table)); err != nil {
				return fmt.Errorf("create backup table %s: %w", backupTable, err)
			}
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("INSERT IGNORE INTO `%s` %s", backupTable, item.selectSQL)); err != nil {
				return fmt.Errorf("insert backup table %s: %w", backupTable, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func backupMongoRows(ctx context.Context, db *mongo.Database, ids scopeIDs, suffix string, workers int) error {
	items := mongoCollectionScopes(ids)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for _, item := range items {
		item := item
		group.Go(func() error {
			backupName := "cleanup_bak_perf_testee_" + item.coll + "_" + suffix
			count, err := backupMongoCollection(groupCtx, db.Collection(item.coll), db.Collection(backupName), item.filters, item.coll)
			if err != nil {
				return fmt.Errorf("backup mongo %s: %w", item.coll, err)
			}
			log.Printf("mongo backup: source=%s backup=%s docs=%d", item.coll, backupName, count)
			return nil
		})
	}
	return group.Wait()
}

func backupMongoCollection(ctx context.Context, source, backup *mongo.Collection, filters []bson.M, label string) (int64, error) {
	batch := make([]interface{}, 0, 1000)
	var total int64
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		result, err := backup.InsertMany(ctx, batch, options.InsertMany().SetOrdered(false))
		if err != nil && !mongo.IsDuplicateKeyError(err) {
			return err
		}
		if result != nil {
			total += int64(len(result.InsertedIDs))
		}
		batch = batch[:0]
		return nil
	}
	seen := map[string]struct{}{}
	for i, filter := range filters {
		filter := filter
		if err := prog.RunStep(label+" filters", i+1, len(filters), func() error {
			cur, err := source.Find(ctx, filter)
			if err != nil {
				return err
			}
			for cur.Next(ctx) {
				var doc bson.M
				if err := cur.Decode(&doc); err != nil {
					_ = cur.Close(ctx)
					return err
				}
				key := mongoDocumentIDKey(doc["_id"])
				if key != "" {
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
				}
				batch = append(batch, doc)
				if len(batch) >= cap(batch) {
					if err := flush(); err != nil {
						_ = cur.Close(ctx)
						return err
					}
				}
				if len(seen)%10000 == 0 && len(seen) > 0 {
					prog.Step(label, int64(len(seen)), 0)
				}
			}
			if err := cur.Err(); err != nil {
				_ = cur.Close(ctx)
				return err
			}
			return cur.Close(ctx)
		}); err != nil {
			return total, err
		}
	}
	return total, flush()
}

func legacyStatisticsCountItems(ctx context.Context, conn *sql.Conn) ([]mysqlCountItem, error) {
	items := []mysqlCountItem{}
	if exists, err := mysqlTableExists(ctx, conn, "statistics_daily"); err != nil {
		return nil, err
	} else if exists {
		items = append(items, mysqlCountItem{"statistics_daily_testee", `SELECT COUNT(*) FROM statistics_daily d JOIN tmp_cleanup_testee_ids t ON BINARY d.statistic_key = BINARY CAST(t.id AS CHAR) WHERE d.statistic_type = 'testee'`})
	} else {
		log.Print("optional mysql table statistics_daily does not exist; skip legacy statistics_daily scope")
	}
	if exists, err := mysqlTableExists(ctx, conn, "statistics_accumulated"); err != nil {
		return nil, err
	} else if exists {
		items = append(items, mysqlCountItem{"statistics_accumulated_testee", `SELECT COUNT(*) FROM statistics_accumulated a JOIN tmp_cleanup_testee_ids t ON BINARY a.statistic_key = BINARY CAST(t.id AS CHAR) WHERE a.statistic_type = 'testee'`})
	} else {
		log.Print("optional mysql table statistics_accumulated does not exist; skip legacy statistics_accumulated scope")
	}
	return items, nil
}

func legacyStatisticsBackupItems(ctx context.Context, conn *sql.Conn) ([]mysqlBackupItem, error) {
	items := []mysqlBackupItem{}
	if exists, err := mysqlTableExists(ctx, conn, "statistics_daily"); err != nil {
		return nil, err
	} else if exists {
		items = append(items, mysqlBackupItem{"statistics_daily", `SELECT d.* FROM statistics_daily d JOIN tmp_cleanup_testee_ids t ON BINARY d.statistic_key = BINARY CAST(t.id AS CHAR) WHERE d.statistic_type = 'testee'`})
	}
	if exists, err := mysqlTableExists(ctx, conn, "statistics_accumulated"); err != nil {
		return nil, err
	} else if exists {
		items = append(items, mysqlBackupItem{"statistics_accumulated", `SELECT a.* FROM statistics_accumulated a JOIN tmp_cleanup_testee_ids t ON BINARY a.statistic_key = BINARY CAST(t.id AS CHAR) WHERE a.statistic_type = 'testee'`})
	}
	return items, nil
}

func mysqlDeleteItems(ctx context.Context, conn *sql.Conn) ([]mysqlDeleteItem, error) {
	items := []mysqlDeleteItem{}
	if exists, err := mysqlTableExists(ctx, conn, "statistics_daily"); err != nil {
		return nil, err
	} else if exists {
		items = append(items, mysqlDeleteItem{"statistics_daily_testee", `DELETE d FROM statistics_daily d JOIN tmp_cleanup_testee_ids t ON BINARY d.statistic_key = BINARY CAST(t.id AS CHAR) WHERE d.statistic_type = 'testee'`})
	} else {
		log.Print("optional mysql table statistics_daily does not exist; skip legacy statistics_daily delete")
	}
	if exists, err := mysqlTableExists(ctx, conn, "statistics_accumulated"); err != nil {
		return nil, err
	} else if exists {
		items = append(items, mysqlDeleteItem{"statistics_accumulated_testee", `DELETE a FROM statistics_accumulated a JOIN tmp_cleanup_testee_ids t ON BINARY a.statistic_key = BINARY CAST(t.id AS CHAR) WHERE a.statistic_type = 'testee'`})
	} else {
		log.Print("optional mysql table statistics_accumulated does not exist; skip legacy statistics_accumulated delete")
	}
	items = append(items,
		mysqlDeleteItem{"analytics_projector_checkpoint", `DELETE c FROM analytics_projector_checkpoint c JOIN tmp_cleanup_event_ids x ON BINARY x.event_id = BINARY c.event_id`},
		mysqlDeleteItem{"analytics_pending_event", `DELETE p FROM analytics_pending_event p JOIN tmp_cleanup_pending_event_ids x ON BINARY x.event_id = BINARY p.event_id`},
		mysqlDeleteItem{"domain_event_outbox", `DELETE o FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id`},
		mysqlDeleteItem{"behavior_footprint", `DELETE bf FROM behavior_footprint bf LEFT JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		mysqlDeleteItem{"assessment_episode", `DELETE ae FROM assessment_episode ae LEFT JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		mysqlDeleteItem{"assessment_entry_intake_log", `DELETE l FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id`},
		mysqlDeleteItem{"clinician_relation", `DELETE r FROM clinician_relation r JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id`},
		mysqlDeleteItem{"assessment_task", `DELETE task FROM assessment_task task LEFT JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id WHERE t.id IS NOT NULL OR a.id IS NOT NULL`},
		mysqlDeleteItem{"assessment_score", `DELETE s FROM assessment_score s LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LEFT JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id WHERE a.id IS NOT NULL OR t.id IS NOT NULL`},
		mysqlDeleteItem{"assessment", `DELETE a FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id`},
		mysqlDeleteItem{"testee", `DELETE t FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
	)
	return items, nil
}

func mysqlTableExists(ctx context.Context, conn *sql.Conn, table string) (bool, error) {
	if err := validateMySQLTableName(table); err != nil {
		return false, err
	}
	// Probe the live schema instead of information_schema; some environments had
	// stale metadata while the legacy table was already dropped by migration 000028.
	_, err := conn.ExecContext(ctx, fmt.Sprintf("SELECT 1 FROM `%s` WHERE 1=0", table))
	if err == nil {
		return true, nil
	}
	if isMySQLUnknownTable(err) {
		return false, nil
	}
	return false, fmt.Errorf("probe mysql table %s: %w", table, err)
}

func validateMySQLTableName(table string) error {
	ok, err := regexp.MatchString(`^[a-z0-9_]+$`, table)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unsafe mysql table name %q", table)
	}
	return nil
}

func isMySQLUnknownTable(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1146
	}
	return strings.Contains(err.Error(), "doesn't exist")
}

func deleteMySQLRows(ctx context.Context, conn *sql.Conn, lockWaitTimeoutSec, maxRetries, batchSize int) ([]namedCount, error) {
	if lockWaitTimeoutSec > 0 {
		if _, err := conn.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout = ?", lockWaitTimeoutSec); err != nil {
			return nil, fmt.Errorf("set innodb_lock_wait_timeout: %w", err)
		}
		log.Printf("mysql delete: innodb_lock_wait_timeout=%ds per-table commit retries=%d batch_size=%d", lockWaitTimeoutSec, maxRetries, batchSize)
	}

	items, err := mysqlDeleteItems(ctx, conn)
	if err != nil {
		return nil, err
	}
	out := make([]namedCount, 0, len(items))
	for i, item := range items {
		item := item
		var n int64
		if err := prog.RunStep("delete mysql "+item.name, i+1, len(items), func() error {
			if spec, ok := mysqlChunkedDeleteSpecFor(item.name); ok && batchSize > 0 {
				return execMySQLChunkedDeleteWithRetry(ctx, conn, spec, batchSize, maxRetries, &n)
			}
			return execMySQLDeleteWithRetry(ctx, conn, item.stmt, maxRetries, &n)
		}); err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		out = append(out, namedCount{Name: item.name, Count: n})
	}
	return out, nil
}

func mysqlChunkedDeleteSpecFor(name string) (mysqlChunkedDeleteSpec, bool) {
	switch name {
	case "statistics_daily_testee":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT d.id
FROM statistics_daily d
JOIN tmp_cleanup_testee_ids t ON BINARY d.statistic_key = BINARY CAST(t.id AS CHAR)
WHERE d.statistic_type = 'testee'
ORDER BY d.id
LIMIT ?`,
			deleteBatch: `DELETE d
FROM statistics_daily d
JOIN tmp_cleanup_batch_ids b ON b.id = d.id`,
		}, true
	case "statistics_accumulated_testee":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT a.id
FROM statistics_accumulated a
JOIN tmp_cleanup_testee_ids t ON BINARY a.statistic_key = BINARY CAST(t.id AS CHAR)
WHERE a.statistic_type = 'testee'
ORDER BY a.id
LIMIT ?`,
			deleteBatch: `DELETE a
FROM statistics_accumulated a
JOIN tmp_cleanup_batch_ids b ON b.id = a.id`,
		}, true
	case "analytics_projector_checkpoint":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_event_ids LIKE tmp_cleanup_event_ids`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_event_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_event_ids (event_id)
SELECT c.event_id
FROM analytics_projector_checkpoint c
JOIN tmp_cleanup_event_ids x ON x.event_id = c.event_id
ORDER BY c.event_id
LIMIT ?`,
			deleteBatch: `DELETE c
FROM analytics_projector_checkpoint c
JOIN tmp_cleanup_batch_event_ids b ON b.event_id = c.event_id`,
		}, true
	case "analytics_pending_event":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_event_ids LIKE tmp_cleanup_event_ids`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_event_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_event_ids (event_id)
SELECT p.event_id
FROM analytics_pending_event p
JOIN tmp_cleanup_pending_event_ids x ON x.event_id = p.event_id
ORDER BY p.event_id
LIMIT ?`,
			deleteBatch: `DELETE p
FROM analytics_pending_event p
JOIN tmp_cleanup_batch_event_ids b ON b.event_id = p.event_id`,
		}, true
	case "domain_event_outbox":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT o.id
FROM domain_event_outbox o
JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id
ORDER BY o.id
LIMIT ?`,
			deleteBatch: `DELETE o
FROM domain_event_outbox o
JOIN tmp_cleanup_batch_ids b ON b.id = o.id`,
		}, true
	case "behavior_footprint":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_string_ids (id VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_string_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_string_ids (id)
SELECT id FROM (
  (SELECT bf.id AS id FROM behavior_footprint bf JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LIMIT ?)
  UNION DISTINCT
  (SELECT bf.id AS id FROM behavior_footprint bf JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LIMIT ?)
  UNION DISTINCT
  (SELECT bf.id AS id FROM behavior_footprint bf JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LIMIT ?)
  UNION DISTINCT
  (SELECT bf.id AS id FROM behavior_footprint bf JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id LIMIT ?)
) scoped
ORDER BY id
LIMIT ?`,
			deleteBatch: `DELETE bf
FROM behavior_footprint bf
JOIN tmp_cleanup_batch_string_ids b ON b.id = bf.id`,
		}, true
	case "assessment_episode":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT id FROM (
  (SELECT ae.episode_id AS id FROM assessment_episode ae JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LIMIT ?)
  UNION DISTINCT
  (SELECT ae.episode_id AS id FROM assessment_episode ae JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LIMIT ?)
  UNION DISTINCT
  (SELECT ae.episode_id AS id FROM assessment_episode ae JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LIMIT ?)
  UNION DISTINCT
  (SELECT ae.episode_id AS id FROM assessment_episode ae JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id LIMIT ?)
) scoped
ORDER BY id
LIMIT ?`,
			deleteBatch: `DELETE ae
FROM assessment_episode ae
JOIN tmp_cleanup_batch_ids b ON b.id = ae.episode_id`,
		}, true
	case "assessment_entry_intake_log":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT l.id
FROM assessment_entry_intake_log l
JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id
ORDER BY l.id
LIMIT ?`,
			deleteBatch: `DELETE l
FROM assessment_entry_intake_log l
JOIN tmp_cleanup_batch_ids b ON b.id = l.id`,
		}, true
	case "clinician_relation":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT r.id
FROM clinician_relation r
JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id
ORDER BY r.id
LIMIT ?`,
			deleteBatch: `DELETE r
FROM clinician_relation r
JOIN tmp_cleanup_batch_ids b ON b.id = r.id`,
		}, true
	case "assessment_task":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT id FROM (
  (SELECT task.id AS id FROM assessment_task task JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LIMIT ?)
  UNION DISTINCT
  (SELECT task.id AS id FROM assessment_task task JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id LIMIT ?)
) scoped
ORDER BY id
LIMIT ?`,
			deleteBatch: `DELETE task
FROM assessment_task task
JOIN tmp_cleanup_batch_ids b ON b.id = task.id`,
		}, true
	case "assessment_score":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT id FROM (
  (SELECT s.id AS id FROM assessment_score s JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LIMIT ?)
  UNION DISTINCT
  (SELECT s.id AS id FROM assessment_score s JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id LIMIT ?)
) scoped
ORDER BY id
LIMIT ?`,
			deleteBatch: `DELETE s
FROM assessment_score s
JOIN tmp_cleanup_batch_ids b ON b.id = s.id`,
		}, true
	case "assessment":
		return mysqlChunkedDeleteSpec{
			name:             name,
			createBatchTable: `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_batch_ids (id BIGINT UNSIGNED NOT NULL PRIMARY KEY)`,
			clearBatchTable:  `DELETE FROM tmp_cleanup_batch_ids`,
			fillBatchTable: `INSERT IGNORE INTO tmp_cleanup_batch_ids (id)
SELECT a.id
FROM assessment a
JOIN tmp_cleanup_assessment_ids x ON x.id = a.id
ORDER BY a.id
LIMIT ?`,
			deleteBatch: `DELETE a
FROM assessment a
JOIN tmp_cleanup_batch_ids b ON b.id = a.id`,
		}, true
	default:
		return mysqlChunkedDeleteSpec{}, false
	}
}

func execMySQLChunkedDeleteWithRetry(ctx context.Context, conn *sql.Conn, spec mysqlChunkedDeleteSpec, batchSize, maxRetries int, affected *int64) error {
	if batchSize <= 0 {
		return fmt.Errorf("invalid mysql delete batch size %d", batchSize)
	}
	if _, err := conn.ExecContext(ctx, spec.createBatchTable); err != nil {
		return fmt.Errorf("create batch table: %w", err)
	}
	var total int64
	for batch := 1; ; batch++ {
		selected, deleted, err := execMySQLChunkedDeleteBatchWithRetry(ctx, conn, spec, batchSize, maxRetries)
		if err != nil {
			return err
		}
		if selected == 0 {
			*affected = total
			return nil
		}
		total += deleted
		log.Printf("mysql delete %s: batch=%d selected=%d deleted=%d total=%d", spec.name, batch, selected, deleted, total)
		if selected < int64(batchSize) {
			*affected = total
			return nil
		}
	}
}

func execMySQLChunkedDeleteBatchWithRetry(ctx context.Context, conn *sql.Conn, spec mysqlChunkedDeleteSpec, batchSize, maxRetries int) (int64, int64, error) {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		selected, deleted, err := execMySQLChunkedDeleteBatch(ctx, conn, spec, batchSize)
		if err == nil {
			return selected, deleted, nil
		}
		lastErr = err
		if isMySQLLockError(err) && attempt < maxRetries {
			wait := time.Duration(attempt) * 5 * time.Second
			log.Printf("mysql delete %s lock contention: attempt=%d/%d retry_in=%s err=%v", spec.name, attempt, maxRetries, wait, err)
			if err := sleepWithContext(ctx, wait); err != nil {
				return 0, 0, err
			}
			continue
		}
		return 0, 0, err
	}
	return 0, 0, lastErr
}

func execMySQLChunkedDeleteBatch(ctx context.Context, conn *sql.Conn, spec mysqlChunkedDeleteSpec, batchSize int) (int64, int64, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, spec.clearBatchTable); err != nil {
		_ = tx.Rollback()
		return 0, 0, fmt.Errorf("clear batch table: %w", err)
	}
	fillArgs := make([]any, strings.Count(spec.fillBatchTable, "?"))
	for i := range fillArgs {
		fillArgs[i] = batchSize
	}
	fillResult, err := tx.ExecContext(ctx, spec.fillBatchTable, fillArgs...)
	if err != nil {
		_ = tx.Rollback()
		return 0, 0, fmt.Errorf("fill batch table: %w", err)
	}
	selected, _ := fillResult.RowsAffected()
	if selected == 0 {
		if err := tx.Commit(); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	}
	deleteResult, err := tx.ExecContext(ctx, spec.deleteBatch)
	if err != nil {
		_ = tx.Rollback()
		return 0, 0, fmt.Errorf("delete batch: %w", err)
	}
	deleted, _ := deleteResult.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return selected, deleted, nil
}

func execMySQLDeleteWithRetry(ctx context.Context, conn *sql.Conn, stmt string, maxRetries int, affected *int64) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, stmt)
		if err != nil {
			_ = tx.Rollback()
			lastErr = err
			if isMySQLLockError(err) && attempt < maxRetries {
				wait := time.Duration(attempt) * 5 * time.Second
				log.Printf("mysql delete lock contention: attempt=%d/%d retry_in=%s err=%v", attempt, maxRetries, wait, err)
				if err := sleepWithContext(ctx, wait); err != nil {
					return err
				}
				continue
			}
			return err
		}
		n, _ := result.RowsAffected()
		if err := tx.Commit(); err != nil {
			lastErr = err
			if isMySQLLockError(err) && attempt < maxRetries {
				wait := time.Duration(attempt) * 5 * time.Second
				log.Printf("mysql delete commit lock contention: attempt=%d/%d retry_in=%s err=%v", attempt, maxRetries, wait, err)
				if err := sleepWithContext(ctx, wait); err != nil {
					return err
				}
				continue
			}
			return err
		}
		*affected = n
		return nil
	}
	return lastErr
}

func isMySQLLockError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1205 || mysqlErr.Number == 1213
	}
	return false
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func deleteMongoRows(ctx context.Context, db *mongo.Database, ids scopeIDs, workers int) ([]namedCount, error) {
	items := mongoCollectionScopes(ids)
	out := make([]namedCount, len(items))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for i, item := range items {
		i, item := i, item
		group.Go(func() error {
			var deleted int64
			for _, filter := range item.filters {
				result, err := db.Collection(item.coll).DeleteMany(groupCtx, filter)
				if err != nil {
					return err
				}
				deleted += result.DeletedCount
			}
			out[i] = namedCount{Name: item.name, Count: deleted}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

type mongoCollectionScope struct {
	name    string
	coll    string
	filters []bson.M
}

func mongoCollectionScopes(ids scopeIDs) []mongoCollectionScope {
	return []mongoCollectionScope{
		{name: "answersheets", coll: "answersheets", filters: answersheetFilters(ids)},
		{name: "answersheet_submit_idempotency", coll: "answersheet_submit_idempotency", filters: answerSheetIdempotencyFilters(ids)},
		{name: "interpret_reports", coll: "interpret_reports", filters: reportFilters(ids)},
		{name: "domain_event_outbox", coll: "domain_event_outbox", filters: mongoOutboxFilters(ids)},
	}
}

func countMongoDocumentsByFilters(ctx context.Context, coll *mongo.Collection, filters []bson.M, label string) (int64, error) {
	seen := map[string]struct{}{}
	for i, filter := range filters {
		filter := filter
		if err := prog.RunStep(label+" filters", i+1, len(filters), func() error {
			cur, err := coll.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
			if err != nil {
				return err
			}
			for cur.Next(ctx) {
				var row bson.M
				if err := cur.Decode(&row); err != nil {
					_ = cur.Close(ctx)
					return err
				}
				key := mongoDocumentIDKey(row["_id"])
				if key == "" {
					continue
				}
				seen[key] = struct{}{}
				if len(seen)%10000 == 0 {
					prog.Step(label, int64(len(seen)), 0)
				}
			}
			if err := cur.Err(); err != nil {
				_ = cur.Close(ctx)
				return err
			}
			return cur.Close(ctx)
		}); err != nil {
			return 0, err
		}
	}
	return int64(len(seen)), nil
}

func answersheetFilters(ids scopeIDs) []bson.M {
	return append(
		inUint64Filters("testee_id", ids.TesteeIDs),
		inUint64Filters("domain_id", ids.AnswerSheetIDs)...,
	)
}

func answerSheetIdempotencyFilters(ids scopeIDs) []bson.M {
	return append(
		inUint64Filters("testee_id", ids.TesteeIDs),
		inUint64Filters("answersheet_id", ids.AnswerSheetIDs)...,
	)
}

func reportFilters(ids scopeIDs) []bson.M {
	filters := inUint64Filters("testee_id", ids.TesteeIDs)
	filters = append(filters, inUint64Filters("domain_id", ids.ReportIDs)...)
	filters = append(filters, inUint64Filters("domain_id", ids.AssessmentIDs)...)
	return filters
}

func mongoOutboxFilters(ids scopeIDs) []bson.M {
	answerStrings := uint64Strings(ids.AnswerSheetIDs)
	assessmentStrings := uint64Strings(ids.AssessmentIDs)
	reportStrings := uint64Strings(ids.ReportIDs)
	testeeStrings := uint64Strings(ids.TesteeIDs)
	behaviorStrings := uniqueStrings(answerStrings, assessmentStrings, reportStrings, testeeStrings)

	filters := inStringWithAggregateFilters("AnswerSheet", answerStrings)
	filters = append(filters, inStringWithAggregateFilters("Assessment", assessmentStrings)...)
	filters = append(filters, inStringWithAggregateFilters("Report", reportStrings)...)
	filters = append(filters, inStringWithAggregateFilters("BehaviorFootprint", behaviorStrings)...)
	return filters
}

func inUint64(field string, values []uint64) bson.M {
	if len(values) == 0 {
		return nil
	}
	return bson.M{field: bson.M{"$in": values}}
}

func inUint64Filters(field string, values []uint64) []bson.M {
	values = uniqueUint64(values)
	filters := make([]bson.M, 0, (len(values)+mongoIDChunkSize-1)/mongoIDChunkSize)
	for start := 0; start < len(values); start += mongoIDChunkSize {
		end := start + mongoIDChunkSize
		if end > len(values) {
			end = len(values)
		}
		filters = append(filters, bson.M{field: bson.M{"$in": values[start:end]}})
	}
	return filters
}

func inStringWithAggregateFilters(aggregateType string, values []string) []bson.M {
	values = uniqueStrings(values)
	filters := make([]bson.M, 0, (len(values)+mongoIDChunkSize-1)/mongoIDChunkSize)
	for start := 0; start < len(values); start += mongoIDChunkSize {
		end := start + mongoIDChunkSize
		if end > len(values) {
			end = len(values)
		}
		filters = append(filters, bson.M{
			"aggregate_type": aggregateType,
			"aggregate_id":   bson.M{"$in": values[start:end]},
		})
	}
	return filters
}

func mongoDocumentIDKey(id any) string {
	switch value := id.(type) {
	case primitive.ObjectID:
		return "oid:" + value.Hex()
	case string:
		return "str:" + value
	case int32:
		return fmt.Sprintf("i32:%d", value)
	case int64:
		return fmt.Sprintf("i64:%d", value)
	case int:
		return fmt.Sprintf("i:%d", value)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func uint64Strings(values []uint64) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.FormatUint(value, 10))
	}
	return out
}

func uniqueStrings(groups ...[]string) []string {
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			if value == "" {
				continue
			}
			seen[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueUint64(values []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(values))
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func scopeIDsEqual(a, b scopeIDs) bool {
	return uint64SlicesEqual(uniqueUint64(a.TesteeIDs), uniqueUint64(b.TesteeIDs)) &&
		uint64SlicesEqual(uniqueUint64(a.AssessmentIDs), uniqueUint64(b.AssessmentIDs)) &&
		uint64SlicesEqual(uniqueUint64(a.AnswerSheetIDs), uniqueUint64(b.AnswerSheetIDs)) &&
		uint64SlicesEqual(uniqueUint64(a.ReportIDs), uniqueUint64(b.ReportIDs))
}

func uint64SlicesEqual(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validateBackupSuffix(suffix string) error {
	if suffix == "" {
		return errors.New("empty suffix")
	}
	ok, err := regexp.MatchString(`^[A-Za-z0-9_]+$`, suffix)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("suffix %q must contain only letters, digits, and underscore", suffix)
	}
	return nil
}

func printScopeSummary(summary scopeSummary, cfg config) {
	log.Printf("scope: apply=%v backup=%v testee_created_after=%q allow_old_testees=%v",
		cfg.apply, !cfg.skipBackup, cfg.testeeCreatedAfter, cfg.allowOldTestees)
	log.Printf("scope ids: testees=%d assessments=%d answersheets=%d reports=%d event_ids=%d org_ids=%v",
		summary.Testees, summary.Assessments, summary.AnswerSheets, summary.Reports, summary.EventIDs, summary.OrgIDs)
	if summary.MinTouchedDate.Valid || summary.MaxTouchedDate.Valid {
		log.Printf("affected source date window: min=%s max=%s", nullableString(summary.MinTouchedDate), nullableString(summary.MaxTouchedDate))
	}
}

func printScopeIDsSummary(ids scopeIDs, cfg config) {
	log.Printf("scope: apply=%v backup=%v testee_created_after=%q allow_old_testees=%v skip_counts=true",
		cfg.apply, !cfg.skipBackup, cfg.testeeCreatedAfter, cfg.allowOldTestees)
	log.Printf("scope ids: testees=%d assessments=%d answersheets=%d reports=%d",
		len(ids.TesteeIDs), len(ids.AssessmentIDs), len(ids.AnswerSheetIDs), len(ids.ReportIDs))
}

func printCounts(prefix string, counts []namedCount) {
	for _, item := range counts {
		log.Printf("%s %s=%d", prefix, item.Name, item.Count)
	}
}

func printTesteePreview(rows []testeePreview) {
	for _, row := range rows {
		createdAt := ""
		if row.CreatedAt.Valid {
			createdAt = row.CreatedAt.Time.Format("2006-01-02 15:04:05")
		}
		log.Printf("preview testee id=%d org=%d name=%s created_at=%s assessment_cnt=%d",
			row.ID, row.OrgID, row.Name, createdAt, row.AssessmentCnt)
	}
}

func nullableString(s sql.NullString) string {
	if !s.Valid {
		return "-"
	}
	return s.String
}

type progressReporter struct {
	mu           sync.Mutex
	enabled      bool
	tty          bool
	out          io.Writer
	phase        string
	label        string
	current      int64
	total        int64
	phaseStarted time.Time
	stepStarted  time.Time
	lastDraw     time.Time
	lastLogAt    time.Time
}

func initProgress(disable bool) {
	prog = progressReporter{
		enabled: !disable,
		tty:     isTerminal(os.Stderr),
		out:     os.Stderr,
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (p *progressReporter) Phase(name string) {
	if !p.enabled {
		log.Printf("phase: %s", name)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phase = name
	p.label = name
	p.current = 0
	p.total = 0
	now := time.Now()
	p.phaseStarted = now
	p.stepStarted = now
	p.lastDraw = time.Time{}
	p.lastLogAt = time.Time{}
	p.renderLocked()
}

func (p *progressReporter) Step(label string, current, total int64) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.label = label
	p.current = current
	p.total = total
	p.stepStarted = time.Now()
	p.renderLocked()
}

func (p *progressReporter) Indeterminate(label string) {
	p.Step(label, 0, 0)
}

func (p *progressReporter) Add(delta int64) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current += delta
	p.renderLocked()
}

func (p *progressReporter) Finish(label string, detail string) {
	if !p.enabled {
		msg := label
		if detail != "" {
			msg += ": " + detail
		}
		log.Printf("done: %s", msg)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := formatProgressDuration(time.Since(p.phaseStarted))
	p.clearLineLocked()
	line := fmt.Sprintf("done: %s (%s)", label, elapsed)
	if detail != "" {
		line += " " + detail
	}
	_, _ = fmt.Fprintln(p.out, line)
	p.label = ""
	p.current = 0
	p.total = 0
	p.phaseStarted = time.Time{}
	p.stepStarted = time.Time{}
}

func (p *progressReporter) SubtaskDone(label, detail string) {
	if !p.enabled {
		msg := label
		if detail != "" {
			msg += ": " + detail
		}
		log.Printf("done: %s", msg)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	started := p.stepStarted
	if started.IsZero() {
		started = p.phaseStarted
	}
	elapsed := formatProgressDuration(time.Since(started))
	p.clearLineLocked()
	line := fmt.Sprintf("done: %s (%s)", label, elapsed)
	if detail != "" {
		line += " " + detail
	}
	_, _ = fmt.Fprintln(p.out, line)
}

func (p *progressReporter) RunStep(label string, index, total int, fn func() error) error {
	if total > 0 {
		p.Step(label, int64(index), int64(total))
	} else {
		p.Indeterminate(label)
	}
	err := fn()
	if err != nil {
		if p.enabled {
			p.mu.Lock()
			p.clearLineLocked()
			_, _ = fmt.Fprintf(p.out, "failed: %s: %v\n", label, err)
			p.mu.Unlock()
		}
		return err
	}
	return nil
}

func (p *progressReporter) renderLocked() {
	now := time.Now()
	if p.tty {
		if !p.lastDraw.IsZero() && now.Sub(p.lastDraw) < 100*time.Millisecond {
			return
		}
		p.lastDraw = now
		_, _ = fmt.Fprintf(p.out, "\r%s", p.buildLineLocked())
		return
	}
	if p.lastLogAt.IsZero() || now.Sub(p.lastLogAt) >= 5*time.Second {
		p.lastLogAt = now
		log.Print(strings.TrimPrefix(p.buildLineLocked(), "\r"))
	}
}

func (p *progressReporter) buildLineLocked() string {
	elapsed := formatProgressDuration(time.Since(p.phaseStarted))
	title := p.phase
	if p.label != "" && p.label != p.phase {
		title = p.phase + " | " + p.label
	}
	title = truncateProgressText(title, 48)
	if p.total > 0 {
		current := p.current
		if current > p.total {
			current = p.total
		}
		pct := float64(current) / float64(p.total)
		filled := int(pct * progressBarWidth)
		if filled > progressBarWidth {
			filled = progressBarWidth
		}
		bar := strings.Repeat("=", filled) + strings.Repeat("-", progressBarWidth-filled)
		return fmt.Sprintf("%s [%s] %3.0f%% (%d/%d) elapsed=%s", title, bar, pct*100, current, p.total, elapsed)
	}
	if p.current > 0 {
		return fmt.Sprintf("%s ... count=%d elapsed=%s", title, p.current, elapsed)
	}
	return fmt.Sprintf("%s ... elapsed=%s", title, elapsed)
}

func (p *progressReporter) clearLineLocked() {
	if !p.tty {
		return
	}
	_, _ = fmt.Fprint(p.out, "\r\033[K")
}

func truncateProgressText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatProgressDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}
