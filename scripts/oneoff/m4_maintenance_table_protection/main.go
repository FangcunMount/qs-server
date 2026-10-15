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
	action, capture, material, materialHash, approval, approvalHash string
	runtimeIntent, runtimeIntentHash                                string
	captureEvidence, captureEvidenceHash                            string
	binding                                                         binding
	timeout                                                         time.Duration
}
type receipt struct {
	Action                     string         `json:"action"`
	ToolSource                 string         `json:"compiled_tool_source"`
	DSource                    string         `json:"d_execution_source"`
	DResources                 string         `json:"d_execution_resources_sha256"`
	DResourceFiles             int            `json:"d_execution_resource_files"`
	Complete                   bool           `json:"complete"`
	ReadOnly                   bool           `json:"database_read_only"`
	MaterialHash               string         `json:"material_sha256,omitempty"`
	TableCounts                map[string]int `json:"table_rows,omitempty"`
	RetainedCounts             map[string]int `json:"retained_table_rows,omitempty"`
	RetainedEOF                bool           `json:"retained_mandatory_empty_eof"`
	MandatoryEOF               bool           `json:"mandatory_empty_eof"`
	SQLSnapshot                bool           `json:"sql_repeatable_read_snapshot"`
	SQLConnectionClosed        bool           `json:"sql_connection_closed"`
	SQLPoolClosed              bool           `json:"sql_pool_closed"`
	MongoClosed                bool           `json:"mongo_closed"`
	Error                      string         `json:"error_category"`
	ReleaseError               string         `json:"release_error_category,omitempty"`
	ProductionFreezeCertified  bool           `json:"production_freeze_certified"`
	CrossDatabaseAtomicRR      bool           `json:"cross_database_atomic_snapshot"`
	BudgetExceeded             bool           `json:"total_budget_exceeded"`
	TransactionReleaseUnproven bool           `json:"transaction_release_unproven"`
	ApprovalConsumed           bool           `json:"approval_consumed"`
	RuntimeGateAcquired        bool           `json:"runtime_gate_acquired"`
	RuntimeGateReleased        bool           `json:"runtime_gate_released"`
	RuntimeGateReleaseUnproven bool           `json:"runtime_gate_release_unproven"`
	MarkerWritten              bool           `json:"runtime_marker_written"`
	MarkerHash                 string         `json:"runtime_marker_sha256,omitempty"`
	MarkerWriteUnknown         bool           `json:"runtime_marker_write_unknown"`
}

