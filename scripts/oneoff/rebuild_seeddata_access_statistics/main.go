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

	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	driverMysql "github.com/go-sql-driver/mysql"
	redis "github.com/redis/go-redis/v9"
	gormMysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	scopeTableName          = "oneoff_seeddata_access_scope"
	eventIDPrefix           = "oneoff:seeddata_access:"
	inferredResolveIDOffset = uint64(8000000000000000000)
)

type config struct {
	mysqlDSN              string
	orgID                 int64
	allOrgs               bool
	startDateRaw          string
	endDateRaw            string
	timeout               time.Duration
	apply                 bool
	skipBackup            bool
	backupSuffix          string
	testeeSourceRaw       string
	inferredTesteeCreated bool
	previewLimit          int
	skipIntake            bool
	skipResolve           bool
	skipFootprint         bool
	skipAggregate         bool
	skipCache             bool
	redisAddr             string
	redisQueryAddr        string
	redisMetaAddr         string
	redisUsername         string
	redisQueryUsername    string
	redisMetaUsername     string
	redisPassword         string
	redisQueryDB          int
	redisMetaDB           int
	redisQueryNS          string
	maxQuestionnaires     int
	maxPlans              int
	questionnaireCodes    csvFlag
	planIDs               uint64CSVFlag
}

type csvFlag []string

func (f *csvFlag) String() string { return strings.Join(*f, ",") }

func (f *csvFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			*f = append(*f, item)
		}
	}
	return nil
}

type uint64CSVFlag []uint64

func (f *uint64CSVFlag) String() string {
	values := make([]string, 0, len(*f))
	for _, item := range *f {
		values = append(values, strconv.FormatUint(item, 10))
	}
	return strings.Join(values, ",")
}

func (f *uint64CSVFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parsed, err := strconv.ParseUint(item, 10, 64)
		if err != nil || parsed == 0 {
			return fmt.Errorf("invalid plan id %q", item)
		}
		*f = append(*f, parsed)
	}
	return nil
}

type statementResult struct {
	Name     string
	Affected int64
}

type statementSpec struct {
	Name  string
	Query string
	Args  []any
}

type rebuildScopeSummary struct {
	SourceKind        string
	Rows              int64
	TesteeCreated     int64
	AssignmentCreated int64
}

type warmScope struct {
	OrgID              int64
	QuestionnaireCodes []string
	PlanIDs            []uint64
}

