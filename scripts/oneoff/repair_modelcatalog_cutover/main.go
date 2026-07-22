// repair_modelcatalog_cutover canonicalizes current active ModelCatalog
// snapshots after historical runtime data has been removed.
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
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	exitOK          = 0
	exitUnavailable = 1
	exitBlocked     = 2
)

type config struct {
	mysqlDSN string
	mongoURI string
	mongoDB  string
	apply    bool
	jsonOut  bool
	timeout  time.Duration
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	cfg, err := parseConfig(args, stderr, getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, "repair modelcatalog cutover:", err)
		return exitUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	mysqlDB, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: open mysql:", err)
		return exitUnavailable
	}
	defer func() { _ = mysqlDB.Close() }()
	if err := mysqlDB.PingContext(ctx); err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: ping mysql:", err)
		return exitUnavailable
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: connect mongo:", err)
		return exitUnavailable
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: ping mongo:", err)
		return exitUnavailable
	}

	db := client.Database(cfg.mongoDB)
	plan, err := buildRepairPlan(ctx, mysqlDB, db)
	if err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: build plan:", err)
		return exitUnavailable
	}
	plan.Mode = "dry-run"
	if cfg.apply {
		plan.Mode = "apply"
	}
	if err := writePlan(stdout, plan, cfg.jsonOut); err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: write plan:", err)
		return exitUnavailable
	}
	if plan.Blocked() {
		return exitBlocked
	}
	if !cfg.apply {
		return exitOK
	}

	if err := requireWritableReplicaSet(ctx, client); err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: apply guard:", err)
		return exitUnavailable
	}
	if err := applyRepairPlan(ctx, client, db, plan); err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: apply:", err)
		return exitUnavailable
	}
	if err := verifyAppliedRepair(ctx, mysqlDB, db, plan); err != nil {
		fmt.Fprintln(stderr, "repair modelcatalog cutover: post-apply verification:", err)
		return exitUnavailable
	}
	fmt.Fprintln(stdout, "MODELCATALOG_CUTOVER_REPAIR_OK")
	return exitOK
}

func parseConfig(args []string, stderr io.Writer, getenv func(string) string) (config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := config{}
	flags := flag.NewFlagSet("repair_modelcatalog_cutover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.mysqlDSN, "mysql-dsn", getenv("MYSQL_DSN"), "MySQL DSN (required for history-empty guard)")
	flags.StringVar(&cfg.mongoURI, "mongo-uri", getenv("MONGO_URI"), "MongoDB URI")
	flags.StringVar(&cfg.mongoDB, "mongo-db", envOr(getenv, "MONGO_DB", "qs_server"), "MongoDB database")
	flags.BoolVar(&cfg.apply, "apply", false, "apply the printed repair plan atomically")
	flags.BoolVar(&cfg.jsonOut, "json", false, "emit a machine-readable repair plan")
	flags.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "command timeout")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if cfg.mysqlDSN == "" {
		return config{}, fmt.Errorf("--mysql-dsn or MYSQL_DSN is required")
	}
	if cfg.mongoURI == "" {
		return config{}, fmt.Errorf("--mongo-uri or MONGO_URI is required")
	}
	if cfg.mongoDB == "" {
		return config{}, fmt.Errorf("--mongo-db is required")
	}
	if cfg.timeout <= 0 {
		return config{}, fmt.Errorf("--timeout must be positive")
	}
	return cfg, nil
}

func writePlan(w io.Writer, plan *repairPlan, jsonOut bool) error {
	if jsonOut {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	_, err := fmt.Fprint(w, plan.Text())
	return err
}

func envOr(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}
