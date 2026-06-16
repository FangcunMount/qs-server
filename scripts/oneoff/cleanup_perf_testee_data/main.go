package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mysqlDSN           string
	mongoURI           string
	mongoDB            string
	testeeIDsRaw       string
	testeeIDsFile      string
	testeeCreatedAfter string
	allowOldTestees    bool
	deriveIDsFromFacts bool
	scanEventPayloads  bool
	backupSuffix       string
	timeout            time.Duration
	apply              bool
	skipBackup         bool
	previewLimit       int
}

type namedCount struct {
	Name  string
	Count int64
}

type namedSQL struct {
	name string
	sql  string
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
	testeeIDs, err := parseTesteeIDs(cfg.testeeIDsRaw, cfg.testeeIDsFile)
	if err != nil {
		log.Fatalf("parse testee ids: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

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

	if err := prepareMySQLScope(ctx, conn, cfg, testeeIDs); err != nil {
		log.Fatalf("prepare mysql scope: %v", err)
	}
	ids, err := loadScopeIDs(ctx, conn)
	if err != nil {
		log.Fatalf("load scope ids: %v", err)
	}
	ids, err = enrichScopeIDsFromMongo(ctx, mongoDB, ids)
	if err != nil {
		log.Fatalf("enrich scope ids from mongo: %v", err)
	}
	if err := storeScopeIDs(ctx, conn, ids); err != nil {
		log.Fatalf("store enriched scope ids: %v", err)
	}
	if err := addMongoOutboxEventIDsToMySQLScope(ctx, conn, mongoDB, ids); err != nil {
		log.Fatalf("load mongo outbox event ids: %v", err)
	}

	summary, err := loadScopeSummary(ctx, conn)
	if err != nil {
		log.Fatalf("load scope summary: %v", err)
	}
	mysqlCounts, err := countMySQLRows(ctx, conn)
	if err != nil {
		log.Fatalf("count mysql rows: %v", err)
	}
	mongoCounts, err := countMongoRows(ctx, mongoDB, ids)
	if err != nil {
		log.Fatalf("count mongo rows: %v", err)
	}

	printScopeSummary(summary, cfg)
	printCounts("mysql", mysqlCounts)
	printCounts("mongo", mongoCounts)

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
		if err := backupMongoRows(ctx, mongoDB, ids, cfg.backupSuffix); err != nil {
			log.Fatalf("backup mongo rows: %v", err)
		}
		if err := backupMySQLRows(ctx, conn, cfg.backupSuffix); err != nil {
			log.Fatalf("backup mysql rows: %v", err)
		}
		log.Printf("backup completed: suffix=%s", cfg.backupSuffix)
	} else {
		log.Print("backup skipped by --skip-backup")
	}

	mysqlDeleted, err := deleteMySQLRows(ctx, conn)
	if err != nil {
		log.Fatalf("delete mysql rows: %v", err)
	}
	mongoDeleted, err := deleteMongoRows(ctx, mongoDB, ids)
	if err != nil {
		log.Fatalf("delete mongo rows: %v", err)
	}
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
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", time.Now().Format("20060102150405"), "backup table/collection suffix")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "overall timeout, for example 30m or 2h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply deletes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip built-in MySQL/Mongo backups before deleting")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 20, "number of scoped testees to preview")
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
		log.Printf("prepare mysql scope: event_id collation=%s", eventIDCollation)
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
	for _, stmt := range stmts {
		log.Printf("prepare mysql scope: %s", firstSQLLine(stmt))
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	for _, id := range testeeIDs {
		log.Printf("prepare mysql scope: insert testee id=%d", id)
		if _, err := conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_testee_ids (id) VALUES (?)`, id); err != nil {
			return fmt.Errorf("insert testee id %d: %w", id, err)
		}
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
	for _, item := range populate {
		log.Printf("prepare mysql scope: %s", item.name)
		if _, err := conn.ExecContext(ctx, item.sql); err != nil {
			return err
		}
	}

	outboxStmts := []namedSQL{
		{"mysql outbox ids from assessment aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_assessment_ids a ON o.aggregate_id = CAST(a.id AS CHAR) WHERE o.aggregate_type = 'Assessment'`,
		},
		{"mysql outbox ids from report aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_report_ids r ON o.aggregate_id = CAST(r.id AS CHAR) WHERE o.aggregate_type = 'Report'`,
		},
		{"mysql outbox ids from answersheet aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_answersheet_ids s ON o.aggregate_id = CAST(s.id AS CHAR) WHERE o.aggregate_type = 'AnswerSheet'`,
		},
		{"mysql outbox ids from testee aggregate", `INSERT IGNORE INTO tmp_cleanup_mysql_outbox_ids (id, event_id)
SELECT o.id, o.event_id FROM domain_event_outbox o JOIN tmp_cleanup_testee_ids t ON o.aggregate_id = CAST(t.id AS CHAR)`,
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
SELECT p.event_id FROM analytics_pending_event p JOIN tmp_cleanup_event_ids e ON e.event_id = p.event_id`,
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
	for _, item := range outboxStmts {
		log.Printf("prepare mysql scope: %s", item.name)
		if _, err := conn.ExecContext(ctx, item.sql); err != nil {
			return err
		}
	}
	return nil
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

func enrichScopeIDsFromMongo(ctx context.Context, db *mongo.Database, ids scopeIDs) (scopeIDs, error) {
	answerSheetIDs, err := loadMongoUint64Field(ctx, db.Collection("answersheets"), inUint64("testee_id", ids.TesteeIDs), "domain_id")
	if err != nil {
		return ids, fmt.Errorf("load answersheet domain ids: %w", err)
	}
	idempotencyAnswerSheetIDs, err := loadMongoUint64Field(ctx, db.Collection("answersheet_submit_idempotency"), inUint64("testee_id", ids.TesteeIDs), "answersheet_id")
	if err != nil {
		return ids, fmt.Errorf("load answersheet idempotency ids: %w", err)
	}
	reportIDs, err := loadMongoUint64Field(ctx, db.Collection("interpret_reports"), inUint64("testee_id", ids.TesteeIDs), "domain_id")
	if err != nil {
		return ids, fmt.Errorf("load report domain ids: %w", err)
	}

	ids.AnswerSheetIDs = uniqueUint64(append(append(ids.AnswerSheetIDs, answerSheetIDs...), idempotencyAnswerSheetIDs...))
	ids.ReportIDs = uniqueUint64(append(ids.ReportIDs, reportIDs...))
	return ids, nil
}

func loadMongoUint64Field(ctx context.Context, coll *mongo.Collection, filter bson.M, field string) ([]uint64, error) {
	if len(filter) == 0 {
		return nil, nil
	}
	cur, err := coll.Find(ctx, filter, options.Find().SetProjection(bson.M{field: 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
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
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return uniqueUint64(out), nil
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
	for _, id := range ids.AssessmentIDs {
		if _, err := conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_assessment_ids (id) VALUES (?)`, id); err != nil {
			return fmt.Errorf("store assessment id %d: %w", id, err)
		}
	}
	for _, id := range ids.AnswerSheetIDs {
		if _, err := conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_answersheet_ids (id) VALUES (?)`, id); err != nil {
			return fmt.Errorf("store answersheet id %d: %w", id, err)
		}
	}
	for _, id := range ids.ReportIDs {
		if _, err := conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_report_ids (id) VALUES (?)`, id); err != nil {
			return fmt.Errorf("store report id %d: %w", id, err)
		}
	}
	return nil
}