func main() {
	cfg := parseFlags()
	startDate := mustParseDate("start-date", cfg.startDateRaw)
	endDate := parseOptionalDate("end-date", cfg.endDateRaw)
	if endDate == nil {
		tomorrow := beginningOfDay(time.Now()).AddDate(0, 0, 1)
		endDate = &tomorrow
	}
	if !startDate.Before(*endDate) {
		log.Fatal("--start-date must be before --end-date")
	}

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

	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("open mysql connection: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close mysql connection: %v", err)
		}
	}()

	orgIDs, err := resolveOrgIDs(ctx, db, cfg)
	if err != nil {
		log.Fatalf("resolve org ids: %v", err)
	}
	if len(orgIDs) == 0 {
		log.Print("scope is empty; nothing to rebuild")
		return
	}

	if err := printPreview(ctx, conn, cfg, startDate, *endDate, orgIDs); err != nil {
		log.Fatalf("preview: %v", err)
	}
	if !cfg.apply {
		log.Printf("dry-run only; re-run with --apply to execute %s", phaseDescription(cfg))
		return
	}

	if !cfg.skipBackup {
		if cfg.backupSuffix == "" {
			cfg.backupSuffix = time.Now().Format("20060102_150405")
		}
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
		if err := createBackups(ctx, conn, cfg, startDate, *endDate); err != nil {
			log.Fatalf("create backups: %v", err)
		}
	}

	if !cfg.skipIntake {
		if err := prepareScope(ctx, conn, cfg, startDate, *endDate); err != nil {
			log.Fatalf("prepare intake scope: %v", err)
		}
		results, err := applyIntakeRebuild(ctx, conn, cfg, startDate, *endDate)
		if err != nil {
			log.Fatalf("apply intake rebuild: %v", err)
		}
		for _, item := range results {
			log.Printf("applied %-42s affected_rows=%d", item.Name, item.Affected)
		}
	}

	if !cfg.skipResolve {
		results, err := applyResolveBackfill(ctx, conn, cfg, startDate, *endDate)
		if err != nil {
			log.Fatalf("apply resolve backfill: %v", err)
		}
		for _, item := range results {
			log.Printf("applied %-42s affected_rows=%d", item.Name, item.Affected)
		}
	}

	if !cfg.skipFootprint {
		results, err := applyFootprintBackfill(ctx, conn, cfg, startDate, *endDate)
		if err != nil {
			log.Fatalf("apply footprint backfill: %v", err)
		}
		for _, item := range results {
			log.Printf("applied %-42s affected_rows=%d", item.Name, item.Affected)
		}
	}

	if !cfg.skipAggregate {
		gormDB, sqlDB, err := openGorm(cfg.mysqlDSN)
		if err != nil {
			log.Fatalf("open gorm: %v", err)
		}
		defer func() {
			if err := sqlDB.Close(); err != nil {
				log.Printf("close gorm sql: %v", err)
			}
		}()
		if err := rebuildAggregates(ctx, gormDB, orgIDs, startDate, *endDate); err != nil {
			log.Fatalf("rebuild aggregates: %v", err)
		}
	}

	if !cfg.skipCache {
		if !cfg.redisEnabled() {
			log.Print("redis is not configured; skip statistics query cache rebuild")
		} else {
			gormDB, sqlDB, err := openGorm(cfg.mysqlDSN)
			if err != nil {
				log.Fatalf("open gorm for cache: %v", err)
			}
			warmScopes, err := resolveWarmScopes(ctx, sqlDB, cfg, orgIDs, startDate, *endDate)
			if err != nil {
				_ = sqlDB.Close()
				log.Fatalf("resolve cache warm scopes: %v", err)
			}
			if err := rebuildCache(ctx, gormDB, cfg, warmScopes); err != nil {
				_ = sqlDB.Close()
				log.Fatalf("rebuild cache: %v", err)
			}
			if err := sqlDB.Close(); err != nil {
				log.Printf("close gorm sql: %v", err)
			}
		}
	}

	log.Print("seeddata access statistics rebuild completed")
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to rebuild; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "rebuild all organizations")
	flag.StringVar(&cfg.startDateRaw, "start-date", "2025-01-01", "inclusive lower bound, format YYYY-MM-DD")
	flag.StringVar(&cfg.endDateRaw, "end-date", "", "exclusive upper bound, format YYYY-MM-DD; default is tomorrow")
	flag.DurationVar(&cfg.timeout, "timeout", 4*time.Hour, "overall script timeout, e.g. 30m, 4h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipBackup, "skip-backup", false, "skip backup tables; only use when an external backup already exists")
	flag.StringVar(&cfg.backupSuffix, "backup-suffix", "", "backup table suffix, letters/digits/underscore only; default is current timestamp")
	flag.StringVar(&cfg.testeeSourceRaw, "testee-source", "daily_simulation", "comma-separated testee.source values for inferring missing intake logs; empty means no source filter")
	flag.BoolVar(&cfg.inferredTesteeCreated, "inferred-testee-created", true, "mark inferred intake rows as testee_created=1")
	flag.IntVar(&cfg.previewLimit, "preview-limit", 30, "maximum daily preview rows to print")
	flag.BoolVar(&cfg.skipIntake, "skip-intake", false, "skip assessment_entry_intake_log rebuild")
	flag.BoolVar(&cfg.skipResolve, "skip-resolve", false, "skip assessment_entry_resolve_log backfill")
	flag.BoolVar(&cfg.skipFootprint, "skip-footprint", false, "skip behavior_footprint backfill for access events")
	flag.BoolVar(&cfg.skipAggregate, "skip-aggregate", false, "skip statistics_journey_daily rebuild")
	flag.BoolVar(&cfg.skipCache, "skip-cache", true, "skip Redis query cache rebuild")
	flag.StringVar(&cfg.redisAddr, "redis-addr", "", "Redis address used for both query and meta cache, e.g. 127.0.0.1:6379")
	flag.StringVar(&cfg.redisQueryAddr, "redis-query-addr", "", "Redis query cache address; defaults to --redis-addr")
	flag.StringVar(&cfg.redisMetaAddr, "redis-meta-addr", "", "Redis meta/version cache address; defaults to --redis-addr")
	flag.StringVar(&cfg.redisUsername, "redis-username", "", "Redis ACL username used for both query and meta cache")
	flag.StringVar(&cfg.redisQueryUsername, "redis-query-username", "", "Redis query cache ACL username; defaults to --redis-username")
	flag.StringVar(&cfg.redisMetaUsername, "redis-meta-username", "", "Redis meta/version cache ACL username; defaults to --redis-username")
	flag.StringVar(&cfg.redisPassword, "redis-password", "", "Redis password")
	flag.IntVar(&cfg.redisQueryDB, "redis-query-db", 0, "Redis DB for query cache")
	flag.IntVar(&cfg.redisMetaDB, "redis-meta-db", 0, "Redis DB for meta/version cache")
	flag.StringVar(&cfg.redisQueryNS, "redis-query-namespace", "", "query cache key namespace, e.g. qs:cache:query")
	flag.StringVar(&cfg.redisQueryNS, "redis-namespace", "", "alias of --redis-query-namespace")
	flag.IntVar(&cfg.maxQuestionnaires, "max-questionnaires", 0, "maximum questionnaire codes to warm per org; 0 means no limit")
	flag.IntVar(&cfg.maxPlans, "max-plans", 0, "maximum plan IDs to warm per org; 0 means no limit")
	flag.Var(&cfg.questionnaireCodes, "questionnaire-code", "questionnaire code to warm; repeat or comma-separate. Empty means discover from assessment")
	flag.Var(&cfg.planIDs, "plan-id", "plan ID to warm; repeat or comma-separate. Empty means discover from assessment_plan/task")
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
	if cfg.previewLimit < 0 {
		log.Fatal("--preview-limit must be >= 0")
	}
	if cfg.backupSuffix != "" {
		if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
			log.Fatalf("invalid --backup-suffix: %v", err)
		}
	}
	if cfg.redisQueryAddr == "" {
		cfg.redisQueryAddr = cfg.redisAddr
	}
	if cfg.redisMetaAddr == "" {
		cfg.redisMetaAddr = cfg.redisAddr
	}
	if cfg.redisQueryUsername == "" {
		cfg.redisQueryUsername = cfg.redisUsername
	}
	if cfg.redisMetaUsername == "" {
		cfg.redisMetaUsername = cfg.redisUsername
	}
	return cfg
}

func (cfg config) redisEnabled() bool {
	return strings.TrimSpace(cfg.redisQueryAddr) != "" && strings.TrimSpace(cfg.redisMetaAddr) != ""
}

func phaseDescription(cfg config) string {
	phases := make([]string, 0, 5)
	if !cfg.skipIntake {
		phases = append(phases, "intake logs")
	}
	if !cfg.skipResolve {
		phases = append(phases, "resolve logs")
	}
	if !cfg.skipFootprint {
		phases = append(phases, "behavior_footprint")
	}
	if !cfg.skipAggregate {
		phases = append(phases, "statistics aggregates")
	}
	if !cfg.skipCache && cfg.redisEnabled() {
		phases = append(phases, "Redis cache")
	}
	if len(phases) == 0 {
		return "nothing"
	}
	return strings.Join(phases, ", ")
}

