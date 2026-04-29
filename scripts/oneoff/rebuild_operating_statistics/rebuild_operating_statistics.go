package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	dbmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type config struct {
	mysqlDSN string
	orgID    int64
	allOrgs  bool
	fromRaw  string
	toRaw    string
	from     time.Time
	to       time.Time
	apply    bool
	timeout  time.Duration
}

func main() {
	cfg := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	db, err := gorm.Open(gormmysql.Open(cfg.mysqlDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}

	orgIDs, err := resolveOrgIDs(ctx, db, cfg)
	if err != nil {
		log.Fatalf("resolve org ids: %v", err)
	}
	if len(orgIDs) == 0 {
		log.Print("no organizations to rebuild")
		return
	}

	log.Printf("rebuild scope: orgs=%v from=%s to=%s apply=%v",
		orgIDs, cfg.from.Format("2006-01-02"), cfg.to.Format("2006-01-02"), cfg.apply)
	for _, orgID := range orgIDs {
		if err := printSummary(ctx, db, orgID, cfg, "before"); err != nil {
			log.Fatalf("summary before org %d: %v", orgID, err)
		}
	}
	if !cfg.apply {
		log.Print("dry-run only; re-run with --apply to rebuild operating statistics projections")
		return
	}

	repo := statisticsInfra.NewStatisticsRepository(db)
	for _, orgID := range orgIDs {
		if err := rebuildOrg(ctx, db, repo, orgID, cfg); err != nil {
			log.Fatalf("rebuild org %d: %v", orgID, err)
		}
		if err := printSummary(ctx, db, orgID, cfg, "after"); err != nil {
			log.Fatalf("summary after org %d: %v", orgID, err)
		}
	}
	log.Print("operating statistics rebuild completed")
}

func parseFlags() config {
	var cfg config
	defaultTo := time.Now().In(time.Local).AddDate(0, 0, 1).Format("2006-01-02")
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/qs?parseTime=true&loc=Local")
	flag.Int64Var(&cfg.orgID, "org-id", 0, "organization ID to rebuild; mutually exclusive with --all-orgs")
	flag.BoolVar(&cfg.allOrgs, "all-orgs", false, "rebuild all organizations")
	flag.StringVar(&cfg.fromRaw, "from", "1970-01-01", "inclusive start date, format YYYY-MM-DD")
	flag.StringVar(&cfg.toRaw, "to", defaultTo, "exclusive end date, format YYYY-MM-DD")
	flag.BoolVar(&cfg.apply, "apply", false, "apply changes; default is dry-run")
	flag.DurationVar(&cfg.timeout, "timeout", 4*time.Hour, "overall timeout, e.g. 30m, 4h")
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
	var err error
	cfg.from, err = time.ParseInLocation("2006-01-02", cfg.fromRaw, time.Local)
	if err != nil {
		log.Fatalf("invalid --from: %v", err)
	}
	cfg.to, err = time.ParseInLocation("2006-01-02", cfg.toRaw, time.Local)
	if err != nil {
		log.Fatalf("invalid --to: %v", err)
	}
	if !cfg.from.Before(cfg.to) {
		log.Fatal("--from must be before --to")
	}
	return cfg
}

func resolveOrgIDs(ctx context.Context, db *gorm.DB, cfg config) ([]int64, error) {
	if !cfg.allOrgs {
		return []int64{cfg.orgID}, nil
	}
	var rows []struct {
		OrgID int64
	}
	err := db.WithContext(ctx).Raw(`
		SELECT DISTINCT org_id
		FROM (
		  SELECT org_id FROM testee WHERE deleted_at IS NULL
		  UNION
		  SELECT org_id FROM assessment WHERE deleted_at IS NULL
		  UNION
		  SELECT org_id FROM assessment_plan WHERE deleted_at IS NULL
		) orgs
		ORDER BY org_id`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.OrgID > 0 {
			result = append(result, row.OrgID)
		}
	}
	return result, nil
}

func rebuildOrg(ctx context.Context, db *gorm.DB, repo *statisticsInfra.StatisticsRepository, orgID int64, cfg config) error {
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	txCtx := dbmysql.WithTx(ctx, tx)
	if err := repo.RebuildDailyStatistics(txCtx, orgID, cfg.from, cfg.to); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("rebuild daily projections: %w", err)
	}
	if err := repo.RebuildAccumulatedStatistics(txCtx, orgID, cfg.to); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("rebuild accumulated statistics: %w", err)
	}
	if err := repo.RebuildPlanStatistics(txCtx, orgID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("rebuild plan statistics: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	return nil
}

func printSummary(ctx context.Context, db *gorm.DB, orgID int64, cfg config, stage string) error {
	items := []struct {
		label string
		sql   string
		args  []interface{}
	}{
		{"source testee.created", "SELECT COUNT(*) AS count FROM testee WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?", []interface{}{orgID, cfg.from, cfg.to}},
		{"source assessment.created", "SELECT COUNT(*) AS count FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?", []interface{}{orgID, cfg.from, cfg.to}},
		{"source assessment.report", "SELECT COUNT(*) AS count FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at >= ? AND interpreted_at < ?", []interface{}{orgID, cfg.from, cfg.to}},
		{"source plan.tasks", "SELECT COUNT(*) AS count FROM assessment_task WHERE org_id = ? AND deleted_at IS NULL", []interface{}{orgID}},
		{"projection access_org_daily", "SELECT COUNT(*) AS count FROM analytics_access_org_daily WHERE org_id = ? AND stat_date >= ? AND stat_date < ?", []interface{}{orgID, cfg.from, cfg.to}},
		{"projection assessment_service_org_daily", "SELECT COUNT(*) AS count FROM analytics_assessment_service_org_daily WHERE org_id = ? AND stat_date >= ? AND stat_date < ?", []interface{}{orgID, cfg.from, cfg.to}},
		{"projection plan_task_daily", "SELECT COUNT(*) AS count FROM analytics_plan_task_daily WHERE org_id = ?", []interface{}{orgID}},
		{"snapshot organization", "SELECT COUNT(*) AS count FROM analytics_organization_snapshot WHERE org_id = ? AND deleted_at IS NULL", []interface{}{orgID}},
		{"snapshot plan_task_window", "SELECT COUNT(*) AS count FROM analytics_plan_task_window_snapshot WHERE org_id = ? AND deleted_at IS NULL", []interface{}{orgID}},
	}
	for _, item := range items {
		count, err := countQuery(ctx, db, item.sql, item.args...)
		if err != nil {
			return fmt.Errorf("%s: %w", item.label, err)
		}
		log.Printf("%s org=%d %s=%d", stage, orgID, item.label, count)
	}
	return nil
}

func countQuery(ctx context.Context, db *gorm.DB, sql string, args ...interface{}) (int64, error) {
	var row struct {
		Count int64
	}
	if err := db.WithContext(ctx).Raw(sql, args...).Scan(&row).Error; err != nil {
		return 0, err
	}
	return row.Count, nil
}