func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("m4-maintenance-table-protection", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.action, "action", "inspect", "inspect, capture, verify-target, restore, verify-restored, mark-first-runtime")
	f.StringVar(&a.binding.SQLName, "mysql-database", "", "explicit selected database")
	f.StringVar(&a.binding.MongoName, "mongo-database", "", "explicit selected database")
	f.StringVar(&a.binding.SQLIdentity, "mysql-identity-sha256", "", "independent target identity")
	f.StringVar(&a.binding.MongoIdentity, "mongo-identity-sha256", "", "independent target identity")
	f.StringVar(&a.binding.MongoUUID, "mongo-ledger-uuid-sha256", "", "independent post-C ledger UUID")
	f.StringVar(&a.capture, "capture-file", "", "new private first capture")
	f.StringVar(&a.material, "material-file", "", "private original material")
	f.StringVar(&a.materialHash, "material-sha256", "", "independently frozen original bytes hash")
	f.StringVar(&a.captureEvidence, "capture-evidence-file", "", "independently frozen original capture execution evidence")
	f.StringVar(&a.captureEvidenceHash, "capture-evidence-sha256", "", "independent capture execution evidence raw SHA")
	f.StringVar(&a.approval, "approval-file", "", "independent pre-first-runtime restore approval")
	f.StringVar(&a.approvalHash, "approval-sha256", "", "independent approval raw SHA")
	f.StringVar(&a.runtimeIntent, "runtime-intent-file", "", "independent private first-runtime intent")
	f.StringVar(&a.runtimeIntentHash, "runtime-intent-sha256", "", "independent first-runtime intent raw SHA")
	f.DurationVar(&a.timeout, "timeout", maxAction, "finite total deadline, at most20m")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return a, errors.New("arguments_invalid")
	}
	evidenceSupplied := false
	f.Visit(func(f *flag.Flag) {
		if f.Name == "capture-evidence-file" || f.Name == "capture-evidence-sha256" {
			evidenceSupplied = true
		}
	})
	if !validBinding(a.binding) || a.timeout <= 0 || a.timeout > maxAction {
		return a, errors.New("external_binding_or_deadline_required")
	}
	if a.action != "mark-first-runtime" && (a.runtimeIntent != "" || a.runtimeIntentHash != "") {
		return a, errors.New("runtime_intent_action_mismatch")
	}
	switch a.action {
	case "inspect":
		if a.capture != "" || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("inspect_rejects_material_inputs")
		}
	case "capture":
		if a.capture == "" || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("capture_arguments_invalid")
		}
		if filepath.Base(a.capture) == "first-runtime" {
			return a, errors.New("capture_runtime_marker_path_conflict")
		}
	case "verify-target", "restore", "verify-restored":
		if a.capture != "" || !absolutePath(a.material) || !nonzeroHash(a.materialHash) || a.action == "verify-target" && (a.approval != "" || a.approvalHash != "") || a.action != "verify-target" && (!absolutePath(a.approval) || !nonzeroHash(a.approvalHash)) {
			return a, errors.New("independently_bound_material_required")
		}
	case "mark-first-runtime":
		if a.capture != "" || !absolutePath(a.material) || !nonzeroHash(a.materialHash) || !absolutePath(a.runtimeIntent) || !nonzeroHash(a.runtimeIntentHash) || (a.approval == "") != (a.approvalHash == "") || a.approval != "" && (!absolutePath(a.approval) || !nonzeroHash(a.approvalHash)) {
			return a, errors.New("runtime_independent_inputs_required")
		}
	default:
		return a, errors.New("action_invalid")
	}
	if a.action == "inspect" || a.action == "capture" {
		if evidenceSupplied {
			return a, errors.New("capture_evidence_action_mismatch")
		}
	} else if !absolutePath(a.captureEvidence) || !nonzeroHash(a.captureEvidenceHash) || a.captureEvidence == a.material {
		return a, errors.New("original_capture_evidence_required")
	}
	return a, nil
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	if serve(ctx, os.Args[1:], os.Getenv, version.GitCommit, os.Stdout) != 0 {
		os.Exit(1)
	}
}
func serve(ctx context.Context, args []string, getenv func(string) string, tool string, out io.Writer) int {
	return emitReceipt(run(ctx, args, getenv, tool), out)
}

