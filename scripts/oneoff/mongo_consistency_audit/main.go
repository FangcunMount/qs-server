// mongo_consistency_audit runs the production cross-collection audit without
// starting the scheduler or writing its checkpoint. It is strictly read-only.
//
// Exit 0: scan completed with no drift.
// Exit 2: scan completed and drift was found.
// Exit 1: configuration, connection, or scan failure.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	mongoconsistency "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	mongoscanner "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/mongoconsistency"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mongoURI     string
	mongoDB      string
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

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	scopes, err := parseScope(cfg.scope)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed:", err)
		os.Exit(1)
	}
	if cfg.batchSize <= 0 || cfg.batchSize > 500 || cfg.batchTimeout <= 0 || cfg.maxSamples < 0 || cfg.maxSamples > 100 || cfg.timeout <= 0 {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: invalid bounded scan options")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "mongo consistency audit failed: ping mongo:", err)
		os.Exit(1)
	}

	started := time.Now()
	stats := mongoconsistency.NewStatistics()
	scanner := mongoscanner.NewScanner(client.Database(cfg.mongoDB), nil)
	for _, scope := range scopes {
		upper, err := scanner.UpperBound(ctx, scope, cfg.batchTimeout)
		if err != nil {
			fail(scope, err)
		}
		cursor := uint64(0)
		for {
			batch, err := scanner.ScanBatch(ctx, mongoconsistency.BatchRequest{
				Phase: scope, AfterID: cursor, UpperBound: upper, Limit: cfg.batchSize,
				MaxTime: cfg.batchTimeout, MaxSamples: cfg.maxSamples,
			})
			if err != nil {
				fail(scope, err)
			}
			stats.Add(batch, cfg.maxSamples)
			cursor = batch.NextID
			if batch.Exhausted {
				break
			}
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
			os.Exit(1)
		}
	} else {
		printReport(result)
	}
	if stats.Total() > 0 {
		os.Exit(2)
	}
}

func parseFlags() config {
	var cfg config
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
}

func fail(scope mongoconsistency.Phase, err error) {
	fmt.Fprintf(os.Stderr, "mongo consistency audit failed: scope=%s: %v\n", scope, err)
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
