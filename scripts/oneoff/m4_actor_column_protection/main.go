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
	"strings"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type arguments struct {
	action, capture, material, materialHash string
	binding                                 binding
	timeout                                 time.Duration
}
type receipt struct {
	Action                    string         `json:"action"`
	ToolSource                string         `json:"compiled_tool_source"`
	CSource                   string         `json:"c_execution_source"`
	CResources                string         `json:"c_execution_resources_sha256"`
	CResourceFiles            int            `json:"c_execution_resource_files"`
	Complete                  bool           `json:"complete"`
	ReadOnly                  bool           `json:"database_read_only"`
	MaterialHash              string         `json:"material_sha256,omitempty"`
	TableCounts               map[string]int `json:"table_rows,omitempty"`
	MandatoryEOF              bool           `json:"mandatory_empty_eof"`
	SQLSnapshot               bool           `json:"sql_repeatable_read_snapshot"`
	SQLConnectionClosed       bool           `json:"sql_connection_closed"`
	SQLPoolClosed             bool           `json:"sql_pool_closed"`
	MongoClosed               bool           `json:"mongo_closed"`
	Error                     string         `json:"error_category"`
	ReleaseError              string         `json:"release_error_category,omitempty"`
	ProductionFreezeCertified bool           `json:"production_freeze_certified"`
	CrossDatabaseAtomicRR     bool           `json:"cross_database_atomic_snapshot"`
	BudgetExceeded            bool           `json:"total_budget_exceeded"`
	ApprovalConsumed          bool           `json:"approval_consumed"`
}