func emitReceipt(r receipt, out io.Writer) int {
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

// The action includes the CLI deadline. Only terminal aggregate reads use this finite
// socket budget; metadata and page reads retain their 5s child contexts.
func configureMySQLAction(ctx context.Context, dsn *mysql.Config) bool {
	deadline, bounded := ctx.Deadline()
	remaining := time.Until(deadline)
	if !bounded || ctx.Err() != nil || remaining <= 0 {
		return false
	}
	dsn.MultiStatements, dsn.ParseTime = false, false
	dsn.Timeout, dsn.ReadTimeout, dsn.WriteTimeout = queryLimit, remaining, queryLimit
	return true
}

func run(parent context.Context, args []string, getenv func(string) string, tool string) (r receipt) {
	r = receipt{ToolSource: tool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, Error: "arguments_invalid", ReadOnly: true}
	a, e := parseArgs(args)
	r.Action, r.ReadOnly = a.action, a.action != "restore"
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
	var ap approval
	var intent runtimeIntent
	if a.action == "verify-target" || a.action == "restore" || a.action == "verify-restored" || a.action == "mark-first-runtime" {
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
		if e := readCaptureEvidence(original, a.materialHash, tool, a.captureEvidence, a.captureEvidenceHash); e != nil {
			r.Error = e.Error()
			return r
		}
	}
	if a.action == "mark-first-runtime" {
		intent, e = readRuntimeIntent(a.runtimeIntent, a.runtimeIntentHash)
		if e != nil || intent.Mode == "restored102" && a.approval == "" || intent.Mode != "restored102" && a.approval != "" {
			r.Error = "runtime_independent_intent_rejected"
			return r
		}
	}
	if a.action == "restore" || a.action == "verify-restored" || a.action == "mark-first-runtime" && intent.Mode == "restored102" {
		raw, e := readPrivateLimit(a.approval, a.approvalHash, 32768)
		if e != nil || strictDecode(raw, &ap) != nil || validateApproval(ap, original, a.materialHash, tool, time.Now()) != nil {
			r.Error = "restore_approval_rejected"
			return r
		}
		if a.action == "restore" {
			e = checkRecoveryBoundary(original, ap, a.materialHash, false)
		} else {
			e = checkRestoredAuthorization(original, ap, a.material, a.materialHash, a.approval, a.approvalHash, tool)
		}
		if e != nil {
			r.Error = e.Error()
			return r
		}
		var c context.CancelFunc
		ctx, c = restoreActionContext(ctx, ap)
		defer c()
	}
	var intentApproval *approval
	if a.action == "mark-first-runtime" {
		if intent.Mode == "restored102" {
			intentApproval = &ap
		}
		if e := validateRuntimeIntent(intent, original, a.materialHash, tool, intentApproval, time.Now()); e != nil {
			r.Error = e.Error()
			return r
		}
		var c context.CancelFunc
		ctx, c = runtimeIntentContext(ctx, intent)
		defer c()
	}
	var captureDirectory captureParentIdentity
	if a.action == "capture" {
		if !absolutePath(a.capture) {
			r.Error = "capture_private_path_rejected"
			return r
		}
		captureDirectory, e = captureParent(a.capture)
		if e != nil {
			r.Error = "capture_private_parent_rejected"
			return r
		}
		defer func() {
			if e := captureDirectory.close(); e != nil {
				if r.ReleaseError == "" {
					r.ReleaseError = e.Error()
				}
				if r.Complete || r.Error == "none" {
					r.Error = e.Error()
				}
				r.Complete = false
				r.MaterialHash = ""
				r.TableCounts = nil
				r.RetainedCounts = nil
				r.RetainedEOF = false
			}
		}()
	}

	dsn, e := mysql.ParseDSN(getenv("QS_M4_D_OBJECT_MYSQL_DSN"))
	uri := getenv("QS_M4_D_OBJECT_MONGO_URI")
	if e != nil || dsn.DBName != a.binding.SQLName || uri == "" {
		r.Error = "selected_connection_input_rejected"
		return r
	}
	if !configureMySQLAction(ctx, dsn) {
		r.Error = "mysql_action_deadline_required"
		return r
	}
	if a.action == "restore" {
		if dsn.Params == nil {
			dsn.Params = map[string]string{}
		}
		dsn.Params["charset"] = "utf8mb4"
		dsn.Collation = "utf8mb4_bin"
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
	d := database{binding: a.binding}
	ok := false
	var rawMaterial []byte
	defer func() {
		released := true
		if d.gate != nil {
			if e := d.gate.release(); e != nil {
				r.ReleaseError = e.Error()
				if ok {
					r.Error = e.Error()
				}
			}
			r.RuntimeGateAcquired, r.RuntimeGateReleased, r.RuntimeGateReleaseUnproven = d.gate.acquired, d.gate.released, d.gate.releaseUnproven
			released = released && !r.RuntimeGateReleaseUnproven
		}
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
		released = released && !r.TransactionReleaseUnproven
		if !released {
			if r.ReleaseError == "" {
				r.ReleaseError = "owned_connection_release_unproven"
			}
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
		if a.action != "inspect" && a.action != "capture" {
			if e := readCaptureEvidence(original, a.materialHash, tool, a.captureEvidence, a.captureEvidenceHash); e != nil {
				r.Error = e.Error()
				return
			}
		}
		if a.action == "restore" || a.action == "verify-restored" {
			if e := checkRestoredAuthorization(original, ap, a.material, a.materialHash, a.approval, a.approvalHash, tool); e != nil {
				r.Error = e.Error()
				return
			}
		}
		if a.action == "capture" {
			if e := finishCapture(ctx, &r, a.capture, &captureDirectory, rawMaterial, writeCaptureExclusive); e != nil {
				r.Error = strings.Split(e.Error(), "\n")[0]
				return
			}
			if ctx.Err() != nil {
				r.Error, r.BudgetExceeded = "total_budget_exceeded", true
				return
			}
		}
		if ctx.Err() != nil {
			r.Error = "total_budget_exceeded"
			r.BudgetExceeded = true
			return
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
	if a.action == "restore" || a.action == "mark-first-runtime" {
		d.gate, e = acquireRuntimeGate(ctx, d.conn, a.binding)
		if e != nil {
			r.Error = e.Error()
			return r
		}
	}
	if a.action == "mark-first-runtime" {
		if e := d.markFirstRuntime(ctx, original, intent, a, tool, intentApproval, &r); e != nil {
			r.Error = strings.Split(e.Error(), "\n")[0]
			return r
		}
		r.SQLSnapshot, r.RetainedEOF, r.MandatoryEOF = true, true, intent.Mode != "target103"
		ok = true
		return r
	}
	if a.action == "restore" {
		e = d.restore(ctx, original, ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool, &r.ApprovalConsumed)
		if e != nil {
			r.Error = strings.Split(e.Error(), "\n")[0]
			var released *transactionReleaseFailure
			if errors.As(e, &released) {
				r.TransactionReleaseUnproven = true
				r.ReleaseError = "transaction_release_unproven"
			}
			return r
		}
		r.SQLSnapshot = true
		r.MandatoryEOF, r.RetainedEOF = true, true
		r.TableCounts, r.RetainedCounts = map[string]int{}, map[string]int{}
		for _, t := range original.Tables {
			r.TableCounts[t.Table] = len(t.Rows)
		}
		for _, t := range original.Retained {
			r.RetainedCounts[t.Table] = len(t.Rows)
		}
		ok = true
		return r
	}
	version := uint64(102)
	if a.action == "verify-target" {
		version = 103
	}
	m, e := d.snapshot(ctx, tool, version, a.action != "inspect")
	if e != nil {
		r.Error = strings.Split(e.Error(), "\n")[0]
		if strings.Contains(e.Error(), "readonly_transaction_release_unproven") {
			r.ReleaseError = "readonly_transaction_release_unproven"
			r.TransactionReleaseUnproven = true
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
	if a.action == "verify-restored" && verifyRestored(original, m) != nil {
		r.Error = "original_restoration_proof_rejected"
		return r
	}
	if a.action == "verify-target" && verifyTarget(original, m) != nil {
		r.Error = "target_object_or_identity_rejected"
		return r
	}
	r.SQLSnapshot = true
	if a.action != "inspect" {
		r.RetainedEOF = true
		r.RetainedCounts = map[string]int{}
		for _, t := range m.Retained {
			r.RetainedCounts[t.Table] = len(t.Rows)
		}
	}
	if a.action == "capture" || a.action == "verify-restored" {
		r.MandatoryEOF = true
		r.TableCounts = map[string]int{}
		for _, t := range m.Tables {
			if t.Table != "" {
				r.TableCounts[t.Table] = len(t.Rows)
			}
		}
	}
	ok = true
	return r
}
