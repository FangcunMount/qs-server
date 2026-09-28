// answersheet_gap_audit performs a bounded, read-only cross-store audit for
// newly durably accepted answer sheets. It never publishes or repairs events.
//
// Exit 0: complete window with no findings requiring attention.
// Exit 2: complete window with at least one finding requiring attention.
// Exit 3: bounded scan stopped before the fixed upper ID.
// Exit 1: invalid input, connection, scan, or output failure.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	mongoURI       string
	mongoDB        string
	mysqlDSN       string
	acceptedBefore string
	afterID        uint64
	upperID        uint64
	batchSize      int
	maxSheets      int
	timeout        time.Duration
}

type pageScanner interface {
	ScanPage(context.Context, uint64, uint64, time.Time, int) (answersheetgap.Page, error)
}

type report struct {
	GeneratedAt    time.Time                          `json:"generated_at"`
	AcceptedBefore time.Time                          `json:"accepted_before"`
	AfterID        uint64                             `json:"after_id"`
	UpperID        uint64                             `json:"upper_id"`
	NextAfterID    uint64                             `json:"next_after_id"`
	Scanned        int                                `json:"scanned"`
	Complete       bool                               `json:"complete"`
	Counts         map[answersheetgap.Disposition]int `json:"counts"`
	Findings       []answersheetgap.Finding           `json:"findings"`
}

func main() { os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }

