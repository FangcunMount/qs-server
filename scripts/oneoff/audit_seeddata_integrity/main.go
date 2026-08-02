// Command audit_seeddata_integrity audits one historical seed batch across
// MySQL and MongoDB. Its default mode is read-only. Cleanup requires a saved
// audit report, an explicit confirmation phrase, and a Mongo backup suffix.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	dateLayout                     = "2006-01-02"
	applyConfirmation              = "DELETE_SEEDDATA_ORPHANS"
	applyWithoutBackupConfirmation = "DELETE_SEEDDATA_ORPHANS_WITHOUT_BACKUP"
	defaultAuditPageSize           = 500
	defaultReportWorkers           = 8
	defaultMaxFindings             = 1000
	defaultMaxCandidates           = 10000
	defaultOperationTimeout        = 12 * time.Hour
)

type config struct {
	MySQLDSN               string
	MongoURI               string
	MongoDB                string
	BatchID                string
	From                   string
	To                     string
	OutputPath             string
	AuditReport            string
	BackupSuffix           string
	Confirm                string
	Timezone               string
	OrgID                  int64
	PageSize               int
	ReportWorkers          int
	MaxFindings            int
	MaxCandidates          int
	Timeout                time.Duration
	Apply                  bool
	SkipBackup             bool
	ConfirmServicesStopped bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "audit seeddata integrity:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	cfg, err := parseConfig(args, output)
	if err != nil {
		return err
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	ctx := context.Background()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	mysqlDB, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer func() { _ = mysqlDB.Close() }()
	mysqlConnections := max(8, cfg.ReportWorkers)
	mysqlDB.SetMaxOpenConns(mysqlConnections)
	mysqlDB.SetMaxIdleConns(mysqlConnections)
	if err := mysqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("connect mysql: %w", err)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		return fmt.Errorf("ping mongo: %w", err)
	}
	mongoDB := mongoClient.Database(cfg.MongoDB)

	if cfg.Apply {
		result, err := applyAuditReport(ctx, mysqlDB, mongoDB, cfg, output)
		if err != nil {
			return err
		}
		return writeResult(output, cfg.OutputPath, result)
	}

	report, err := auditBatch(ctx, mysqlDB, mongoDB, cfg, output)
	if err != nil {
		return err
	}
	if err := writeResult(output, cfg.OutputPath, report); err != nil {
		return err
	}
	if !report.Valid {
		return fmt.Errorf("batch %s has %d integrity problem(s); review %s", cfg.BatchID, report.ProblemCount, cfg.OutputPath)
	}
	return nil
}

