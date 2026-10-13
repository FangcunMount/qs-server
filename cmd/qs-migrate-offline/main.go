// qs-migrate-offline owns only database connections and a reviewed migration run.
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
	"os/signal"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/pkg/version"
	mysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type arguments struct {
	mysqlDatabase, mongoDatabase, approvalFile, approvalSHA256 string
	apply                                                      bool
	timeout                                                    time.Duration
}

func parseArguments(args []string) (arguments, error) {
	var out arguments
	flags := flag.NewFlagSet("qs-migrate-offline", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&out.mysqlDatabase, "mysql-database", "", "selected MySQL database")
	flags.StringVar(&out.mongoDatabase, "mongo-database", "", "selected Mongo database")
	flags.StringVar(&out.approvalFile, "approval-file", "", "private reviewed offline-upgrade JSON")
	flags.StringVar(&out.approvalSHA256, "approval-sha256", "", "approved raw file digest")
	flags.BoolVar(&out.apply, "apply", false, "consume approval and execute embedded migrations")
	flags.DurationVar(&out.timeout, "timeout", 15*time.Minute, "total connection/migration deadline, at most 30m")
	if err := flags.Parse(args); err != nil {
		return out, err
	}
	if flags.NArg() != 0 || out.mysqlDatabase == "" || out.mongoDatabase == "" || out.timeout <= 0 || out.timeout > 30*time.Minute ||
		(out.apply && (out.approvalFile == "" || out.approvalSHA256 == "")) ||
		(!out.apply && (out.approvalFile != "" || out.approvalSHA256 != "")) {
		return out, errors.New("explicit selected databases and bounded offline action required")
	}
	return out, nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		// Provider errors, connection strings and approval contents stay private.
		fmt.Fprintln(os.Stderr, "offline migration refused; preserve evidence and remain stopped")
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, getenv func(string) string, output io.Writer) (resultErr error) {
	a, err := parseArguments(args)
	if err != nil {
		return err
	}
	if !validBuildIdentity(version.GitCommit) {
		return errors.New("compiled source identity required")
	}
	dsn, err := mysql.ParseDSN(getenv("QS_OFFLINE_MYSQL_DSN"))
	if err != nil || dsn.DBName != a.mysqlDatabase {
		return errors.New("selected MySQL binding rejected")
	}
	uri := getenv("QS_OFFLINE_MONGO_URI")
	if uri == "" {
		return errors.New("explicit Mongo binding required")
	}
	// Credentials never become flags/output. Only the host opens/closes pools.
	dsn.MultiStatements = true
	dsn.Timeout, dsn.ReadTimeout, dsn.WriteTimeout = 15*time.Second, 15*time.Second, 15*time.Second
	db, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	db.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(2).SetServerSelectionTimeout(15*time.Second).SetTimeout(15*time.Second))
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancelCleanup := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelCleanup()
		resultErr = errors.Join(resultErr, client.Disconnect(cleanup))
	}()
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return err
	}
	pair := migration.PairConfig{MySQLDatabase: a.mysqlDatabase, MongoDatabase: a.mongoDatabase, ExpectedSourceSHA: version.GitCommit}
	if !a.apply {
		snapshot, err := migration.InspectOffline(ctx, db, client, pair)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(struct {
			ReadOnly bool                      `json:"read_only"`
			Snapshot migration.OfflineSnapshot `json:"snapshot"`
		}{true, snapshot})
	}
	result, err := migration.RunOffline(ctx, db, client, migration.OfflineConfig{
		Pair: pair, AuthorizationFile: a.approvalFile, AuthorizationSHA256: a.approvalSHA256,
	})
	return errors.Join(err, json.NewEncoder(output).Encode(result))
}

func validBuildIdentity(source string) bool {
	if len(source) != 40 {
		return false
	}
	for _, c := range source {
		if (c < 'a' || c > 'f') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
