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

type config struct {
	mysqlDSN           string
	orgID              int64
	allOrgs            bool
	startDateRaw       string
	endDateRaw         string
	timeout            time.Duration
	apply              bool
	skipAggregate      bool
	skipCache          bool
	redisAddr          string
	redisQueryAddr     string
	redisMetaAddr      string
	redisUsername      string
	redisQueryUsername string
	redisMetaUsername  string
	redisPassword      string
	redisQueryDB       int
	redisMetaDB        int
	redisQueryNS       string
	maxQuestionnaires  int
	maxPlans           int
	questionnaireCodes csvFlag
	planIDs            uint64CSVFlag
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
		tomorrow := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
		endDate = &tomorrow
	}
	if !startDate.Before(*endDate) {
		log.Fatal("--start-date must be before --end-date")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	db, sqlDB, err := openGorm(cfg.mysqlDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("close mysql: %v", err)
		}
	}()
	if err := prepareMySQL(ctx, sqlDB); err != nil {
		log.Fatalf("prepare mysql: %v", err)
	}

	orgIDs, err := resolveOrgIDs(ctx, sqlDB, cfg, startDate, *endDate)
	if err != nil {
		log.Fatalf("resolve org ids: %v", err)
	}
	log.Printf("scope: orgs=%v start=%s end=%s apply=%v aggregate=%v cache=%v",
		orgIDs, formatDay(startDate), formatDay(*endDate), cfg.apply, !cfg.skipAggregate, !cfg.skipCache)
	if len(orgIDs) == 0 {
		log.Print("scope is empty; nothing to rebuild")
		return
	}

	warmScopes, err := resolveWarmScopes(ctx, sqlDB, cfg, orgIDs, startDate, *endDate)
	if err != nil {
		log.Fatalf("resolve cache warm scopes: %v", err)
	}
	for _, scope := range warmScopes {
		log.Printf("warm scope org_id=%d questionnaires=%d plans=%d", scope.OrgID, len(scope.QuestionnaireCodes), len(scope.PlanIDs))
	}
	if !cfg.apply {
		if cfg.redisEnabled() && !cfg.skipCache {
			queryPattern, versionPattern := cachePatterns(cfg.redisQueryNS)
			log.Printf("dry-run cache patterns: query=%q version=%q", queryPattern, versionPattern)
		}
		log.Print("dry-run only; re-run with --apply to rebuild statistics aggregates and Redis query cache")
		return
	}

	if !cfg.skipAggregate {
		if err := rebuildAggregates(ctx, db, orgIDs, startDate, *endDate); err != nil {
			log.Fatalf("rebuild aggregates: %v", err)
		}
	}

	if !cfg.skipCache {
		if !cfg.redisEnabled() {
			log.Print("redis is not configured; skip statistics query cache rebuild")
		} else if err := rebuildCache(ctx, db, cfg, warmScopes); err != nil {
			log.Fatalf("rebuild cache: %v", err)
		}
	}
	log.Print("statistics aggregate/cache rebuild completed")
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to rebuild; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "rebuild all organizations with source data in the selected window")
	flag.StringVar(&cfg.startDateRaw, "start-date", "2025-01-01", "inclusive lower bound, format YYYY-MM-DD")
	flag.StringVar(&cfg.endDateRaw, "end-date", "", "exclusive upper bound, format YYYY-MM-DD; default is tomorrow")
	flag.DurationVar(&cfg.timeout, "timeout", 4*time.Hour, "overall script timeout, e.g. 30m, 4h")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.BoolVar(&cfg.skipAggregate, "skip-aggregate", false, "skip MySQL aggregate rebuild")
	flag.BoolVar(&cfg.skipCache, "skip-cache", false, "skip Redis query cache rebuild")
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