func parseConfig(args []string, output io.Writer) (config, error) {
	flags := flag.NewFlagSet("audit_seeddata_integrity", flag.ContinueOnError)
	flags.SetOutput(output)
	cfg := config{}
	flags.StringVar(&cfg.MySQLDSN, "mysql-dsn", os.Getenv("QS_MYSQL_DSN"), "QS MySQL DSN or QS_MYSQL_DSN")
	flags.StringVar(&cfg.MongoURI, "mongo-uri", firstNonEmpty(os.Getenv("QS_MONGO_URI"), os.Getenv("MONGO_URI")), "QS MongoDB URI")
	flags.StringVar(&cfg.MongoDB, "mongo-db", firstNonEmpty(os.Getenv("QS_MONGO_DB"), os.Getenv("MONGO_DB"), "qs"), "QS MongoDB database")
	flags.Int64Var(&cfg.OrgID, "org-id", 0, "organization ID")
	flags.StringVar(&cfg.BatchID, "batch-id", "", "historical seed batch ID")
	flags.StringVar(&cfg.From, "from", "", "first Shanghai business date, inclusive")
	flags.StringVar(&cfg.To, "to", "", "last Shanghai business date, inclusive")
	flags.StringVar(&cfg.Timezone, "timezone", "Asia/Shanghai", "business timezone")
	flags.StringVar(&cfg.OutputPath, "output", "", "write audit/apply JSON result to this path")
	flags.IntVar(&cfg.PageSize, "page-size", defaultAuditPageSize, "report stages per audit page")
	flags.IntVar(&cfg.ReportWorkers, "report-workers", defaultReportWorkers, "concurrent report-stage page workers (1-16)")
	flags.IntVar(&cfg.MaxFindings, "max-findings", defaultMaxFindings, "maximum detailed non-candidate findings in the report")
	flags.IntVar(&cfg.MaxCandidates, "max-delete-candidates", defaultMaxCandidates, "safety ceiling for exact orphan deletion candidates")
	flags.DurationVar(&cfg.Timeout, "timeout", defaultOperationTimeout, "overall timeout; 0 disables")
	flags.BoolVar(&cfg.Apply, "apply", false, "apply exact deletion candidates from a saved audit report")
	flags.BoolVar(&cfg.SkipBackup, "skip-backup", false, "delete exact candidates without creating tool-managed backups")
	flags.StringVar(&cfg.AuditReport, "audit-report", "", "saved audit report required by --apply")
	flags.StringVar(&cfg.BackupSuffix, "backup-suffix", "", "safe suffix for Mongo backup collections")
	flags.StringVar(&cfg.Confirm, "confirm", "", "destructive confirmation phrase required by the selected apply mode")
	flags.BoolVar(&cfg.ConfirmServicesStopped, "confirm-services-stopped", false, "confirm runner, workers and Statistics scheduler are stopped")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (c *config) validate() error {
	if c == nil {
		return errors.New("config is required")
	}
	c.MySQLDSN = strings.TrimSpace(c.MySQLDSN)
	c.MongoURI = strings.TrimSpace(c.MongoURI)
	c.MongoDB = strings.TrimSpace(c.MongoDB)
	c.BatchID = strings.TrimSpace(c.BatchID)
	c.From = strings.TrimSpace(c.From)
	c.To = strings.TrimSpace(c.To)
	c.OutputPath = strings.TrimSpace(c.OutputPath)
	c.AuditReport = strings.TrimSpace(c.AuditReport)
	c.BackupSuffix = strings.TrimSpace(c.BackupSuffix)
	c.Confirm = strings.TrimSpace(c.Confirm)
	c.Timezone = strings.TrimSpace(c.Timezone)

	if c.MySQLDSN == "" || c.MongoURI == "" || c.MongoDB == "" {
		return errors.New("mysql-dsn, mongo-uri and mongo-db are required")
	}
	if c.OrgID <= 0 || c.BatchID == "" {
		return errors.New("positive org-id and batch-id are required")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._:-]+$`).MatchString(c.BatchID) {
		return errors.New("batch-id contains unsupported characters")
	}
	location, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone: %w", err)
	}
	from, err := time.ParseInLocation(dateLayout, c.From, location)
	if err != nil {
		return fmt.Errorf("invalid from date: %w", err)
	}
	to, err := time.ParseInLocation(dateLayout, c.To, location)
	if err != nil || to.Before(from) {
		return errors.New("invalid inclusive to date")
	}
	if c.PageSize < 1 || c.PageSize > 5000 || c.ReportWorkers < 1 || c.ReportWorkers > 16 || c.MaxFindings < 1 || c.MaxFindings > 100000 || c.MaxCandidates < 1 || c.MaxCandidates > 1000000 {
		return errors.New("page-size, report-workers, max-findings or max-delete-candidates is outside the safe range")
	}
	if c.OutputPath == "" {
		return errors.New("output is required so the audit/apply result can be retained")
	}
	if !c.Apply {
		if c.AuditReport != "" || c.BackupSuffix != "" || c.Confirm != "" || c.SkipBackup || c.ConfirmServicesStopped {
			return errors.New("audit-report, backup-suffix, skip-backup, confirm and confirm-services-stopped are apply-only options")
		}
		return nil
	}
	if c.AuditReport == "" {
		return errors.New("apply requires audit-report")
	}
	if c.SkipBackup {
		if c.BackupSuffix != "" {
			return errors.New("skip-backup and backup-suffix are mutually exclusive")
		}
		if c.Confirm != applyWithoutBackupConfirmation {
			return fmt.Errorf("apply with --skip-backup requires --confirm=%s", applyWithoutBackupConfirmation)
		}
	} else {
		if c.BackupSuffix == "" {
			return errors.New("apply requires backup-suffix unless --skip-backup is set")
		}
		if c.Confirm != applyConfirmation {
			return fmt.Errorf("apply requires --confirm=%s", applyConfirmation)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_]{1,24}$`).MatchString(c.BackupSuffix) {
			return errors.New("backup-suffix must contain 1-24 letters, digits or underscores")
		}
	}
	if !c.ConfirmServicesStopped {
		return errors.New("apply requires --confirm-services-stopped")
	}
	auditPath, err := filepath.Abs(c.AuditReport)
	if err != nil {
		return fmt.Errorf("resolve audit-report: %w", err)
	}
	outputPath, err := filepath.Abs(c.OutputPath)
	if err != nil {
		return fmt.Errorf("resolve output: %w", err)
	}
	if auditPath == outputPath {
		return errors.New("audit-report and output must differ so the approved audit evidence is preserved")
	}
	return nil
}

func (c config) businessWindow() (time.Time, time.Time, error) {
	location, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("load timezone: %w", err)
	}
	from, err := time.ParseInLocation(dateLayout, c.From, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid from date: %w", err)
	}
	to, err := time.ParseInLocation(dateLayout, c.To, location)
	if err != nil || to.Before(from) {
		return time.Time{}, time.Time{}, errors.New("invalid inclusive to date")
	}
	return from, to.AddDate(0, 0, 1), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