func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("m4-actor-column-protection", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.action, "action", "inspect", "inspect, capture, verify-target; restore unsupported")
	f.StringVar(&a.binding.SQLName, "mysql-database", "", "explicit selected database")
	f.StringVar(&a.binding.MongoName, "mongo-database", "", "explicit selected database")
	f.StringVar(&a.binding.SQLIdentity, "mysql-identity-sha256", "", "independent target identity")
	f.StringVar(&a.binding.MongoIdentity, "mongo-identity-sha256", "", "independent target identity")
	f.StringVar(&a.binding.MongoUUID, "mongo-ledger-uuid-sha256", "", "independent post-B ledger UUID")
	f.StringVar(&a.capture, "capture-file", "", "new private first capture")
	f.StringVar(&a.material, "material-file", "", "private original material")
	f.StringVar(&a.materialHash, "material-sha256", "", "independently frozen original bytes hash")
	f.DurationVar(&a.timeout, "timeout", 20*time.Minute, "finite total deadline, at most20m")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return a, errors.New("arguments_invalid")
	}
	// No approval flags are accepted and no input/configuration is read for a
	// restore request. There is no restore or migration implementation here.
	if a.action == "restore" {
		return a, errors.New("restore_not_implemented")
	}
	if !validBinding(a.binding) || a.timeout <= 0 || a.timeout > 20*time.Minute {
		return a, errors.New("external_binding_or_deadline_required")
	}
	switch a.action {
	case "inspect":
		if a.capture != "" || a.material != "" || a.materialHash != "" {
			return a, errors.New("inspect_rejects_material_inputs")
		}
	case "capture":
		if a.capture == "" || a.material != "" || a.materialHash != "" {
			return a, errors.New("capture_arguments_invalid")
		}
	case "verify-target":
		if a.capture != "" || a.material == "" || !hashRE.MatchString(a.materialHash) {
			return a, errors.New("independently_bound_material_required")
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

func mongoReadOptions(uri string) *options.ClientOptions {
	// Ping's preference is local to that probe. Override the URI for every
	// subsequent ledger/catalog read as well; hello separately proves the role.
	return options.Client().ApplyURI(uri).SetReadPreference(readpref.Primary()).SetMaxPoolSize(1).SetServerSelectionTimeout(queryLimit).SetTimeout(queryLimit).SetRetryReads(false).SetRetryWrites(false)
}

func run(parent context.Context, args []string, getenv func(string) string, tool string) (r receipt) {
	r = receipt{ToolSource: tool, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, Error: "arguments_invalid", ReadOnly: true}
	a, e := parseArgs(args)
	r.Action = a.action
	if e != nil {
		r.Error = e.Error()
		return r
	}
	if !sourceRE.MatchString(tool) {
		r.Error = "compiled_tool_source_required"
		return r
	}
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	var original material
	if a.action == "verify-target" {
		raw, e := readPrivate(a.material, a.materialHash)
		if e != nil {
			r.Error = strings.Split(e.Error(), "\n")[0]
			return r
		}
		if strictDecode(raw, &original) != nil || validateMaterial(original, tool, a.binding) != nil || original.CapturePath != a.material {
			r.Error = "original_material_rejected"
			return r
		}
		r.MaterialHash = a.materialHash
	}
	if a.action == "capture" {
		if !absolutePath(a.capture) {
			r.Error = "capture_private_path_rejected"
			return r
		}
		parent, e := privateParent(a.capture)
		if e != nil {
			r.Error = "capture_private_parent_rejected"
			return r
		}
		if parent.Close() != nil {
			r.Error = "capture_private_parent_release_unproven"
			return r
		}
	}
	dsn, e := mysql.ParseDSN(getenv("QS_M4_C_OBJECT_MYSQL_DSN"))
	uri := getenv("QS_M4_C_OBJECT_MONGO_URI")
	if e != nil || dsn.DBName != a.binding.SQLName || uri == "" {
		r.Error = "selected_connection_input_rejected"
		return r
	}
	dsn.MultiStatements, dsn.ParseTime = false, false
	dsn.Timeout, dsn.ReadTimeout, dsn.WriteTimeout = queryLimit, queryLimit, queryLimit
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
	d := database{binding: a.binding}
	ok := false
	var rawMaterial []byte
	defer func() {
		released := true
		if d.conn != nil {
			r.SQLConnectionClosed = d.conn.Close() == nil
			released = released && r.SQLConnectionClosed
		}
		closed := db.Close() == nil
		stats := db.Stats()
		r.SQLPoolClosed = closed && stats.OpenConnections == 0 && stats.InUse == 0
		released = released && r.SQLPoolClosed
		if d.mongo != nil {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), queryLimit)
			r.MongoClosed = d.mongo.Disconnect(closeCtx) == nil
			closeCancel()
			released = released && r.MongoClosed
		}
		if !released {
			r.ReleaseError = "owned_connection_release_unproven"
			if ok {
				r.Error = r.ReleaseError
			}
			return
		}
		if ctx.Err() != nil {
			r.BudgetExceeded = true
			if ok {
				r.Error = "total_budget_exceeded"
			}
			return
		}
		if !ok {
			return
		}
		if a.action == "capture" {
			if e := writeExclusive(a.capture, rawMaterial); e != nil {
				r.Error = strings.Split(e.Error(), "\n")[0]
				return
			}
			if ctx.Err() != nil {
				r.Error, r.BudgetExceeded = "total_budget_exceeded", true
				return
			}
			r.MaterialHash = fingerprint(rawMaterial)
		}
		r.Complete, r.Error = true, "none"
	}()
	q, stop := context.WithTimeout(ctx, queryLimit)
	d.conn, e = db.Conn(q)
	stop()
	if e != nil {
		r.Error = "mysql_connection_failed"
		return r
	}
	q, stop = context.WithTimeout(ctx, queryLimit)
	_, e = d.conn.ExecContext(q, "SET SESSION time_zone='+00:00',character_set_results='utf8mb4',sql_mode='"+sessionMode+"'")
	stop()
	if e != nil {
		r.Error = "session_binding_failed"
		return r
	}
	d.mongo, e = mongo.Connect(ctx, mongoReadOptions(uri))
	if e != nil {
		r.Error = "mongo_connection_failed"
		return r
	}
	q, stop = context.WithTimeout(ctx, queryLimit)
	e = d.mongo.Ping(q, readpref.Primary())
	stop()
	if e != nil {
		r.Error = "mongo_primary_unavailable"
		return r
	}
	version := uint64(101)
	if a.action == "verify-target" {
		version = 102
	}
	m, e := d.snapshot(ctx, tool, version, a.action != "inspect")
	if e != nil {
		r.Error = strings.Split(e.Error(), "\n")[0]
		if strings.Contains(e.Error(), "readonly_transaction_release_unproven") {
			r.ReleaseError = "readonly_transaction_release_unproven"
		}
		return r
	}
	if a.action == "capture" {
		m.CapturePath = a.capture
		if validateMaterial(m, tool, a.binding) != nil {
			r.Error = "captured_material_rejected"
			return r
		}
		rawMaterial, e = json.Marshal(m)
		if e != nil || len(rawMaterial) > maxMaterialBytes {
			r.Error = "material_encoding_limit"
			return r
		}
	}
	if a.action == "verify-target" && verifyTarget(original, m) != nil {
		r.Error = "target_retained_facts_rejected"
		return r
	}
	if a.action != "inspect" {
		r.SQLSnapshot, r.MandatoryEOF = true, true
		r.TableCounts = map[string]int{}
		for _, t := range m.Tables {
			r.TableCounts[t.Table] = len(t.Rows)
		}
	}
	ok = true
	return r
}