func addMongoOutboxEventIDsToMySQLScope(ctx context.Context, conn *sql.Conn, db *mongo.Database, ids scopeIDs) error {
	filter := mongoOutboxFilter(ids)
	if len(filter) == 0 {
		return nil
	}
	cur, err := db.Collection("domain_event_outbox").Find(ctx, filter, options.Find().SetProjection(bson.M{"event_id": 1}))
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		var row struct {
			EventID string `bson:"event_id"`
		}
		if err := cur.Decode(&row); err != nil {
			return err
		}
		if row.EventID == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_event_ids (event_id) VALUES (?)`, row.EventID); err != nil {
			return err
		}
	}
	if err := cur.Err(); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_pending_event_ids (event_id)
SELECT p.event_id FROM analytics_pending_event p JOIN tmp_cleanup_event_ids e ON e.event_id = p.event_id`)
	return err
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
	items := []struct {
		name  string
		query string
	}{
		{"testee", `SELECT COUNT(*) FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
		{"assessment", `SELECT COUNT(*) FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id`},
		{"assessment_score", `SELECT COUNT(*) FROM assessment_score s LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LEFT JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id WHERE a.id IS NOT NULL OR t.id IS NOT NULL`},
		{"assessment_task", `SELECT COUNT(*) FROM assessment_task task LEFT JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id WHERE t.id IS NOT NULL OR a.id IS NOT NULL`},
		{"clinician_relation", `SELECT COUNT(*) FROM clinician_relation r JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id`},
		{"assessment_entry_intake_log", `SELECT COUNT(*) FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id`},
		{"behavior_footprint", `SELECT COUNT(*) FROM behavior_footprint bf LEFT JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"assessment_episode", `SELECT COUNT(*) FROM assessment_episode ae LEFT JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"domain_event_outbox", `SELECT COUNT(*) FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id`},
		{"analytics_pending_event", `SELECT COUNT(*) FROM analytics_pending_event p JOIN tmp_cleanup_pending_event_ids x ON x.event_id = p.event_id`},
		{"analytics_projector_checkpoint", `SELECT COUNT(*) FROM analytics_projector_checkpoint c JOIN tmp_cleanup_event_ids x ON x.event_id = c.event_id`},
		{"statistics_daily_testee", `SELECT COUNT(*) FROM statistics_daily d JOIN tmp_cleanup_testee_ids t ON d.statistic_key = CAST(t.id AS CHAR) WHERE d.statistic_type = 'testee'`},
		{"statistics_accumulated_testee", `SELECT COUNT(*) FROM statistics_accumulated a JOIN tmp_cleanup_testee_ids t ON a.statistic_key = CAST(t.id AS CHAR) WHERE a.statistic_type = 'testee'`},
	}
	out := make([]namedCount, 0, len(items))
	for _, item := range items {
		var count int64
		if err := conn.QueryRowContext(ctx, item.query).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		out = append(out, namedCount{Name: item.name, Count: count})
	}
	return out, nil
}

func countMongoRows(ctx context.Context, db *mongo.Database, ids scopeIDs) ([]namedCount, error) {
	items := []struct {
		name   string
		coll   string
		filter bson.M
	}{
		{"answersheets", "answersheets", answersheetFilter(ids)},
		{"answersheet_submit_idempotency", "answersheet_submit_idempotency", answerSheetIdempotencyFilter(ids)},
		{"interpret_reports", "interpret_reports", reportFilter(ids)},
		{"domain_event_outbox", "domain_event_outbox", mongoOutboxFilter(ids)},
	}
	out := make([]namedCount, 0, len(items))
	for _, item := range items {
		count, err := db.Collection(item.coll).CountDocuments(ctx, item.filter)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		out = append(out, namedCount{Name: item.name, Count: count})
	}
	return out, nil
}

func backupMySQLRows(ctx context.Context, conn *sql.Conn, suffix string) error {
	items := []struct {
		table     string
		selectSQL string
	}{
		{"testee", `SELECT t.* FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
		{"assessment", `SELECT a.* FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id`},
		{"assessment_score", `SELECT s.* FROM assessment_score s LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LEFT JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id WHERE a.id IS NOT NULL OR t.id IS NOT NULL`},
		{"assessment_task", `SELECT task.* FROM assessment_task task LEFT JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id WHERE t.id IS NOT NULL OR a.id IS NOT NULL`},
		{"clinician_relation", `SELECT r.* FROM clinician_relation r JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id`},
		{"assessment_entry_intake_log", `SELECT l.* FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id`},
		{"behavior_footprint", `SELECT bf.* FROM behavior_footprint bf LEFT JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"assessment_episode", `SELECT ae.* FROM assessment_episode ae LEFT JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"domain_event_outbox", `SELECT o.* FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id`},
		{"analytics_pending_event", `SELECT p.* FROM analytics_pending_event p JOIN tmp_cleanup_pending_event_ids x ON x.event_id = p.event_id`},
		{"analytics_projector_checkpoint", `SELECT c.* FROM analytics_projector_checkpoint c JOIN tmp_cleanup_event_ids x ON x.event_id = c.event_id`},
		{"statistics_daily", `SELECT d.* FROM statistics_daily d JOIN tmp_cleanup_testee_ids t ON d.statistic_key = CAST(t.id AS CHAR) WHERE d.statistic_type = 'testee'`},
		{"statistics_accumulated", `SELECT a.* FROM statistics_accumulated a JOIN tmp_cleanup_testee_ids t ON a.statistic_key = CAST(t.id AS CHAR) WHERE a.statistic_type = 'testee'`},
	}
	for _, item := range items {
		backupTable := fmt.Sprintf("cleanup_bak_perf_testee_%s_%s", item.table, suffix)
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE `%s`", backupTable, item.table)); err != nil {
			return fmt.Errorf("create backup table %s: %w", backupTable, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("INSERT IGNORE INTO `%s` %s", backupTable, item.selectSQL)); err != nil {
			return fmt.Errorf("insert backup table %s: %w", backupTable, err)
		}
	}
	return nil
}

