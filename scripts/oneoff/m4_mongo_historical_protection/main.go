package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type arguments struct {
	action, sqlName, mongoName, capture, material, materialHash, approval, approvalHash string
	timeout                                                                             time.Duration
}
type receipt struct {
	Action                     string    `json:"action,omitempty"`
	ToolSource                 string    `json:"compiled_tool_source,omitempty"`
	BSource                    string    `json:"b_execution_source"`
	BResources                 string    `json:"b_execution_resources_sha256"`
	BResourceFiles             int       `json:"b_execution_resource_files"`
	Complete                   bool      `json:"complete"`
	ReadOnly                   bool      `json:"database_read_only"`
	FreezeCertified            bool      `json:"production_freeze_certified"`
	MaterialHash               string    `json:"material_sha256,omitempty"`
	SQLIdentity                string    `json:"mysql_identity_sha256,omitempty"`
	MongoIdentity              string    `json:"mongo_identity_sha256,omitempty"`
	SQLVersion                 uint64    `json:"mysql_clean_version,omitempty"`
	MongoVersion               int64     `json:"mongo_clean_version,omitempty"`
	Counts                     [2]*int64 `json:"exact_fixed_collection_counts"`
	SQLConnectionClosed        *bool     `json:"sql_connection_closed"`
	SQLPoolClosed              *bool     `json:"sql_pool_closed"`
	MongoClosed                *bool     `json:"mongo_closed"`
	MongoCursorsClosed         *bool     `json:"mongo_cursors_exhausted_and_closed"`
	LedgerCommitIssued         bool      `json:"ledger_commit_issued"`
	LedgerCommitAcknowledged   bool      `json:"ledger_commit_acknowledged"`
	LedgerCommitOutcomeUnknown bool      `json:"ledger_commit_outcome_unknown"`
	LedgerAbortCalled          bool      `json:"application_ledger_abort_called"`
	SessionEndCalled           bool      `json:"mongo_snapshot_session_end_called"`
	TransactionReleaseUnproven bool      `json:"sql_transaction_release_unproven"`
	Error                      string    `json:"error_category"`
	ReleaseError               string    `json:"release_error_category,omitempty"`
	BudgetExceeded             bool      `json:"total_budget_exceeded"`
}

func absolutePath(s string) bool { return filepath.IsAbs(s) && filepath.Clean(s) == s }
func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("m4-mongo-historical-protection", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.action, "action", "inspect", "inspect, capture, verify-target, restore")
	f.StringVar(&a.sqlName, "mysql-database", "", "explicit selected database")
	f.StringVar(&a.mongoName, "mongo-database", "", "explicit selected database")
	f.StringVar(&a.capture, "capture-file", "", "first private capture path")
	f.StringVar(&a.material, "material-file", "", "independently frozen private material")
	f.StringVar(&a.materialHash, "material-sha256", "", "externally frozen raw bytes hash")
	f.StringVar(&a.approval, "approval-file", "", "independent pre-start restore approval")
	f.StringVar(&a.approvalHash, "approval-sha256", "", "independent approval raw bytes hash")
	f.DurationVar(&a.timeout, "timeout", 15*time.Minute, "finite total deadline, at most20m")
	if f.Parse(args) != nil || f.NArg() != 0 || !nameRE.MatchString(a.sqlName) || !nameRE.MatchString(a.mongoName) || a.timeout <= 0 || a.timeout > 20*time.Minute {
		return a, errors.New("arguments_invalid")
	}
	switch a.action {
	case "inspect":
		if a.capture != "" || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("inspect_rejects_write_inputs")
		}
	case "capture":
		if !absolutePath(a.capture) || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("capture_arguments_invalid")
		}
	case "verify-target", "restore":
		if a.capture != "" || !absolutePath(a.material) || !nonzeroHash(a.materialHash) || a.action == "restore" && (!absolutePath(a.approval) || !nonzeroHash(a.approvalHash)) || a.action == "verify-target" && (a.approval != "" || a.approvalHash != "") {
			return a, errors.New("bound_material_and_action_required")
		}
	default:
		return a, errors.New("action_invalid")
	}
	return a, nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if serve(ctx, os.Args[1:], os.Getenv, version.GitCommit, os.Stdout) != 0 {
		os.Exit(1)
	}
}
func serve(ctx context.Context, args []string, getenv func(string) string, tool string, out io.Writer) int {
	r := run(ctx, args, getenv, tool)
	if json.NewEncoder(out).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}
