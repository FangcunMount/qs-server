// Reuse the existing application snapshot without exporting identities or errors.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/mysqltls"
	mysql "github.com/go-sql-driver/mysql"
)

func main() {
	report := map[string]any{"read_only": true, "mutation": false, "available": false}
	defer func() { _ = json.NewEncoder(os.Stdout).Encode(report) }()
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr = "tcp", os.Getenv("QS_APISERVER_MYSQL_HOST")
	cfg.User, cfg.Passwd = os.Getenv("QS_APISERVER_MYSQL_USERNAME"), os.Getenv("QS_APISERVER_MYSQL_PASSWORD")
	cfg.DBName, cfg.ParseTime, cfg.Loc = os.Getenv("QS_APISERVER_MYSQL_DATABASE"), true, time.UTC
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 2*time.Second, 2*time.Second, 2*time.Second
	if cfg.Addr == "" || cfg.User == "" || cfg.DBName == "" || os.Getenv("QS_APISERVER_MYSQL_TLS_MODE") != "VERIFY_IDENTITY" {
		report["category"] = "configuration_unavailable"
		return
	}
	connector, err := mysqltls.NewConnector(cfg)
	if err != nil {
		report["category"] = "tls_configuration"
		return
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	_, err = store.MessagingSnapshot(ctx, db)
	report["elapsed_ms"] = time.Since(started).Milliseconds()
	switch {
	case err == nil:
		report["available"], report["category"] = true, "available"
	case errors.Is(err, store.ErrMessagingLedgerIntegrity):
		report["category"] = "ledger_evidence_integrity"
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		report["category"] = "observation_budget_exhausted"
	default:
		var driverError *mysql.MySQLError
		if errors.As(err, &driverError) {
			report["category"], report["mysql_error_number"] = "mysql_error", driverError.Number
		} else {
			report["category"] = "connection_or_query_error"
		}
	}
}
