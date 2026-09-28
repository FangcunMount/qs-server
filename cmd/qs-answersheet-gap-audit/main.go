// qs-answersheet-gap-audit performs a bounded, read-only check of accepted
// AnswerSheets against their original standard Outbox intent and Assessment.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type findingSample struct {
	AnswerSheetID uint64                     `json:"answer_sheet_id"`
	EventID       string                     `json:"event_id"`
	Disposition   answersheetgap.Disposition `json:"disposition"`
	Reason        string                     `json:"reason,omitempty"`
}

type report struct {
	AfterID        uint64         `json:"after_id"`
	UpperID        uint64         `json:"upper_id"`
	AcceptedBefore time.Time      `json:"accepted_before_utc"`
	NextAfterID    uint64         `json:"next_after_id"`
	Pages          int            `json:"pages"`
	Scanned        int            `json:"scanned"`
	Complete       bool           `json:"complete"`
	Counts         map[string]int `json:"counts"`
}

type connectionInput struct {
	MySQLDSN    string `json:"mysql_dsn"`
	MongoURI    string `json:"mongo_uri"`
	MongoDBName string `json:"mongo_db_name"`
}

func main() {
	if err := run(); err != nil {
		// Connection errors can include endpoint details. Only emit a fixed
		// category; the process exit code and report absence mark the audit invalid.
		fmt.Fprintln(os.Stderr, "read-only answer-sheet gap audit failed:", classifyError(err))
		os.Exit(1)
	}
}

func run() error {
	var afterID, upperID uint64
	var acceptedBeforeRaw, detailReport string
	var pageSize, maxPages int
	var connectionsStdin bool
	flag.Uint64Var(&afterID, "after-id", 0, "exclusive AnswerSheet ID cursor")
	flag.Uint64Var(&upperID, "upper-id", 0, "inclusive fixed AnswerSheet ID upper bound")
	flag.StringVar(&acceptedBeforeRaw, "accepted-before", "", "inclusive RFC3339 acceptance cutoff")
	flag.StringVar(&detailReport, "detail-report", "", "new restricted JSON file for up to 20 actionable IDs")
	flag.IntVar(&pageSize, "page-size", 100, "rows per page, 1..500")
	flag.IntVar(&maxPages, "max-pages", 1, "maximum pages, 1..20")
	flag.BoolVar(&connectionsStdin, "connections-stdin", false, "read connection JSON from stdin instead of environment")
	flag.Parse()
	if flag.NArg() != 0 || upperID == 0 || upperID <= afterID || pageSize < 1 || pageSize > 500 || maxPages < 1 || maxPages > 20 || os.Getenv("RM_QS_GAP_READ_ONLY") != "1" {
		return errors.New("invalid_bounds")
	}
	if detailReport != "" && (!filepath.IsAbs(detailReport) || filepath.Clean(detailReport) != detailReport) {
		return errors.New("invalid_detail_path")
	}
	acceptedBefore, err := time.Parse(time.RFC3339Nano, acceptedBeforeRaw)
	if err != nil || acceptedBefore.After(time.Now().Add(-10*time.Minute)) {
		return errors.New("invalid_cutoff")
	}
	acceptedBefore = acceptedBefore.UTC()
	mysqlDSN := os.Getenv("RM_QS_GAP_MYSQL_DSN")
	mongoURI := os.Getenv("RM_QS_GAP_MONGO_URI")
	mongoDBName := os.Getenv("RM_QS_GAP_MONGO_DB")
	if connectionsStdin {
		decoder := json.NewDecoder(io.LimitReader(os.Stdin, 16385))
		decoder.DisallowUnknownFields()
		var input connectionInput
		if err := decoder.Decode(&input); err != nil {
			return errors.New("invalid_connection_input")
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return errors.New("invalid_connection_input")
		}
		mysqlDSN, mongoURI, mongoDBName = input.MySQLDSN, input.MongoURI, input.MongoDBName
	}
	parsed, err := mysqldriver.ParseDSN(mysqlDSN)
	if err != nil || parsed.DBName == "" || mongoURI == "" || mongoDBName == "" {
		return errors.New("invalid_connections")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(5*time.Second))
	if err != nil {
		return errors.New("mongo_connect")
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		return errors.New("mongo_ping")
	}
	sqlDB, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return errors.New("mysql_open")
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(0)
	if err := sqlDB.PingContext(ctx); err != nil {
		return errors.New("mysql_ping")
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return errors.New("mysql_adapter")
	}
	readOnly := db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true})
	if readOnly.Error != nil {
		return errors.New("mysql_read_only_transaction")
	}
	defer func() { _ = readOnly.Rollback().Error }()
	scanner, err := answersheetgap.New(mongoClient.Database(mongoDBName), readOnly)
	if err != nil {
		return errors.New("scanner_setup")
	}
	result := report{
		AfterID: afterID, UpperID: upperID, AcceptedBefore: acceptedBefore,
		NextAfterID: afterID, Counts: map[string]int{},
	}
	actionable := make([]findingSample, 0, 20)
	for result.Pages < maxPages && !result.Complete {
		page, scanErr := scanner.ScanPage(ctx, result.NextAfterID, upperID, acceptedBefore, pageSize)
		if scanErr != nil {
			return errors.New("scan_page")
		}
		result.Pages++
		result.Scanned += len(page.Findings)
		if len(page.Findings) > 0 {
			result.NextAfterID = page.NextID
		}
		result.Complete = page.Exhausted
		for _, finding := range page.Findings {
			result.Counts[string(finding.Disposition)]++
			if finding.Disposition == answersheetgap.Missing || finding.Disposition == answersheetgap.Unknown || finding.Disposition == answersheetgap.ManualRequired {
				if detailReport != "" && len(actionable) < 20 {
					actionable = append(actionable, findingSample{
						AnswerSheetID: finding.AnswerSheetID, EventID: finding.EventID,
						Disposition: finding.Disposition, Reason: finding.Reason,
					})
				}
			}
		}
	}
	if detailReport != "" {
		file, err := os.OpenFile(detailReport, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return errors.New("detail_file_create")
		}
		if err := json.NewEncoder(file).Encode(actionable); err != nil {
			_ = file.Close()
			return errors.New("detail_file_write")
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return errors.New("detail_file_sync")
		}
		if err := file.Close(); err != nil {
			return errors.New("detail_file_close")
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return errors.New("report_write")
	}
	return nil
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}
	return err.Error()
}