func resolveOrgIDs(ctx context.Context, db *sql.DB, cfg config) ([]int64, error) {
	if !cfg.allOrgs {
		return []int64{cfg.orgID}, nil
	}
	rows, err := db.QueryContext(ctx, `
SELECT DISTINCT org_id
FROM clinician_relation
WHERE deleted_at IS NULL
ORDER BY org_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func printPreview(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time, orgIDs []int64) error {
	log.Printf("scope: orgs=%v %s start=%s end=%s apply=%v phases=%s testee_sources=%q",
		orgIDs, scopeDescription(cfg), formatDay(startDate), formatDay(endDate), cfg.apply, phaseDescription(cfg), strings.TrimSpace(cfg.testeeSourceRaw))

	if !cfg.skipIntake {
		if err := prepareScope(ctx, conn, cfg, startDate, endDate); err != nil {
			return fmt.Errorf("prepare intake scope: %w", err)
		}
		summaries, err := loadScopeSummaries(ctx, conn)
		if err != nil {
			return err
		}
		var totalRows int64
		for _, item := range summaries {
			totalRows += item.Rows
			log.Printf("candidate intake %-10s rows=%d testee_created=%d assignment_created=%d",
				item.SourceKind, item.Rows, item.TesteeCreated, item.AssignmentCreated)
		}
		log.Printf("candidate intake %-10s rows=%d", "total", totalRows)
		if cfg.previewLimit > 0 {
			rows, err := loadDailyPreview(ctx, conn, cfg.previewLimit)
			if err != nil {
				return err
			}
			for _, item := range rows {
				log.Printf("preview intake day=%s kind=%s rows=%d", item.StatDate, item.SourceKind, item.Rows)
			}
		}
	}

	if !cfg.skipResolve {
		count, err := countMissingResolveLogs(ctx, conn, cfg, startDate, endDate)
		if err != nil {
			return err
		}
		log.Printf("candidate resolve_logs_to_infer=%d", count)
	}

	if !cfg.skipFootprint {
		counts, err := previewFootprintCounts(ctx, conn, cfg, startDate, endDate)
		if err != nil {
			return err
		}
		for _, item := range counts {
			log.Printf("candidate footprint %-28s %d", item.Name, item.Affected)
		}
	}

	if !cfg.skipAggregate {
		count, err := countJourneyRows(ctx, conn, cfg, startDate, endDate)
		if err != nil {
			return err
		}
		log.Printf("aggregate journey_rows_to_reset=%d", count)
	}

	if !cfg.skipCache && cfg.redisEnabled() {
		queryPattern, versionPattern := cachePatterns(cfg.redisQueryNS)
		log.Printf("cache patterns: query=%q version=%q", queryPattern, versionPattern)
	}
	return nil
}

func prepareScope(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) error {
	if _, err := conn.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS "+scopeTableName); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TEMPORARY TABLE `+scopeTableName+` (
  source_kind VARCHAR(16) NOT NULL,
  existing_id BIGINT UNSIGNED NULL,
  org_id BIGINT NOT NULL,
  clinician_id BIGINT UNSIGNED NOT NULL,
  entry_id BIGINT UNSIGNED NOT NULL,
  testee_id BIGINT UNSIGNED NOT NULL,
  testee_created TINYINT(1) NOT NULL DEFAULT 0,
  assignment_created TINYINT(1) NOT NULL DEFAULT 0,
  intake_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_scope_kind (source_kind),
  KEY idx_scope_org_time (org_id, intake_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, buildExistingIntakeScopeInsert(cfg), buildExistingIntakeScopeArgs(cfg, startDate, endDate)...); err != nil {
		return fmt.Errorf("load existing intake logs: %w", err)
	}
	if _, err := conn.ExecContext(ctx, buildInferredIntakeScopeInsert(cfg), buildInferredIntakeScopeArgs(cfg, startDate, endDate)...); err != nil {
		return fmt.Errorf("load inferred assessment_entry intake logs: %w", err)
	}
	if _, err := conn.ExecContext(ctx, buildInferredManualRelationScopeInsert(cfg), buildInferredManualRelationScopeArgs(cfg, startDate, endDate)...); err != nil {
		return fmt.Errorf("load inferred manual relation intake logs: %w", err)
	}
	return nil
}

func applyIntakeRebuild(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) ([]statementResult, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	results := make([]statementResult, 0, 3)
	for _, stmt := range []statementSpec{
		buildDeleteIntakeLogs(cfg, startDate, endDate),
		{Name: "restore_existing_intake_logs", Query: restoreExistingIntakeLogsSQL},
		{Name: "insert_inferred_intake_logs", Query: insertInferredIntakeLogsSQL},
	} {
		res, err := tx.ExecContext(ctx, stmt.Query, stmt.Args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.Name, err)
		}
		affected, _ := res.RowsAffected()
		results = append(results, statementResult{Name: stmt.Name, Affected: affected})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func applyResolveBackfill(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) ([]statementResult, error) {
	stmt := buildInsertInferredResolveLogs(cfg, startDate, endDate)
	res, err := conn.ExecContext(ctx, stmt.Query, stmt.Args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", stmt.Name, err)
	}
	affected, _ := res.RowsAffected()
	return []statementResult{{Name: stmt.Name, Affected: affected}}, nil
}

func applyFootprintBackfill(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) ([]statementResult, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	results := make([]statementResult, 0, 6)
	for _, stmt := range []statementSpec{
		buildEntryOpenedFootprintInsert(cfg, startDate, endDate),
		buildIntakeConfirmedFootprintInsert(cfg, startDate, endDate),
		buildManualRelationEntryOpenedFootprintInsert(cfg, startDate, endDate),
		buildManualRelationIntakeConfirmedFootprintInsert(cfg, startDate, endDate),
		buildTesteeProfileCreatedFootprintInsert(cfg, startDate, endDate),
		buildCareRelationshipEstablishedFootprintInsert(cfg, startDate, endDate),
	} {
		res, err := tx.ExecContext(ctx, stmt.Query, stmt.Args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.Name, err)
		}
		affected, _ := res.RowsAffected()
		results = append(results, statementResult{Name: stmt.Name, Affected: affected})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func createBackups(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) error {
	tables := []struct {
		backupName  string
		sourceTable string
		alias       string
		whereSQL    string
		args        []any
	}{
		{
			backupName:  backupTableName("oneoff_bak_seeddata_intake_log", cfg.backupSuffix),
			sourceTable: "assessment_entry_intake_log",
			alias:       "l",
			whereSQL:    "l.deleted_at IS NULL AND l.intake_at >= ? AND l.intake_at < ?",
			args:        []any{startDate, endDate},
		},
		{
			backupName:  backupTableName("oneoff_bak_seeddata_resolve_log", cfg.backupSuffix),
			sourceTable: "assessment_entry_resolve_log",
			alias:       "l",
			whereSQL:    "l.deleted_at IS NULL AND l.resolved_at >= ? AND l.resolved_at < ?",
			args:        []any{startDate, endDate},
		},
		{
			backupName:  backupTableName("oneoff_bak_seeddata_footprint", cfg.backupSuffix),
			sourceTable: "behavior_footprint",
			alias:       "f",
			whereSQL: `f.deleted_at IS NULL
  AND f.occurred_at >= ? AND f.occurred_at < ?
  AND f.event_name IN ('entry_opened', 'intake_confirmed', 'testee_profile_created', 'care_relationship_established')`,
			args: []any{startDate, endDate},
		},
		{
			backupName:  backupTableName("oneoff_bak_seeddata_journey_daily", cfg.backupSuffix),
			sourceTable: "statistics_journey_daily",
			alias:       "d",
			whereSQL:    "d.deleted_at IS NULL AND d.stat_date >= ? AND d.stat_date < ?",
			args:        []any{startDate, endDate},
		},
	}
	for _, item := range tables {
		if _, err := conn.ExecContext(ctx, "CREATE TABLE "+item.backupName+" LIKE "+item.sourceTable); err != nil {
			return fmt.Errorf("create %s: %w", item.backupName, err)
		}
		query := "INSERT INTO " + item.backupName + " SELECT " + item.alias + ".* FROM " + item.sourceTable + " " + item.alias + " WHERE " + item.whereSQL
		args := item.args
		query, args = appendOrgPredicate(query, args, cfg, item.alias)
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("backup %s: %w", item.backupName, err)
		}
		log.Printf("backup table created: %s", item.backupName)
	}
	return nil
}

func buildInsertInferredResolveLogs(cfg config, startDate, endDate time.Time) statementSpec {
	offset := strconv.FormatUint(inferredResolveIDOffset, 10)
	query := `
INSERT INTO assessment_entry_resolve_log (
  id, org_id, clinician_id, entry_id, resolved_at, created_at, updated_at
)
SELECT
  (` + offset + ` + l.id),
  l.org_id,
  l.clinician_id,
  l.entry_id,
  l.intake_at,
  l.created_at,
  l.updated_at
FROM assessment_entry_intake_log l
LEFT JOIN assessment_entry_resolve_log r
  ON r.org_id = l.org_id
 AND r.entry_id = l.entry_id
 AND r.clinician_id = l.clinician_id
 AND r.deleted_at IS NULL
 AND r.resolved_at >= DATE_SUB(l.intake_at, INTERVAL 1 DAY)
 AND r.resolved_at <= l.intake_at
WHERE l.deleted_at IS NULL
  AND l.intake_at >= ?
  AND l.intake_at < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "l")
	query += `
ON DUPLICATE KEY UPDATE
  org_id = VALUES(org_id),
  clinician_id = VALUES(clinician_id),
  entry_id = VALUES(entry_id),
  resolved_at = VALUES(resolved_at),
  updated_at = VALUES(updated_at),
  deleted_at = NULL`
	return statementSpec{Name: "insert_inferred_resolve_logs", Query: query, Args: args}
}

