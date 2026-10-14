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
	Action                    string `json:"action"`
	ToolSource                string `json:"compiled_tool_source"`
	ASource                   string `json:"a_execution_source"`
	AResources                string `json:"a_execution_resources_sha256"`
	AResourceFiles            int    `json:"a_execution_resource_files"`
	Complete                  bool   `json:"complete"`
	ReadOnly                  bool   `json:"database_read_only"`
	MaterialHash              string `json:"material_sha256,omitempty"`
	SQLConnectionClosed       bool   `json:"sql_connection_closed"`
	SQLPoolClosed             bool   `json:"sql_pool_closed"`
	MongoClosed               bool   `json:"mongo_closed"`
	Error                     string `json:"error_category"`
	ReleaseError              string `json:"release_error_category,omitempty"`
	ProductionFreezeCertified bool   `json:"production_freeze_certified"`
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
	r := run(ctx, os.Args[1:], os.Getenv, version.GitCommit)
	_ = json.NewEncoder(os.Stdout).Encode(r)
	if !r.Complete {
		os.Exit(1)
	}
}
func run(parent context.Context, args []string, getenv func(string) string, tool string) (r receipt) {
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
	dsn, e := mysql.ParseDSN(getenv("QS_M4_OBJECT_MYSQL_DSN"))
	if e != nil || dsn.DBName != a.sqlName || getenv("QS_M4_OBJECT_MONGO_URI") == "" {
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
	var rawMaterial []byte
	ok := false
	defer func() {
		if d.conn != nil {
			r.SQLConnectionClosed = d.conn.Close() == nil
		}
		r.SQLPoolClosed = db.Close() == nil
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
				r.Error = "total_budget_exceeded"
			}
			return
		}
		if a.action == "capture" {
			if e := finishCapture(ctx, &r, a.capture, rawMaterial, writeExclusive); e != nil {
				r.Error = e.Error()
				return
			}
		}

		r.Complete = true
		r.Error = "none"
	}()
	d.conn, e = db.Conn(ctx)
	if e != nil {
		r.Error = "mysql_connection_failed"
		return r
	}
	if _, e = d.conn.ExecContext(ctx, "SET SESSION time_zone='+00:00', character_set_results='utf8mb4',sql_mode='"+sessionMode+"'"); e != nil {
		r.Error = "session_binding_failed"
		return r
	}
	d.mongo, e = mongo.Connect(ctx, options.Client().ApplyURI(getenv("QS_M4_OBJECT_MONGO_URI")).SetMaxPoolSize(1).SetServerSelectionTimeout(queryLimit).SetTimeout(queryLimit))
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
