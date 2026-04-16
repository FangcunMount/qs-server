package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const dsnEnvKey = "QS_PENDING_ASSESSMENT_DSN"

type options struct {
	dsn            string
	eventsConfig   string
	apply          bool
	batchSize      int
	maxCount       int
	sleepBetween   time.Duration
	orgID          int64
	afterID        uint64
	beforeID       uint64
	createdAfter   string
	createdBefore  string
	includeNoScale bool
	verbose        bool
}

func main() {
	opts := parseFlags()

	if opts.dsn == "" {
		opts.dsn = os.Getenv(dsnEnvKey)
	}
	if opts.dsn == "" {
		log.Fatalf("mysql dsn is required (use --dsn or set %s)", dsnEnvKey)
	}

	if err := eventconfig.Initialize(opts.eventsConfig); err != nil {
		log.Fatalf("failed to initialize event config: %v", err)
	}

	db := mustOpenGORM(opts.dsn, opts.verbose)
	defer closeGORM(db)

	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	createdAfter, createdBefore, err := parseTimeRange(opts.createdAfter, opts.createdBefore)
	if err != nil {
		log.Fatalf("invalid time range: %v", err)
	}

	filters := queryFilters{
		orgID:          opts.orgID,
		afterID:        opts.afterID,
		beforeID:       opts.beforeID,
		createdAfter:   createdAfter,
		createdBefore:  createdBefore,
		includeNoScale: opts.includeNoScale,
	}

	total, err := countPendingAssessments(runCtx, db, filters)
	if err != nil {
		log.Fatalf("count pending assessments: %v", err)
	}

	log.Printf(
		"matched pending assessments: total=%d org_id=%d after_id=%d before_id=%d include_no_scale=%v created_after=%s created_before=%s",
		total,
		opts.orgID,
		opts.afterID,
		opts.beforeID,
		opts.includeNoScale,
		formatTime(createdAfter),
		formatTime(createdBefore),
	)

	if !opts.apply {
		log.Printf("dry-run only; re-run with --apply to submit matching assessments and stage assessment.submitted events")
		return
	}

	repo := mysqlEval.NewAssessmentRepository(db)
	mapper := mysqlEval.NewAssessmentMapper()

	startTime := time.Now()
	var (
		lastID    = opts.afterID
		submitted int
		failed    int
		batchNo   int
		remaining = opts.maxCount
	)

	for {
		if opts.maxCount > 0 && remaining <= 0 {
			break
		}

		limit := opts.batchSize
		if limit <= 0 {
			limit = 100
		}
		if opts.maxCount > 0 && remaining < limit {
			limit = remaining
		}

		batch, err := loadPendingBatch(runCtx, db, filters, lastID, limit)
		if err != nil {
			log.Fatalf("load pending batch after id %d: %v", lastID, err)
		}
		if len(batch) == 0 {
			break
		}

		batchNo++
		batchSubmitted := 0
		batchFailed := 0
		batchStart := time.Now()

		for _, po := range batch {
			lastID = uint64(po.ID.Uint64())

			a := mapper.ToDomain(po)
			if a == nil {
				batchFailed++
				failed++
				log.Printf("batch=%d assessment_id=%d convert_to_domain=nil", batchNo, po.ID)
				continue
			}

			if !a.Status().IsPending() {
				log.Printf("batch=%d assessment_id=%d skip non-pending status=%s", batchNo, po.ID, a.Status())
				continue
			}

			if err := a.Submit(); err != nil {
				batchFailed++
				failed++
				log.Printf("batch=%d assessment_id=%d submit failed: %v", batchNo, po.ID, err)
				continue
			}

			if err := repo.SaveWithEvents(runCtx, a); err != nil {
				batchFailed++
				failed++
				log.Printf("batch=%d assessment_id=%d save_with_events failed: %v", batchNo, po.ID, err)
				continue
			}

			batchSubmitted++
			submitted++
			if opts.maxCount > 0 {
				remaining--
			}
		}

		log.Printf(
			"batch=%d done fetched=%d submitted=%d failed=%d last_id=%d duration=%s total_submitted=%d total_failed=%d",
			batchNo,
			len(batch),
			batchSubmitted,
			batchFailed,
			lastID,
			time.Since(batchStart).Round(time.Millisecond),
			submitted,
			failed,
		)

		if opts.sleepBetween > 0 {
			select {
			case <-runCtx.Done():
				log.Printf("interrupted during batch sleep: %v", runCtx.Err())
				goto DONE
			case <-time.After(opts.sleepBetween):
			}
		}
	}

DONE:
	log.Printf(
		"finished: submitted=%d failed=%d elapsed=%s",
		submitted,
		failed,
		time.Since(startTime).Round(time.Millisecond),
	)
}