func buildEntryOpenedFootprintInsert(cfg config, startDate, endDate time.Time) statementSpec {
	query := `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('` + eventIDPrefix + `entry_opened:', l.id),
  l.org_id, 'assessment_entry', l.entry_id, 'assessment_entry', l.entry_id,
  l.entry_id, l.clinician_id, 0, 0,
  0, 0, 0, 'entry_opened', l.resolved_at,
  JSON_OBJECT('legacy_source', 'assessment_entry_resolve_log', 'legacy_id', l.id, 'rebuilt_by', 'rebuild_seeddata_access_statistics')
FROM assessment_entry_resolve_log l
WHERE l.deleted_at IS NULL
  AND l.resolved_at >= ?
  AND l.resolved_at < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "l")
	query += behaviorFootprintUpsertSQL()
	return statementSpec{Name: "insert_entry_opened_footprint", Query: query, Args: args}
}

func buildIntakeConfirmedFootprintInsert(cfg config, startDate, endDate time.Time) statementSpec {
	return buildIntakeFootprintInsert(cfg, startDate, endDate, "intake_confirmed", "intake_confirmed", "1=1")
}

func buildTesteeProfileCreatedFootprintInsert(cfg config, startDate, endDate time.Time) statementSpec {
	return buildIntakeFootprintInsert(cfg, startDate, endDate, "testee_profile_created", "testee_profile_created", "l.testee_created = 1")
}

func buildCareRelationshipEstablishedFootprintInsert(cfg config, startDate, endDate time.Time) statementSpec {
	return buildIntakeFootprintInsert(cfg, startDate, endDate, "care_relationship_established", "care_relationship_established", "l.assignment_created = 1")
}

func buildManualRelationIntakeConfirmedFootprintInsert(cfg config, startDate, endDate time.Time) statementSpec {
	return buildManualRelationFootprintInsert(cfg, startDate, endDate, "manual_intake_confirmed", "intake_confirmed")
}

func buildManualRelationEntryOpenedFootprintInsert(cfg config, startDate, endDate time.Time) statementSpec {
	return buildManualRelationFootprintInsert(cfg, startDate, endDate, "manual_entry_opened", "entry_opened")
}

func buildManualRelationFootprintInsert(cfg config, startDate, endDate time.Time, idPart, eventName string) statementSpec {
	query := `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('` + eventIDPrefix + idPart + `:', cr.id),
  cr.org_id,
  CASE WHEN '` + eventName + `' = 'entry_opened' THEN 'assessment_entry' ELSE 'testee' END,
  CASE WHEN '` + eventName + `' = 'entry_opened' THEN ae.entry_id ELSE cr.testee_id END,
  CASE WHEN '` + eventName + `' = 'entry_opened' THEN 'assessment_entry' ELSE 'clinician' END,
  CASE WHEN '` + eventName + `' = 'entry_opened' THEN ae.entry_id ELSE cr.clinician_id END,
  ae.entry_id,
  cr.clinician_id,
  0,
  CASE WHEN '` + eventName + `' = 'entry_opened' THEN 0 ELSE cr.testee_id END,
  0, 0, 0,
  '` + eventName + `',
  cr.bound_at,
  JSON_OBJECT(
    'source_table', 'clinician_relation',
    'source_id', cr.id,
    'relation_type', cr.relation_type,
    'source_type', cr.source_type,
    'rebuilt_by', 'rebuild_seeddata_access_statistics'
  )
FROM clinician_relation cr
INNER JOIN (
  SELECT org_id, clinician_id, MIN(id) AS entry_id
  FROM assessment_entry
  WHERE deleted_at IS NULL
    AND is_active = 1
    AND (expires_at IS NULL OR expires_at > NOW(3))
  GROUP BY org_id, clinician_id
) ae
  ON ae.org_id = cr.org_id
 AND ae.clinician_id = cr.clinician_id
WHERE cr.deleted_at IS NULL
  AND cr.source_type IN ('manual', 'import')
  AND cr.relation_type IN ('primary', 'attending', 'collaborator', 'assigned')
  AND cr.bound_at >= ?
  AND cr.bound_at < ?
  AND NOT EXISTS (
    SELECT 1
    FROM behavior_footprint f
    WHERE f.org_id = cr.org_id
      AND f.clinician_id = cr.clinician_id
      AND f.testee_id = cr.testee_id
      AND f.event_name = '` + eventName + `'
      AND f.deleted_at IS NULL
      AND f.occurred_at >= DATE_SUB(cr.bound_at, INTERVAL 1 DAY)
      AND f.occurred_at <= DATE_ADD(cr.bound_at, INTERVAL 1 DAY)
  )`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "cr")
	query += behaviorFootprintUpsertSQL()
	return statementSpec{Name: "insert_" + idPart + "_footprint", Query: query, Args: args}
}

func buildIntakeFootprintInsert(cfg config, startDate, endDate time.Time, idPart, eventName, extraWhere string) statementSpec {
	query := `
INSERT INTO behavior_footprint (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at, properties_json
)
SELECT
  CONCAT('` + eventIDPrefix + idPart + `:', l.id),
  l.org_id, 'testee', l.testee_id, 'clinician', l.clinician_id,
  l.entry_id, l.clinician_id, 0, l.testee_id,
  0, 0, 0, '` + eventName + `', l.intake_at,
  JSON_OBJECT('legacy_source', 'assessment_entry_intake_log', 'legacy_id', l.id, 'rebuilt_by', 'rebuild_seeddata_access_statistics')
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND ` + extraWhere + `
  AND l.intake_at >= ?
  AND l.intake_at < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "l")
	query += behaviorFootprintUpsertSQL()
	return statementSpec{Name: "insert_" + idPart + "_footprint", Query: query, Args: args}
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

