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
	Action                     string `json:"action"`
	ToolSource                 string `json:"compiled_tool_source"`
	ASource                    string `json:"a_execution_source"`
	AResources                 string `json:"a_execution_resources_sha256"`
	AResourceFiles             int    `json:"a_execution_resource_files"`
	Complete                   bool   `json:"complete"`
	ReadOnly                   bool   `json:"database_read_only"`
	MaterialHash               string `json:"material_sha256,omitempty"`
	SQLConnectionClosed        bool   `json:"sql_connection_closed"`
	SQLPoolClosed              bool   `json:"sql_pool_closed"`
	MongoClosed                bool   `json:"mongo_closed"`
	Error                      string `json:"error_category"`
	ReleaseError               string `json:"release_error_category,omitempty"`
	TransactionReleaseUnproven bool   `json:"transaction_release_unproven"`
	ProductionFreezeCertified  bool   `json:"production_freeze_certified"`
	BudgetExceeded             bool   `json:"total_budget_exceeded"`
}

func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("m4-sql-historical-protection", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.action, "action", "inspect", "inspect, capture, verify-target, restore")
	f.StringVar(&a.sqlName, "mysql-database", "", "explicit selected database")
	f.StringVar(&a.mongoName, "mongo-database", "", "explicit selected database")
	f.StringVar(&a.capture, "capture-file", "", "first private capture path")
	f.StringVar(&a.material, "material-file", "", "independently frozen private material")
	f.StringVar(&a.materialHash, "material-sha256", "", "externally approved original bytes hash")
	f.StringVar(&a.approval, "approval-file", "", "independent pre-start restore authorization")
	f.StringVar(&a.approvalHash, "approval-sha256", "", "independent authorization bytes hash")
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
		if a.capture == "" || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("capture_arguments_invalid")
		}
	case "verify-target", "restore":
		if a.capture != "" || a.material == "" || !hashRE.MatchString(a.materialHash) || (a.action == "restore" && (a.approval == "" || !hashRE.MatchString(a.approvalHash))) || (a.action == "verify-target" && (a.approval != "" || a.approvalHash != "")) {
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

// The action context already includes the CLI/parent deadline. Restore narrows
// it again to its independently approved deadline and expiry before connecting.
// Metadata and page reads keep their shorter query contexts. A complete COUNT
// or ALTER response may need longer, but never an unbounded socket wait.
func mysqlActionReadTimeout(ctx context.Context) (time.Duration, bool) {
	deadline, bounded := ctx.Deadline()
	if !bounded || ctx.Err() != nil {
		return 0, false
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0, false
	}
	return remaining, true
}

func configureMySQLAction(ctx context.Context, dsn *mysql.Config) bool {
	budget, bounded := mysqlActionReadTimeout(ctx)
	if !bounded {
		return false
	}
	dsn.MultiStatements = false
	dsn.ParseTime = false
	dsn.Timeout = queryLimit
	dsn.ReadTimeout = budget
	dsn.WriteTimeout = queryLimit
	return true
}

func restoreActionContext(ctx context.Context, a approval) (context.Context, context.CancelFunc) {
	deadline, _ := time.Parse(time.RFC3339Nano, a.Deadline)
	expiry, _ := time.Parse(time.RFC3339Nano, a.Expires)
	if expiry.Before(deadline) {
		deadline = expiry
	}
	return context.WithDeadline(ctx, deadline)
}

func mongoReadOptions(uri string) *options.ClientOptions {
	// Ping's preference applies only to that probe. Ledger/catalog reads must
	// also override a URI that asks for a secondary or nearest server.
	return options.Client().ApplyURI(uri).SetReadPreference(readpref.Primary()).SetMaxPoolSize(1).SetServerSelectionTimeout(queryLimit).SetTimeout(queryLimit)
}

func serve(ctx context.Context, args []string, getenv func(string) string, tool string, out io.Writer) int {
	return deliverReceipt(run(ctx, args, getenv, tool, nil), out)
}
func deliverReceipt(r receipt, out io.Writer) int {
	if json.NewEncoder(out).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}

// The CLI always supplies nil. The observer receives only a fixed object index
// after ExecContext has received a successful ADD acknowledgment.
func run(parent context.Context, args []string, getenv func(string) string, tool string, afterRestoreDDL func(int)) (r receipt) {
	r = receipt{ToolSource: tool, ASource: aSource, AResources: aResources, AResourceFiles: aResourceFiles, Error: "arguments_invalid"}
	a, e := parseArgs(args)
	r.Action = a.action
	r.ReadOnly = a.action != "restore"
	if e != nil || !sourceRE.MatchString(tool) {
		return r
	}
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
		if strictDecode(raw, &m) != nil || validateMaterial(m, tool, a.sqlName, a.mongoName) != nil || m.CapturePath != a.material {
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
		if e := checkRecoveryBoundary(m, ap, a.materialHash, false); e != nil {
			r.Error = e.Error()
			return r
		}
		var c context.CancelFunc
		ctx, c = restoreActionContext(ctx, ap)
		defer c()
	}
	if a.action == "capture" {
		if e := recoveryParent(a.capture); e != nil {
			r.Error = e.Error()
			return r
		}
	}
	dsn, e := mysql.ParseDSN(getenv("QS_M4_OBJECT_MYSQL_DSN"))
	if e != nil || dsn.DBName != a.sqlName || getenv("QS_M4_OBJECT_MONGO_URI") == "" {
		r.Error = "selected_connection_input_rejected"
		return r
	}
	if !configureMySQLAction(ctx, dsn) {
		r.Error = "mysql_connection_failed"
		r.BudgetExceeded = ctx.Err() != nil
		return r
	}
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
	d := database{sqlName: a.sqlName, mongoName: a.mongoName, afterRestoreDDL: afterRestoreDDL}
	var rawMaterial []byte
	ok := false
	defer func() {
		if d.conn != nil {
			r.SQLConnectionClosed = d.conn.Close() == nil
		}
		r.SQLPoolClosed = closeSQLPool(db)
		if d.mongo != nil {
			closeCtx, c := context.WithTimeout(context.Background(), queryLimit)
			r.MongoClosed = d.mongo.Disconnect(closeCtx) == nil
			c()
		}
		if !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
			r.ReleaseError = "owned_connection_release_unproven"
			if ok {
				r.Error = r.ReleaseError
			}
			return
		}
		if !ok || ctx.Err() != nil {
			if ctx.Err() != nil {
				r.BudgetExceeded = true
				if ok {
					r.Error = "total_budget_exceeded"
				}
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
	connectContext, cancelConnect := context.WithTimeout(ctx, queryLimit)
	d.conn, e = db.Conn(connectContext)
	cancelConnect()
	if e != nil {
		r.Error = "mysql_connection_failed"
		return r
	}
	sessionContext, cancelSession := context.WithTimeout(ctx, queryLimit)
	_, e = d.conn.ExecContext(sessionContext, "SET SESSION time_zone='+00:00', character_set_results='utf8mb4',sql_mode='"+sessionMode+"'")
	cancelSession()
	if e != nil {
		r.Error = "session_binding_failed"
		return r
	}
	d.mongo, e = mongo.Connect(ctx, mongoReadOptions(getenv("QS_M4_OBJECT_MONGO_URI")))
	if e != nil {
		r.Error = "mongo_connect_failed"
		return r
	}
	if d.mongo.Ping(ctx, readpref.Primary()) != nil {
		r.Error = "mongo_primary_unavailable"
		return r
	}
	switch a.action {
	case "inspect":
		var o observation
		o, e = d.observe(ctx, d.conn)
		if e == nil && (o.Version != 100 || o.Dirty) {
			e = errors.New("inspect_requires_clean10039")
		}
	case "capture":
		m, _, e = d.snapshot(ctx, tool, true)
		if e == nil {
			m.CapturePath = a.capture
			e = validateMaterial(m, tool, a.sqlName, a.mongoName)
		}
		if e == nil {
			rawMaterial, e = json.Marshal(m)
		}
	case "verify-target":
		e = d.verifyTarget(ctx, m, tool)
	case "restore":
		e = d.restore(ctx, m, ap, tool, a.material, a.materialHash, a.approval, a.approvalHash)
	}
	if e != nil {
		r.TransactionReleaseUnproven = hasTransactionReleaseFailure(e)
		r.Error = e.Error()
		return r
	}
	ok = true
	return r
}

func finishCapture(ctx context.Context, r *receipt, path string, raw []byte, write func(string, []byte) error) error {
	if e := write(path, raw); e != nil {
		return e
	}
	r.MaterialHash = fingerprint(raw)
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	return nil
}

func hasTransactionReleaseFailure(e error) bool {
	var release *transactionReleaseFailure
	return errors.As(e, &release)
}

func closeSQLPool(db *sql.DB) bool {
	e := db.Close()
	s := db.Stats()
	return e == nil && s.OpenConnections == 0 && s.InUse == 0
}