type queryFilters struct {
	orgID          int64
	afterID        uint64
	beforeID       uint64
	createdAfter   *time.Time
	createdBefore  *time.Time
	includeNoScale bool
}

func basePendingQuery(ctx context.Context, db *gorm.DB, filters queryFilters) *gorm.DB {
	query := db.WithContext(ctx).
		Model(&mysqlEval.AssessmentPO{}).
		Where("status = ? AND deleted_at IS NULL", domainAssessment.StatusPending.String())

	if !filters.includeNoScale {
		query = query.Where("medical_scale_id IS NOT NULL")
	}
	if filters.orgID > 0 {
		query = query.Where("org_id = ?", filters.orgID)
	}
	if filters.beforeID > 0 {
		query = query.Where("id <= ?", filters.beforeID)
	}
	if filters.createdAfter != nil {
		query = query.Where("created_at >= ?", *filters.createdAfter)
	}
	if filters.createdBefore != nil {
		query = query.Where("created_at <= ?", *filters.createdBefore)
	}
	return query
}

func countPendingAssessments(ctx context.Context, db *gorm.DB, filters queryFilters) (int64, error) {
	var total int64
	err := basePendingQuery(ctx, db, filters).Count(&total).Error
	return total, err
}

func loadPendingBatch(ctx context.Context, db *gorm.DB, filters queryFilters, lastID uint64, limit int) ([]*mysqlEval.AssessmentPO, error) {
	var batch []*mysqlEval.AssessmentPO

	query := basePendingQuery(ctx, db, filters)
	if lastID > 0 {
		query = query.Where("id > ?", lastID)
	}

	err := query.
		Order("id ASC").
		Limit(limit).
		Find(&batch).Error
	if err != nil {
		return nil, err
	}
	return batch, nil
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.dsn, "dsn", "", "MySQL DSN; falls back to QS_PENDING_ASSESSMENT_DSN")
	flag.StringVar(&opts.eventsConfig, "events-config", "configs/events.yaml", "Path to events.yaml used to resolve outbox topics")
	flag.BoolVar(&opts.apply, "apply", false, "Actually submit matching pending assessments; default is dry-run")
	flag.IntVar(&opts.batchSize, "batch-size", 100, "Number of rows fetched per batch")
	flag.IntVar(&opts.maxCount, "max-count", 0, "Maximum number of assessments to submit in this run; 0 means no limit")
	flag.DurationVar(&opts.sleepBetween, "sleep", 0, "Optional sleep duration between batches")
	flag.Int64Var(&opts.orgID, "org-id", 0, "Only process assessments from this org_id; 0 means all orgs")
	flag.Uint64Var(&opts.afterID, "after-id", 0, "Only process assessment.id greater than this value")
	flag.Uint64Var(&opts.beforeID, "before-id", 0, "Only process assessment.id less than or equal to this value")
	flag.StringVar(&opts.createdAfter, "created-after", "", "Only process rows created at or after this RFC3339 timestamp")
	flag.StringVar(&opts.createdBefore, "created-before", "", "Only process rows created at or before this RFC3339 timestamp")
	flag.BoolVar(&opts.includeNoScale, "include-no-scale", false, "Also submit pending assessments without medical_scale_id; default only processes rows that need downstream evaluation")
	flag.BoolVar(&opts.verbose, "verbose", false, "Enable GORM SQL logs")
	flag.Parse()
	return opts
}

func parseTimeRange(afterRaw, beforeRaw string) (*time.Time, *time.Time, error) {
	var after *time.Time
	var before *time.Time

	if afterRaw != "" {
		t, err := time.Parse(time.RFC3339, afterRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("parse created-after: %w", err)
		}
		after = &t
	}
	if beforeRaw != "" {
		t, err := time.Parse(time.RFC3339, beforeRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("parse created-before: %w", err)
		}
		before = &t
	}
	return after, before, nil
}

func mustOpenGORM(dsn string, verbose bool) *gorm.DB {
	logLevel := logger.Silent
	if verbose {
		logLevel = logger.Info
	}
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	}

	db, err := gorm.Open(mysql.Open(dsn), cfg)
	if err != nil {
		log.Fatalf("failed to open gorm connection: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get sql DB from gorm: %v", err)
	}

	sqlDB.SetConnMaxIdleTime(30 * time.Second)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(20)

	pingCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		log.Fatalf("failed to ping mysql: %v", err)
	}

	return db
}

func closeGORM(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