func backupMongoRows(ctx context.Context, db *mongo.Database, ids scopeIDs, suffix string) error {
	items := []struct {
		coll   string
		filter bson.M
	}{
		{"answersheets", answersheetFilter(ids)},
		{"answersheet_submit_idempotency", answerSheetIdempotencyFilter(ids)},
		{"interpret_reports", reportFilter(ids)},
		{"domain_event_outbox", mongoOutboxFilter(ids)},
	}
	for _, item := range items {
		backupName := "cleanup_bak_perf_testee_" + item.coll + "_" + suffix
		count, err := backupMongoCollection(ctx, db.Collection(item.coll), db.Collection(backupName), item.filter)
		if err != nil {
			return fmt.Errorf("backup mongo %s: %w", item.coll, err)
		}
		log.Printf("mongo backup: source=%s backup=%s docs=%d", item.coll, backupName, count)
	}
	return nil
}

func backupMongoCollection(ctx context.Context, source, backup *mongo.Collection, filter bson.M) (int64, error) {
	cur, err := source.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	defer func() { _ = cur.Close(ctx) }()
	batch := make([]interface{}, 0, 1000)
	var total int64
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		result, err := backup.InsertMany(ctx, batch, options.InsertMany().SetOrdered(false))
		if err != nil {
			return err
		}
		total += int64(len(result.InsertedIDs))
		batch = batch[:0]
		return nil
	}
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return total, err
		}
		batch = append(batch, doc)
		if len(batch) >= cap(batch) {
			if err := flush(); err != nil {
				return total, err
			}
		}
	}
	if err := cur.Err(); err != nil {
		return total, err
	}
	return total, flush()
}