func countMissingResolveLogs(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) (int64, error) {
	query := `
SELECT COUNT(*)
FROM assessment_entry_intake_log l
LEFT JOIN assessment_entry_resolve_log r
  ON r.org_id = l.org_id
 AND r.entry_id = l.entry_id
 AND r.clinician_id = l.clinician_id
 AND r.deleted_at IS NULL
 AND r.resolved_at >= DATE_SUB(l.intake_at, INTERVAL 1 DAY)
 AND r.resolved_at <= l.intake_at
WHERE l.deleted_at IS NULL
  AND l.intake_at >= ?
  AND l.intake_at < ?
  AND r.id IS NULL`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "l")
	var count int64
	if err := conn.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func previewFootprintCounts(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) ([]statementResult, error) {
	queries := []statementSpec{
		buildEntryOpenedFootprintInsert(cfg, startDate, endDate),
		buildIntakeConfirmedFootprintInsert(cfg, startDate, endDate),
		buildManualRelationEntryOpenedFootprintInsert(cfg, startDate, endDate),
		buildManualRelationIntakeConfirmedFootprintInsert(cfg, startDate, endDate),
		buildTesteeProfileCreatedFootprintInsert(cfg, startDate, endDate),
		buildCareRelationshipEstablishedFootprintInsert(cfg, startDate, endDate),
	}
	results := make([]statementResult, 0, len(queries))
	for _, stmt := range queries {
		countSQL := footprintCountSQL(stmt.Query)
		var count int64
		if err := conn.QueryRowContext(ctx, countSQL, stmt.Args...).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.Name, err)
		}
		results = append(results, statementResult{Name: stmt.Name, Affected: count})
	}
	return results, nil
}

func footprintCountSQL(insertSQL string) string {
	selectIdx := strings.Index(insertSQL, "SELECT")
	fromIdx := strings.Index(insertSQL, "FROM")
	if selectIdx < 0 || fromIdx < 0 {
		return insertSQL
	}
	body := insertSQL[fromIdx:]
	for _, marker := range []string{"ON DUPLICATE KEY UPDATE"} {
		if idx := strings.Index(body, marker); idx >= 0 {
			body = body[:idx]
		}
	}
	return "SELECT COUNT(*) " + body
}