func prepareMySQL(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func resolveOrgIDs(ctx context.Context, db *sql.DB, cfg config, startDate, endDate time.Time) ([]int64, error) {
	if !cfg.allOrgs {
		return []int64{cfg.orgID}, nil
	}
	rows, err := db.QueryContext(ctx, `
SELECT DISTINCT org_id
FROM (
  SELECT org_id FROM assessment
  WHERE deleted_at IS NULL
    AND (
      created_at >= ? AND created_at < ?
      OR submitted_at >= ? AND submitted_at < ?
      OR interpreted_at >= ? AND interpreted_at < ?
      OR failed_at >= ? AND failed_at < ?
    )
  UNION
  SELECT org_id FROM assessment_task
  WHERE deleted_at IS NULL
    AND (
      created_at >= ? AND created_at < ?
      OR open_at >= ? AND open_at < ?
      OR completed_at >= ? AND completed_at < ?
      OR expire_at >= ? AND expire_at < ?
    )
  UNION
  SELECT org_id FROM testee WHERE deleted_at IS NULL AND created_at >= ? AND created_at < ?
  UNION
  SELECT org_id FROM clinician_relation WHERE deleted_at IS NULL AND bound_at >= ? AND bound_at < ?
) scoped
ORDER BY org_id`,
		startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate,
		startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate,
		startDate, endDate, startDate, endDate,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("close org rows: %v", err)
		}
	}()
	var orgIDs []int64
	for rows.Next() {
		var orgID int64
		if err := rows.Scan(&orgID); err != nil {
			return nil, err
		}
		if orgID > 0 {
			orgIDs = append(orgIDs, orgID)
		}
	}
	return orgIDs, rows.Err()
}

func resolveWarmScopes(ctx context.Context, db *sql.DB, cfg config, orgIDs []int64, startDate, endDate time.Time) ([]warmScope, error) {
	scopes := make([]warmScope, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		codes := append([]string(nil), cfg.questionnaireCodes...)
		if len(codes) == 0 {
			discovered, err := discoverQuestionnaireCodes(ctx, db, orgID, startDate, endDate, cfg.maxQuestionnaires)
			if err != nil {
				return nil, err
			}
			codes = discovered
		}
		plans := append([]uint64(nil), cfg.planIDs...)
		if len(plans) == 0 {
			discovered, err := discoverPlanIDs(ctx, db, orgID, startDate, endDate, cfg.maxPlans)
			if err != nil {
				return nil, err
			}
			plans = discovered
		}
		scopes = append(scopes, warmScope{OrgID: orgID, QuestionnaireCodes: dedupeStrings(codes), PlanIDs: dedupeUint64(plans)})
	}
	return scopes, nil
}

func discoverQuestionnaireCodes(ctx context.Context, db *sql.DB, orgID int64, startDate, endDate time.Time, limit int) ([]string, error) {
	query := `
SELECT DISTINCT questionnaire_code
FROM assessment
WHERE org_id = ? AND deleted_at IS NULL
  AND questionnaire_code <> ''
  AND (
    created_at >= ? AND created_at < ?
    OR submitted_at >= ? AND submitted_at < ?
    OR interpreted_at >= ? AND interpreted_at < ?
    OR failed_at >= ? AND failed_at < ?
  )
ORDER BY questionnaire_code`
	if limit > 0 {
		query += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := db.QueryContext(ctx, query, orgID, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			log.Printf("close questionnaire rows: %v", err)
		}
	}()
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

func discoverPlanIDs(ctx context.Context, db *sql.DB, orgID int64, startDate, endDate time.Time, limit int) ([]uint64, error) {
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
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("close plan rows: %v", err)
		}
	}()
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

func rebuildAggregates(ctx context.Context, db *gorm.DB, orgIDs []int64, startDate, endDate time.Time) error {
	repo := statisticsInfra.NewStatisticsRepository(db)
	for _, orgID := range orgIDs {
		log.Printf("rebuild aggregate org_id=%d daily/content/journey window", orgID)
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
		log.Printf("rebuild aggregate org_id=%d plan daily", orgID)
		if err := withinTx(ctx, db, func(txCtx context.Context) error {
			if err := repo.RebuildPlanStatistics(txCtx, orgID); err != nil {
				return err
			}
			tx, err := gormuow.RequireTx(txCtx)
			if err != nil {
				return err
			}
			return tx.WithContext(txCtx).
				Exec("DELETE FROM statistics_plan_daily WHERE org_id = ? AND (stat_date < ? OR stat_date >= ?)", orgID, startDate, endDate).
				Error
		}); err != nil {
			return fmt.Errorf("org %d plan: %w", orgID, err)
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

func newRedisClient(addr, username, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr, Username: username, Password: password, DB: db})
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

func (cfg config) redisEnabled() bool {
	return strings.TrimSpace(cfg.redisQueryAddr) != "" && strings.TrimSpace(cfg.redisMetaAddr) != ""
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func dedupeUint64(values []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(values))
	result := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
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

func formatDay(t time.Time) string {
	return t.Format("2006-01-02")
}