func deleteMySQLRows(ctx context.Context, conn *sql.Conn) ([]namedCount, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	items := []struct {
		name string
		stmt string
	}{
		{"statistics_daily_testee", `DELETE d FROM statistics_daily d JOIN tmp_cleanup_testee_ids t ON d.statistic_key = CAST(t.id AS CHAR) WHERE d.statistic_type = 'testee'`},
		{"statistics_accumulated_testee", `DELETE a FROM statistics_accumulated a JOIN tmp_cleanup_testee_ids t ON a.statistic_key = CAST(t.id AS CHAR) WHERE a.statistic_type = 'testee'`},
		{"analytics_projector_checkpoint", `DELETE c FROM analytics_projector_checkpoint c JOIN tmp_cleanup_event_ids x ON x.event_id = c.event_id`},
		{"analytics_pending_event", `DELETE p FROM analytics_pending_event p JOIN tmp_cleanup_pending_event_ids x ON x.event_id = p.event_id`},
		{"domain_event_outbox", `DELETE o FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id = o.id`},
		{"behavior_footprint", `DELETE bf FROM behavior_footprint bf LEFT JOIN tmp_cleanup_testee_ids t ON t.id = bf.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = bf.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = bf.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = bf.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"assessment_episode", `DELETE ae FROM assessment_episode ae LEFT JOIN tmp_cleanup_testee_ids t ON t.id = ae.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id = ae.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = ae.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id = ae.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"assessment_entry_intake_log", `DELETE l FROM assessment_entry_intake_log l JOIN tmp_cleanup_testee_ids t ON t.id = l.testee_id`},
		{"clinician_relation", `DELETE r FROM clinician_relation r JOIN tmp_cleanup_testee_ids t ON t.id = r.testee_id`},
		{"assessment_task", `DELETE task FROM assessment_task task LEFT JOIN tmp_cleanup_testee_ids t ON t.id = task.testee_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = task.assessment_id WHERE t.id IS NOT NULL OR a.id IS NOT NULL`},
		{"assessment_score", `DELETE s FROM assessment_score s LEFT JOIN tmp_cleanup_assessment_ids a ON a.id = s.assessment_id LEFT JOIN tmp_cleanup_testee_ids t ON t.id = s.testee_id WHERE a.id IS NOT NULL OR t.id IS NOT NULL`},
		{"assessment", `DELETE a FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id = a.id`},
		{"testee", `DELETE t FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id = t.id`},
	}
	out := make([]namedCount, 0, len(items))
	for _, item := range items {
		result, err := tx.ExecContext(ctx, item.stmt)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		n, _ := result.RowsAffected()
		out = append(out, namedCount{Name: item.name, Count: n})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func deleteMongoRows(ctx context.Context, db *mongo.Database, ids scopeIDs) ([]namedCount, error) {
	items := []struct {
		name   string
		coll   string
		filter bson.M
	}{
		{"answersheets", "answersheets", answersheetFilter(ids)},
		{"answersheet_submit_idempotency", "answersheet_submit_idempotency", answerSheetIdempotencyFilter(ids)},
		{"interpret_reports", "interpret_reports", reportFilter(ids)},
		{"domain_event_outbox", "domain_event_outbox", mongoOutboxFilter(ids)},
	}
	out := make([]namedCount, 0, len(items))
	for _, item := range items {
		result, err := db.Collection(item.coll).DeleteMany(ctx, item.filter)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		out = append(out, namedCount{Name: item.name, Count: result.DeletedCount})
	}
	return out, nil
}

func answersheetFilter(ids scopeIDs) bson.M {
	return orFilter(
		inUint64("testee_id", ids.TesteeIDs),
		inUint64("domain_id", ids.AnswerSheetIDs),
	)
}

func answerSheetIdempotencyFilter(ids scopeIDs) bson.M {
	return orFilter(
		inUint64("testee_id", ids.TesteeIDs),
		inUint64("answersheet_id", ids.AnswerSheetIDs),
	)
}

func reportFilter(ids scopeIDs) bson.M {
	return orFilter(
		inUint64("testee_id", ids.TesteeIDs),
		inUint64("domain_id", ids.ReportIDs),
		inUint64("domain_id", ids.AssessmentIDs),
	)
}

func mongoOutboxFilter(ids scopeIDs) bson.M {
	answerStrings := uint64Strings(ids.AnswerSheetIDs)
	assessmentStrings := uint64Strings(ids.AssessmentIDs)
	reportStrings := uint64Strings(ids.ReportIDs)
	testeeStrings := uint64Strings(ids.TesteeIDs)
	behaviorStrings := uniqueStrings(answerStrings, assessmentStrings, reportStrings, testeeStrings)
	return orFilter(
		inStringWithAggregate("AnswerSheet", answerStrings),
		inStringWithAggregate("Assessment", assessmentStrings),
		inStringWithAggregate("Report", reportStrings),
		inStringWithAggregate("BehaviorFootprint", behaviorStrings),
	)
}

func inUint64(field string, values []uint64) bson.M {
	if len(values) == 0 {
		return nil
	}
	return bson.M{field: bson.M{"$in": values}}
}

func inStringWithAggregate(aggregateType string, values []string) bson.M {
	if len(values) == 0 {
		return nil
	}
	return bson.M{"aggregate_type": aggregateType, "aggregate_id": bson.M{"$in": values}}
}

func orFilter(filters ...bson.M) bson.M {
	ors := make([]bson.M, 0, len(filters))
	for _, filter := range filters {
		if len(filter) > 0 {
			ors = append(ors, filter)
		}
	}
	if len(ors) == 0 {
		return bson.M{"_id": bson.M{"$exists": false}}
	}
	if len(ors) == 1 {
		return ors[0]
	}
	return bson.M{"$or": ors}
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

func firstSQLLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return line[:120] + "..."
			}
			return line
		}
	}
	return ""
}
