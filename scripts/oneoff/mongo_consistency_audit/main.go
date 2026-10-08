// mongo_consistency_audit runs the production cross-collection audit without
// starting the scheduler or writing its checkpoint. It is strictly read-only.
//
// Exit 0: scan completed with no drift.
// Exit 2: scan completed and drift was found.
// Exit 1: configuration, connection, or scan failure.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	mongoconsistency "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	mongoscanner "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/mongoconsistency"
	mysqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	mongoURI     string
	mongoDB      string
	mysqlDSN     string
	scope        string
	batchSize    int
	batchTimeout time.Duration
	maxSamples   int
	timeout      time.Duration
	jsonOut      bool
}

type report struct {
	GeneratedAt        time.Time                   `json:"generated_at"`
	Scopes             []mongoconsistency.Phase    `json:"scopes"`
	Statistics         mongoconsistency.Statistics `json:"statistics"`
	ReportCatalogAudit string                      `json:"report_catalog_audit"`
	Duration           string                      `json:"duration"`
}

func main() { os.Exit(run(parseFlags())) }

func run(cfg config) int {
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: --mongo-uri is required (or set MONGO_URI)")
		return 1
	}
	scopes, err := parseScope(cfg.scope)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed:", err)
		return 1
	}
	if cfg.batchSize <= 0 || cfg.batchSize > 500 || cfg.batchTimeout <= 0 || cfg.maxSamples < 0 || cfg.maxSamples > 100 || cfg.timeout <= 0 {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: invalid bounded scan options")
		return 1
	}
	if requiresOutcomeFacts(scopes) && strings.TrimSpace(cfg.mysqlDSN) == "" {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: MYSQL_DSN is required for standard reverse/report/retry business facts")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	var facts evaluationfact.Repository
	if requiresOutcomeFacts(scopes) {
		var closeFacts func()
		facts, closeFacts, err = openReadOnlyFacts(ctx, cfg.mysqlDSN)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mongo consistency audit failed: read-only MySQL fact connection unavailable")
			return 1
		}
		defer closeFacts()
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: connect mongo")
		return 1
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: ping mongo")
		return 1
	}

	started := time.Now()
	stats := mongoconsistency.NewStatistics()
	scanner := mongoscanner.NewScanner(client.Database(cfg.mongoDB), nil).WithOutcomeFacts(facts)
	for _, scope := range scopes {
		phaseStats, err := scanScope(ctx, scanner, scope, cfg)
		if err != nil {
			fail(scope)
			return 1
		}
		stats.Scanned += phaseStats.Scanned
		for kind, count := range phaseStats.Findings {
			stats.Findings[kind] += count
			for _, sample := range phaseStats.Samples[kind] {
				if len(stats.Samples[kind]) < cfg.maxSamples {
					stats.Samples[kind] = append(stats.Samples[kind], sample)
				}
			}
		}
		if stats.EvidenceClasses == nil {
			stats.EvidenceClasses = map[string]int64{}
		}
		for class, count := range phaseStats.EvidenceClasses {
			stats.EvidenceClasses[class] += count
		}
	}
	result := report{
		GeneratedAt: time.Now().UTC(), Scopes: scopes, Statistics: stats,
		ReportCatalogAudit: "reused separately; this command does not rescan artifact/query-catalog winner drift",
		Duration:           time.Since(started).String(),
	}
	if cfg.jsonOut {
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "mongo consistency audit failed: encode report:", err)
			return 1
		}
	} else {
		printReport(result)
	}
	if stats.Total() > 0 {
		return 2
	}
	return 0
}

func parseFlags() config {
	var cfg config
	cfg.mysqlDSN = os.Getenv("MYSQL_DSN") // env only; never expose credentials in argv/help
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database")
	flag.StringVar(&cfg.scope, "scope", "all", "comma-separated audit scopes or all")
	flag.IntVar(&cfg.batchSize, "batch-size", 200, "maximum anchor documents per batch (1-500)")
	flag.DurationVar(&cfg.batchTimeout, "batch-timeout", 3*time.Second, "deadline and maxTimeMS for one batch")
	flag.IntVar(&cfg.maxSamples, "max-samples", 10, "maximum internal sample IDs per drift kind (0-100)")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Minute, "overall audit deadline")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON")
	flag.Parse()
	return cfg
}