func runCLI(parent context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, cutoff, err := parseConfig(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "answersheet gap audit: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(stderr, "answersheet gap audit: connect Mongo failed; check URI and server availability")
		return 1
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(stderr, "answersheet gap audit: ping Mongo failed")
		return 1
	}
	if err := requireAuditIndex(ctx, client.Database(cfg.mongoDB).Collection("answersheets")); err != nil {
		fmt.Fprintln(stderr, "answersheet gap audit: required answersheet audit index is missing or unreadable")
		return 1
	}
	db, err := gorm.Open(gormmysql.Open(cfg.mysqlDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(stderr, "answersheet gap audit: connect MySQL failed; check DSN and server availability")
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Fprintf(stderr, "answersheet gap audit: open MySQL connection: %v\n", err)
		return 1
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		fmt.Fprintln(stderr, "answersheet gap audit: ping MySQL failed")
		return 1
	}
	scanner, err := answersheetgap.New(client.Database(cfg.mongoDB), db)
	if err != nil {
		fmt.Fprintf(stderr, "answersheet gap audit: configure scanner: %v\n", err)
		return 1
	}
	result, err := scanWindow(ctx, scanner, cfg, cutoff)
	if err != nil {
		fmt.Fprintf(stderr, "answersheet gap audit: scan failed: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintf(stderr, "answersheet gap audit: encode report: %v\n", err)
		return 1
	}
	if !result.Complete {
		return 3
	}
	if result.Counts[answersheetgap.Missing]+result.Counts[answersheetgap.ManualRequired]+
		result.Counts[answersheetgap.Unknown]+result.Counts[answersheetgap.DeliveryPending] > 0 {
		return 2
	}
	return 0
}

func parseConfig(args []string, stderr io.Writer) (config, time.Time, error) {
	var cfg config
	flags := flag.NewFlagSet("answersheet_gap_audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI (or MONGO_URI)")
	flags.StringVar(&cfg.mongoDB, "mongo-db", os.Getenv("MONGO_DB"), "MongoDB database (or MONGO_DB)")
	flags.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN (or MYSQL_DSN)")
	flags.StringVar(&cfg.acceptedBefore, "accepted-before", "", "fixed RFC3339 acceptance cutoff")
	flags.Uint64Var(&cfg.afterID, "after-id", 0, "exclusive answer sheet ID cursor")
	flags.Uint64Var(&cfg.upperID, "upper-id", 0, "inclusive fixed answer sheet ID upper bound")
	flags.IntVar(&cfg.batchSize, "batch-size", 100, "maximum answer sheets per page (1-500)")
	flags.IntVar(&cfg.maxSheets, "max-sheets", 1000, "maximum answer sheets in this invocation (1-10000)")
	flags.DurationVar(&cfg.timeout, "timeout", 2*time.Minute, "overall deadline (1s-30m)")
	if err := flags.Parse(args); err != nil {
		return config{}, time.Time{}, err
	}
	if flags.NArg() != 0 || cfg.mongoURI == "" || cfg.mongoDB == "" || cfg.mysqlDSN == "" ||
		cfg.afterID >= cfg.upperID || cfg.batchSize < 1 || cfg.batchSize > 500 ||
		cfg.maxSheets < 1 || cfg.maxSheets > 10000 || cfg.timeout < time.Second || cfg.timeout > 30*time.Minute {
		return config{}, time.Time{}, fmt.Errorf("explicit database connections, a nonempty ID window, and bounded scan options are required")
	}
	cutoff, err := time.Parse(time.RFC3339, cfg.acceptedBefore)
	if err != nil || cutoff.After(time.Now()) {
		return config{}, time.Time{}, fmt.Errorf("--accepted-before must be an explicit, non-future RFC3339 time")
	}
	return cfg, cutoff, nil
}

var auditIndexKeys = []string{"durable_acceptance.schema_version", "deleted_at", "domain_id"}

func requireAuditIndex(ctx context.Context, collection *mongo.Collection) error {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			Name                    string `bson:"name"`
			Key                     bson.D `bson:"key"`
			PartialFilterExpression bson.M `bson:"partialFilterExpression"`
			Sparse                  bool   `bson:"sparse"`
			Hidden                  bool   `bson:"hidden"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		if row.Name == "idx_answersheet_durable_audit" && matchesAuditIndexKeys(row.Key) &&
			len(row.PartialFilterExpression) == 0 && !row.Sparse && !row.Hidden {
			return nil
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	return fmt.Errorf("required answersheet audit index is absent or has incompatible keys")
}

func matchesAuditIndexKeys(keys bson.D) bool {
	if len(keys) != len(auditIndexKeys) {
		return false
	}
	for i, field := range auditIndexKeys {
		if keys[i].Key != field {
			return false
		}
		switch value := keys[i].Value.(type) {
		case int32:
			if value != 1 {
				return false
			}
		case int64:
			if value != 1 {
				return false
			}
		case int:
			if value != 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func scanWindow(ctx context.Context, scanner pageScanner, cfg config, cutoff time.Time) (report, error) {
	zone := time.FixedZone("UTC+8", 8*60*60)
	result := report{
		GeneratedAt: time.Now().In(zone), AcceptedBefore: cutoff.In(zone),
		AfterID: cfg.afterID, UpperID: cfg.upperID, NextAfterID: cfg.afterID,
		Counts:   make(map[answersheetgap.Disposition]int),
		Findings: make([]answersheetgap.Finding, 0),
	}
	for result.Scanned < cfg.maxSheets {
		limit := min(cfg.batchSize, cfg.maxSheets-result.Scanned)
		page, err := scanner.ScanPage(ctx, result.NextAfterID, cfg.upperID, cutoff, limit)
		if err != nil {
			return report{}, err
		}
		if len(page.Findings) > limit || !page.Exhausted && (len(page.Findings) == 0 || page.NextID <= result.NextAfterID) {
			return report{}, fmt.Errorf("scanner did not advance the bounded ID cursor")
		}
		previousID := result.NextAfterID
		for _, finding := range page.Findings {
			if finding.AnswerSheetID <= previousID || finding.AnswerSheetID > cfg.upperID {
				return report{}, fmt.Errorf("scanner returned an answer sheet outside the fixed ID window")
			}
			switch finding.Disposition {
			case answersheetgap.Present, answersheetgap.NotRequired, answersheetgap.Missing,
				answersheetgap.DeliveryPending, answersheetgap.Unknown, answersheetgap.ManualRequired:
			default:
				return report{}, fmt.Errorf("scanner returned an unknown disposition")
			}
			previousID = finding.AnswerSheetID
			result.Counts[finding.Disposition]++
			result.Findings = append(result.Findings, finding)
		}
		if len(page.Findings) > 0 && page.NextID != previousID {
			return report{}, fmt.Errorf("scanner returned an inconsistent ID cursor")
		}
		result.Scanned += len(page.Findings)
		if len(page.Findings) > 0 {
			result.NextAfterID = page.NextID
		}
		if page.Exhausted {
			result.Complete = true
			break
		}
	}
	return result, nil
}