func countJourneyRows(ctx context.Context, conn *sql.Conn, cfg config, startDate, endDate time.Time) (int64, error) {
	query := `SELECT COUNT(*) FROM statistics_journey_daily d WHERE d.deleted_at IS NULL AND d.stat_date >= ? AND d.stat_date < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "d")
	var count int64
	if err := conn.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// --- intake scope helpers (from rebuild_access_funnel_from_sources) ---

func buildExistingIntakeScopeInsert(cfg config) string {
	query := `
INSERT INTO ` + scopeTableName + ` (
  source_kind, existing_id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  'existing',
  l.id,
  l.org_id,
  l.clinician_id,
  l.entry_id,
  l.testee_id,
  l.testee_created,
  l.assignment_created,
  l.intake_at,
  l.created_at,
  l.updated_at
FROM assessment_entry_intake_log l
WHERE l.deleted_at IS NULL
  AND l.intake_at >= ?
  AND l.intake_at < ?`
	query, _ = appendOrgPredicate(query, nil, cfg, "l")
	return query
}

func buildExistingIntakeScopeArgs(cfg config, startDate, endDate time.Time) []any {
	args := []any{startDate, endDate}
	_, args = appendOrgPredicate("", args, cfg, "l")
	return args
}

func buildInferredIntakeScopeInsert(cfg config) string {
	testeeSources := parseCSV(cfg.testeeSourceRaw)
	sourcePredicate := ""
	if len(testeeSources) > 0 {
		sourcePredicate = " AND t.source IN (" + placeholders(len(testeeSources)) + ")"
	}
	testeeCreatedValue := "0"
	if cfg.inferredTesteeCreated {
		testeeCreatedValue = "1"
	}
	query := `
INSERT INTO ` + scopeTableName + ` (
  source_kind, existing_id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  'inferred',
  NULL,
  cr.org_id,
  cr.clinician_id,
  cr.source_id AS entry_id,
  cr.testee_id,
  ` + testeeCreatedValue + ` AS testee_created,
  1 AS assignment_created,
  MIN(cr.bound_at) AS intake_at,
  MIN(cr.created_at) AS created_at,
  MAX(cr.updated_at) AS updated_at
FROM clinician_relation cr
INNER JOIN testee t
  ON t.id = cr.testee_id
 AND t.org_id = cr.org_id
 AND t.deleted_at IS NULL
INNER JOIN assessment_entry ae
  ON ae.id = cr.source_id
 AND ae.org_id = cr.org_id
 AND ae.clinician_id = cr.clinician_id
 AND ae.deleted_at IS NULL
LEFT JOIN assessment_entry_intake_log l
  ON l.org_id = cr.org_id
 AND l.clinician_id = cr.clinician_id
 AND l.entry_id = cr.source_id
 AND l.testee_id = cr.testee_id
 AND l.deleted_at IS NULL
WHERE cr.deleted_at IS NULL
  AND cr.source_type = 'assessment_entry'
  AND cr.source_id IS NOT NULL
  AND cr.relation_type IN ('assigned', 'primary', 'attending', 'collaborator')
  AND cr.bound_at >= ?
  AND cr.bound_at < ?
  AND l.id IS NULL` + sourcePredicate
	query, _ = appendOrgPredicate(query, nil, cfg, "cr")
	query += `
GROUP BY cr.org_id, cr.clinician_id, cr.source_id, cr.testee_id`
	return query
}

func buildInferredIntakeScopeArgs(cfg config, startDate, endDate time.Time) []any {
	args := []any{startDate, endDate}
	for _, item := range parseCSV(cfg.testeeSourceRaw) {
		args = append(args, item)
	}
	_, args = appendOrgPredicate("", args, cfg, "cr")
	return args
}

func buildInferredManualRelationScopeInsert(cfg config) string {
	// manual/import 后台直挂是明确修复目标，不按 testee.source 过滤。
	testeeCreatedValue := "CASE WHEN ABS(TIMESTAMPDIFF(SECOND, t.created_at, cr.bound_at)) <= 5 THEN 1 ELSE 0 END"
	if cfg.inferredTesteeCreated {
		testeeCreatedValue = "1"
	}

	query := `
INSERT INTO ` + scopeTableName + ` (
  source_kind, existing_id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  'inferred_manual',
  NULL,
  cr.org_id,
  cr.clinician_id,
  ae.entry_id,
  cr.testee_id,
  ` + testeeCreatedValue + ` AS testee_created,
  1 AS assignment_created,
  cr.bound_at AS intake_at,
  cr.created_at,
  cr.updated_at
FROM clinician_relation cr
INNER JOIN testee t
  ON t.id = cr.testee_id
 AND t.org_id = cr.org_id
 AND t.deleted_at IS NULL
INNER JOIN (
  SELECT org_id, clinician_id, MIN(id) AS entry_id
  FROM assessment_entry
  WHERE deleted_at IS NULL
    AND is_active = 1
    AND (expires_at IS NULL OR expires_at > NOW(3))
  GROUP BY org_id, clinician_id
) ae
  ON ae.org_id = cr.org_id
 AND ae.clinician_id = cr.clinician_id
LEFT JOIN assessment_entry_intake_log l
  ON l.org_id = cr.org_id
 AND l.clinician_id = cr.clinician_id
 AND l.testee_id = cr.testee_id
 AND l.deleted_at IS NULL
 AND ABS(TIMESTAMPDIFF(SECOND, l.intake_at, cr.bound_at)) <= 5
WHERE cr.deleted_at IS NULL
  AND cr.source_type IN ('manual', 'import')
  AND cr.relation_type IN ('primary', 'attending', 'collaborator', 'assigned')
  AND cr.bound_at >= ?
  AND cr.bound_at < ?
  AND l.id IS NULL`
	query, _ = appendOrgPredicate(query, nil, cfg, "cr")
	return query
}

func buildInferredManualRelationScopeArgs(cfg config, startDate, endDate time.Time) []any {
	args := []any{startDate, endDate}
	_, args = appendOrgPredicate("", args, cfg, "cr")
	return args
}

func buildDeleteIntakeLogs(cfg config, startDate, endDate time.Time) statementSpec {
	query := `DELETE FROM assessment_entry_intake_log WHERE deleted_at IS NULL AND intake_at >= ? AND intake_at < ?`
	args := []any{startDate, endDate}
	query, args = appendOrgPredicate(query, args, cfg, "")
	return statementSpec{Name: "delete_window_intake_logs", Query: query, Args: args}
}

const restoreExistingIntakeLogsSQL = `
INSERT INTO assessment_entry_intake_log (
  id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  existing_id, org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
FROM ` + scopeTableName + `
WHERE source_kind = 'existing'
ORDER BY intake_at, existing_id`

const insertInferredIntakeLogsSQL = `
INSERT INTO assessment_entry_intake_log (
  org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
)
SELECT
  org_id, clinician_id, entry_id, testee_id,
  testee_created, assignment_created, intake_at, created_at, updated_at
FROM ` + scopeTableName + `
WHERE source_kind IN ('inferred', 'inferred_manual')
ORDER BY intake_at, org_id, clinician_id, entry_id, testee_id`

func loadScopeSummaries(ctx context.Context, conn *sql.Conn) ([]rebuildScopeSummary, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT source_kind, COUNT(*), COALESCE(SUM(testee_created), 0), COALESCE(SUM(assignment_created), 0)
FROM `+scopeTableName+`
GROUP BY source_kind
ORDER BY source_kind`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []rebuildScopeSummary
	for rows.Next() {
		var item rebuildScopeSummary
		if err := rows.Scan(&item.SourceKind, &item.Rows, &item.TesteeCreated, &item.AssignmentCreated); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type dailyPreviewRow struct {
	StatDate   string
	SourceKind string
	Rows       int64
}

func loadDailyPreview(ctx context.Context, conn *sql.Conn, limit int) ([]dailyPreviewRow, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT DATE(intake_at) AS stat_date, source_kind, COUNT(*)
FROM `+scopeTableName+`
GROUP BY DATE(intake_at), source_kind
ORDER BY stat_date, source_kind
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []dailyPreviewRow
	for rows.Next() {
		var item dailyPreviewRow
		if err := rows.Scan(&item.StatDate, &item.SourceKind, &item.Rows); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// --- aggregate + cache (from rebuild_statistics_aggregates_and_cache) ---

func rebuildAggregates(ctx context.Context, db *gorm.DB, orgIDs []int64, startDate, endDate time.Time) error {
	repo := statisticsInfra.NewStatisticsRepository(db)
	for _, orgID := range orgIDs {
		log.Printf("rebuild aggregate org_id=%d daily/journey window", orgID)
		if err := withinTx(ctx, db, func(txCtx context.Context) error {
			return repo.RebuildDailyStatistics(txCtx, orgID, startDate, endDate)
		}); err != nil {
			return fmt.Errorf("org %d daily: %w", orgID, err)
		}
		log.Printf("rebuild aggregate org_id=%d org snapshot", orgID)
		if err := withinTx(ctx, db, func(txCtx context.Context) error {
			return repo.RebuildOrgSnapshotStatistics(txCtx, orgID, time.Now())
		}); err != nil {
			return fmt.Errorf("org %d snapshot: %w", orgID, err)
		}
	}
	return nil
}

func withinTx(ctx context.Context, db *gorm.DB, fn func(context.Context) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(gormuow.WithTx(ctx, tx))
	})
}

func rebuildCache(ctx context.Context, db *gorm.DB, cfg config, scopes []warmScope) error {
	queryClient := newRedisClient(cfg.redisQueryAddr, cfg.redisQueryUsername, cfg.redisPassword, cfg.redisQueryDB)
	defer func() {
		if err := queryClient.Close(); err != nil {
			log.Printf("close query redis: %v", err)
		}
	}()
	metaClient := newRedisClient(cfg.redisMetaAddr, cfg.redisMetaUsername, cfg.redisPassword, cfg.redisMetaDB)
	defer func() {
		if err := metaClient.Close(); err != nil {
			log.Printf("close meta redis: %v", err)
		}
	}()
	if err := queryClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping query redis: %w", err)
	}
	if err := metaClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping meta redis: %w", err)
	}

	queryPattern, versionPattern := cachePatterns(cfg.redisQueryNS)
	queryDeleted, err := deleteRedisPattern(ctx, queryClient, queryPattern)
	if err != nil {
		return fmt.Errorf("delete query cache pattern %q: %w", queryPattern, err)
	}
	versionDeleted, err := deleteRedisPattern(ctx, metaClient, versionPattern)
	if err != nil {
		return fmt.Errorf("delete version cache pattern %q: %w", versionPattern, err)
	}
	log.Printf("cleared statistics query cache: query_keys=%d version_keys=%d", queryDeleted, versionDeleted)

	cache := statisticsCache.NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
		queryClient,
		keyspace.NewBuilderWithNamespace(cfg.redisQueryNS),
		cachepolicy.CachePolicy{},
		cachequery.NewRedisVersionTokenStoreWithKind(metaClient, string(cachepolicy.PolicyStatsQuery)),
		nil,
	)
	repo := statisticsInfra.NewStatisticsRepository(db)
	readService := statisticsApp.NewReadService(
		statisticsReadModelInfra.NewReadModel(db),
		nil,
		statisticsApp.WithReadServiceCache(cache),
	)
	systemService := statisticsApp.NewSystemStatisticsService(repo, repo, cache, nil)
	questionnaireService := statisticsApp.NewQuestionnaireStatisticsService(repo, repo, cache, nil)
	planService := statisticsApp.NewPlanStatisticsService(repo, repo, cache, nil)

	for _, scope := range scopes {
		log.Printf("warm statistics cache org_id=%d overview/system", scope.OrgID)
		for _, preset := range []string{"today", "7d", "30d"} {
			if _, err := readService.GetOverview(ctx, scope.OrgID, statisticsApp.QueryFilter{Preset: preset}); err != nil {
				return fmt.Errorf("warm overview org=%d preset=%s: %w", scope.OrgID, preset, err)
			}
		}
		if _, err := systemService.GetSystemStatistics(ctx, scope.OrgID); err != nil {
			return fmt.Errorf("warm system org=%d: %w", scope.OrgID, err)
		}
		for _, code := range scope.QuestionnaireCodes {
			if _, err := questionnaireService.GetQuestionnaireStatistics(ctx, scope.OrgID, code); err != nil {
				return fmt.Errorf("warm questionnaire org=%d code=%s: %w", scope.OrgID, code, err)
			}
		}
		for _, planID := range scope.PlanIDs {
			if _, err := planService.GetPlanStatistics(ctx, scope.OrgID, planID); err != nil {
				return fmt.Errorf("warm plan org=%d plan=%d: %w", scope.OrgID, planID, err)
			}
		}
	}
	return nil
}

func resolveWarmScopes(ctx context.Context, db *sql.DB, cfg config, orgIDs []int64, startDate, endDate time.Time) ([]warmScope, error) {
	scopes := make([]warmScope, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		questionnaires, err := discoverQuestionnaireCodes(ctx, db, orgID, startDate, endDate, cfg.maxQuestionnaires, cfg.questionnaireCodes)
		if err != nil {
			return nil, err
		}
		plans, err := discoverPlanIDs(ctx, db, orgID, startDate, endDate, cfg.maxPlans, cfg.planIDs)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, warmScope{OrgID: orgID, QuestionnaireCodes: questionnaires, PlanIDs: plans})
	}
	return scopes, nil
}

func discoverQuestionnaireCodes(ctx context.Context, db *sql.DB, orgID int64, startDate, endDate time.Time, limit int, pinned []string) ([]string, error) {
	if len(pinned) > 0 {
		return pinned, nil
	}
	query := `
SELECT DISTINCT questionnaire_code
FROM assessment
WHERE org_id = ? AND deleted_at IS NULL
  AND created_at >= ? AND created_at < ?
ORDER BY questionnaire_code`
	if limit > 0 {
		query += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := db.QueryContext(ctx, query, orgID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

func discoverPlanIDs(ctx context.Context, db *sql.DB, orgID int64, startDate, endDate time.Time, limit int, pinned []uint64) ([]uint64, error) {
	if len(pinned) > 0 {
		return pinned, nil
	}
	query := `
SELECT DISTINCT p.id
FROM assessment_plan p
LEFT JOIN assessment_task t ON t.org_id = p.org_id AND t.plan_id = p.id AND t.deleted_at IS NULL
WHERE p.org_id = ? AND p.deleted_at IS NULL
  AND (
    p.created_at >= ? AND p.created_at < ?
    OR t.created_at >= ? AND t.created_at < ?
    OR t.open_at >= ? AND t.open_at < ?
    OR t.completed_at >= ? AND t.completed_at < ?
    OR t.expire_at >= ? AND t.expire_at < ?
  )
ORDER BY p.id`
	if limit > 0 {
		query += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := db.QueryContext(ctx, query, orgID, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func cachePatterns(namespace string) (queryPattern, versionPattern string) {
	return prefixRedisKey(namespace, "query:stats:query:*"), prefixRedisKey(namespace, "query:version:stats:query:*")
}

func prefixRedisKey(namespace, key string) string {
	namespace = strings.Trim(strings.TrimSpace(namespace), ":")
	if namespace == "" {
		return key
	}
	return namespace + ":" + key
}

func deleteRedisPattern(ctx context.Context, client redis.UniversalClient, pattern string) (int64, error) {
	var deleted int64
	var batch []string
	iter := client.Scan(ctx, 0, pattern, 1000).Iterator()
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())
		if len(batch) >= 500 {
			n, err := client.Del(ctx, batch...).Result()
			if err != nil {
				return deleted, err
			}
			deleted += n
			batch = batch[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return deleted, err
	}
	if len(batch) > 0 {
		n, err := client.Del(ctx, batch...).Result()
		if err != nil {
			return deleted, err
		}
		deleted += n
	}
	return deleted, nil
}

func newRedisClient(addr, username, password string, db int) *redis.Client {
	opts := &redis.Options{Addr: addr, Password: password, DB: db}
	if strings.TrimSpace(username) != "" {
		opts.Username = username
	}
	return redis.NewClient(opts)
}

// --- shared helpers ---

func openMySQL(dsn string) (*sql.DB, error) {
	normalized, err := normalizeMySQLDSN(dsn)
	if err != nil {
		return nil, err
	}
	return sql.Open("mysql", normalized)
}

func openGorm(dsn string) (*gorm.DB, *sql.DB, error) {
	normalized, err := normalizeMySQLDSN(dsn)
	if err != nil {
		return nil, nil, err
	}
	db, err := gorm.Open(gormMysql.Open(normalized), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	return db, sqlDB, nil
}

func normalizeMySQLDSN(dsn string) (string, error) {
	c, err := driverMysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	c.ParseTime = true
	if c.Collation == "" {
		c.Collation = "utf8mb4_unicode_ci"
	}
	return c.FormatDSN(), nil
}

func pingAndPrepare(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func appendOrgPredicate(query string, args []any, cfg config, alias string) (string, []any) {
	if cfg.orgID <= 0 {
		return query, args
	}
	column := "org_id"
	if strings.TrimSpace(alias) != "" {
		column = strings.TrimSpace(alias) + ".org_id"
	}
	query += " AND " + column + " = ?"
	args = append(args, cfg.orgID)
	return query, args
}

func parseCSV(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	values := make([]string, n)
	for i := range values {
		values[i] = "?"
	}
	return strings.Join(values, ",")
}

func scopeDescription(cfg config) string {
	if cfg.allOrgs {
		return "all_orgs"
	}
	return "org_id=" + strconv.FormatInt(cfg.orgID, 10)
}

func mustParseDate(name, raw string) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		log.Fatalf("--%s is required", name)
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		log.Fatalf("invalid --%s %q: %v", name, raw, err)
	}
	return beginningOfDay(parsed)
}

func parseOptionalDate(name, raw string) *time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	parsed := mustParseDate(name, value)
	return &parsed
}

func beginningOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func formatDay(t time.Time) string {
	return t.Format("2006-01-02")
}

func backupTableName(prefix, suffix string) string {
	return prefix + "_" + suffix
}

func validateBackupSuffix(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("suffix is empty")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("suffix %q contains invalid character %q", value, r)
	}
	return nil
}