func parseScope(value string) ([]mongoconsistency.Phase, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "all" {
		return mongoconsistency.ParseScopes(nil)
	}
	raw := strings.Split(value, ",")
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return mongoconsistency.ParseScopes(values)
}

func printReport(result report) {
	fmt.Printf("Mongo consistency audit: scanned=%d drift=%d duration=%s\n", result.Statistics.Scanned, result.Statistics.Total(), result.Duration)
	for _, kind := range mongoconsistency.SortedFindingKinds(result.Statistics) {
		fmt.Printf("  %-44s severity=%-6s count=%d samples=%v\n",
			kind, mongoconsistency.DriftSeverities[kind], result.Statistics.Findings[kind], result.Statistics.Samples[kind])
	}
	fmt.Println("  report_catalog_audit: reused separately")
	for _, class := range []string{"standard_reference", "retired_verified", "unverifiable"} {
		fmt.Printf("  event_evidence %-20s count=%d\n", class, result.Statistics.EvidenceClasses[class])
	}
}

func fail(scope mongoconsistency.Phase) {
	fmt.Fprintf(os.Stderr, "mongo consistency audit failed: scope=%s: bounded read or verification failed\n", scope)
}

func requiresOutcomeFacts(scopes []mongoconsistency.Phase) bool {
	for _, scope := range scopes {
		if scope == mongoconsistency.PhaseOutboxAnswerSheet || scope == mongoconsistency.PhaseGeneratedTerminal || scope == mongoconsistency.PhaseRetryOutbox {
			return true
		}
	}
	return false
}

func scanScope(ctx context.Context, scanner mongoconsistency.Scanner, scope mongoconsistency.Phase, cfg config) (mongoconsistency.Statistics, error) {
	stats := mongoconsistency.NewStatistics()
	request := mongoconsistency.BatchRequest{Phase: scope, Limit: cfg.batchSize, MaxTime: cfg.batchTimeout, MaxSamples: cfg.maxSamples}
	var err error
	if scope == mongoconsistency.PhaseOutboxAnswerSheet {
		request.OutboxUpperBound, err = scanner.OutboxUpperBound(ctx, cfg.batchTimeout)
	} else {
		request.UpperBound, err = scanner.UpperBound(ctx, scope, cfg.batchTimeout)
	}
	if err != nil {
		return stats, err
	}
	for {
		batch, err := scanner.ScanBatch(ctx, request)
		if err != nil {
			return stats, err
		}
		if !batch.Exhausted && (batch.Scanned <= 0 || (scope == mongoconsistency.PhaseOutboxAnswerSheet && (len(batch.NextOutboxCursor) == 0 || bytes.Equal(batch.NextOutboxCursor, request.OutboxCursor))) || (scope != mongoconsistency.PhaseOutboxAnswerSheet && batch.NextID <= request.AfterID)) {
			return stats, fmt.Errorf("audit cursor made no progress")
		}
		stats.Add(batch, cfg.maxSamples)
		if batch.Exhausted {
			return stats, nil
		}
		request.AfterID = batch.NextID
		request.OutboxCursor = append([]byte(nil), batch.NextOutboxCursor...)
	}
}

// This command owns the connection and its read-only transaction. The borrowed
// repository and scanner cannot open, commit, or close either resource.
func openReadOnlyFacts(ctx context.Context, rawDSN string) (evaluationfact.Repository, func(), error) {
	parsed, err := mysqldriver.ParseDSN(rawDSN)
	if err != nil {
		return nil, nil, err
	}
	parsed.ParseTime = true
	parsed.MultiStatements = false
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{DSN: parsed.FormatDSN(), SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, nil, err
	}
	pool, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, nil, err
	}
	readOnly := db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true})
	if readOnly.Error != nil {
		_ = pool.Close()
		return nil, nil, readOnly.Error
	}
	return factReader{source: mysqlevaluation.NewOutcomeRepository(readOnly)}, func() { _ = readOnly.Rollback().Error; _ = pool.Close() }, nil
}

type outcomeReads interface {
	FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error)
	FindByAssessmentID(context.Context, meta.ID) (*domainoutcome.Record, error)
}
type factReader struct{ source outcomeReads }

func (r factReader) FindByID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	record, err := r.source.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, evaluationfact.ErrNotFound
	}
	return evaloutcome.FactRecord(record), nil
}
func (r factReader) FindByAssessmentID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	record, err := r.source.FindByAssessmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, evaluationfact.ErrNotFound
	}
	return evaloutcome.FactRecord(record), nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