func boolean(v bool) *bool { return &v }
func closeSQLPool(db *sql.DB) bool {
	e := db.Close()
	s := db.Stats()
	return e == nil && s.OpenConnections == 0 && s.InUse == 0
}

func run(parent context.Context, args []string, getenv func(string) string, tool string) (r receipt) {
	r = receipt{BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, Error: "arguments_invalid"}
	a, e := parseArgs(args)
	if e != nil || !sourceRE.MatchString(tool) {
		return r
	}
	r.Action, r.ToolSource, r.ReadOnly = a.action, tool, a.action != "restore"
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	var m material
	var ap approval
	if a.action == "verify-target" || a.action == "restore" {
		raw, e := readPrivate(a.material, a.materialHash, maxMaterialBytes)
		if e != nil {
			r.Error = e.Error()
			return r
		}
		if strictDecode(raw, &m) != nil || validateMaterial(m, tool, a.sqlName, a.mongoName) != nil {
			r.Error = "material_rejected"
			return r
		}
		r.MaterialHash = a.materialHash
	}
	if a.action == "restore" {
		raw, e := readPrivate(a.approval, a.approvalHash, 32768)
		if e != nil {
			r.Error = e.Error()
			return r
		}
		if strictDecode(raw, &ap) != nil || validateApproval(ap, m, a.materialHash, tool, time.Now()) != nil {
			r.Error = "restore_approval_rejected"
			return r
		}
		deadline, _ := time.Parse(time.RFC3339Nano, ap.Deadline)
		expiry, _ := time.Parse(time.RFC3339Nano, ap.Expires)
		if expiry.Before(deadline) {
			deadline = expiry
		}
		var c context.CancelFunc
		ctx, c = context.WithDeadline(ctx, deadline)
		defer c()
	}
	if a.action == "capture" {
		if s, e := privatePath(a.capture, false); e != nil || s != nil {
			r.Error = "first_capture_exists_or_path_rejected"
			return r
		}
	}
	dsn, e := mysql.ParseDSN(getenv("QS_M4_OBJECT_MYSQL_DSN"))
	uri := getenv("QS_M4_OBJECT_MONGO_URI")
	if e != nil || dsn.DBName != a.sqlName || uri == "" {
		r.Error = "selected_connection_input_rejected"
		return r
	}
	dsn.MultiStatements = false
	dsn.ParseTime = false
	dsn.Timeout = queryLimit
	dsn.ReadTimeout = queryLimit
	dsn.WriteTimeout = queryLimit
	if mysql.SetLogger(&mysql.NopLogger{}) != nil {
		r.Error = "mysql_logging_boundary_failed"
		return r
	}
	db, e := sql.Open("mysql", dsn.FormatDSN())
	if e != nil {
		r.Error = "mysql_pool_open_failed"
		return r
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	d := database{sqlName: a.sqlName, mongoName: a.mongoName}
	ok := false
	var rawMaterial []byte
	defer func() {
		defer func() {
			if !r.Complete {
				r.Counts = [2]*int64{}
			}
		}()
		if d.conn != nil {
			r.SQLConnectionClosed = boolean(d.conn.Close() == nil)
		}
		r.SQLPoolClosed = boolean(closeSQLPool(db))
		if d.mongo != nil {
			q, c := context.WithTimeout(context.Background(), queryLimit)
			r.MongoClosed = boolean(d.mongo.Disconnect(q) == nil)
			c()
		}
		if d.cursorCount > 0 {
			r.MongoCursorsClosed = boolean(!d.cursorReleaseUnproven)
		}
		r.LedgerCommitIssued = d.ledgerCommitIssued
		r.LedgerCommitAcknowledged = d.ledgerCommitAcknowledged
		r.LedgerCommitOutcomeUnknown = d.ledgerCommitUnknown
		r.LedgerAbortCalled = d.ledgerAbortCalled
		r.SessionEndCalled = d.sessionEndCalled
		r.TransactionReleaseUnproven = d.sqlRollbackUnproven
		r.BudgetExceeded = ctx.Err() != nil
		released := (r.SQLConnectionClosed == nil || *r.SQLConnectionClosed) && *r.SQLPoolClosed && (r.MongoClosed == nil || *r.MongoClosed) && !d.cursorReleaseUnproven && !d.sqlRollbackUnproven
		if !released {
			r.ReleaseError = "owned_resource_release_unproven"
			if ok {
				r.Error = r.ReleaseError
			}
			return
		}
		if !ok || ctx.Err() != nil {
			r.BudgetExceeded = ctx.Err() != nil
			if ok {
				r.Error = "total_budget_exceeded"
			}
			return
		}
		if a.action == "capture" {
			if e := finishCapture(ctx, &r, a.capture, rawMaterial, writeExclusive); e != nil {
				r.BudgetExceeded = ctx.Err() != nil
				r.Error = e.Error()
				return
			}
		}
		r.Complete = true
		r.Error = "none"
	}()
	q, c := context.WithTimeout(ctx, queryLimit)
	d.conn, e = db.Conn(q)
	c()
	if e != nil {
		r.Error = "mysql_connection_failed"
		return r
	}
	// Operation contexts, not a client-wide timeout, bound each fixed read.
	// This prevents a future approved createIndexes wait being silently capped
	// by a URI timeoutMS/socketTimeoutMS or retried by the driver.
	d.mongo, e = mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(1).SetReadPreference(readpref.Primary()).SetConnectTimeout(queryLimit).SetServerSelectionTimeout(queryLimit).SetTimeout(0).SetSocketTimeout(0).SetRetryReads(false).SetRetryWrites(false))
	if e != nil {
		r.Error = "mongo_connect_failed"
		return r
	}
	q, c = context.WithTimeout(ctx, queryLimit)
	e = d.mongo.Ping(q, readpref.Primary())
	c()
	if e != nil {
		r.Error = "mongo_primary_unavailable"
		return r
	}
	var o observation
	switch a.action {
	case "inspect", "capture":
		at := time.Now()
		o, e = d.snapshot(ctx)
		if e == nil {
			m = makeMaterial(o, tool, a.sqlName, a.mongoName, at)
			e = validateMaterial(m, tool, a.sqlName, a.mongoName)
		}
		if e == nil && a.action == "capture" {
			rawMaterial, e = json.Marshal(m)
			if e != nil || len(rawMaterial) > maxMaterialBytes {
				e = errors.New("material_encoding_or_size_failed")
			}
		}
	case "verify-target":
		o, e = d.verifyTarget(ctx, m)
	case "restore":
		o, e = d.restore(ctx, m, ap, a.material, a.materialHash, a.approval, a.approvalHash, tool)
	}
	if e != nil {
		r.Error = e.Error()
		return r
	}
	r.SQLIdentity, r.MongoIdentity, r.SQLVersion = o.SQLIdentity, o.MongoIdentity, o.SQLVersion
	r.MongoVersion, _, _ = ledgerVersion(o.Ledger.Document)
	r.Counts = [2]*int64{o.Collections[0].Count, o.Collections[1].Count}
	ok = true
	return r
}
func finishCapture(ctx context.Context, r *receipt, path string, raw []byte, write func(string, []byte) error) error {
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	if e := write(path, raw); e != nil {
		return e
	}
	r.MaterialHash = fingerprint(raw)
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	return nil
}
