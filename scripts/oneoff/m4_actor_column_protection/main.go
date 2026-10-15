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
	action, capture, material, materialHash, approval, approvalHash string
	captureEvidence, captureEvidenceHash                            string
	runtimeIntent, runtimeIntentHash                                string
	binding                                                         binding
	timeout                                                         time.Duration
}
type receipt struct {
	Action                         string                 `json:"action"`
	ToolSource                     string                 `json:"compiled_tool_source"`
	CSource                        string                 `json:"c_execution_source"`
	CResources                     string                 `json:"c_execution_resources_sha256"`
	CResourceFiles                 int                    `json:"c_execution_resource_files"`
	Complete                       bool                   `json:"complete"`
	ReadOnly                       bool                   `json:"database_read_only"`
	MaterialHash                   string                 `json:"material_sha256,omitempty"`
	TableCounts                    map[string]int         `json:"table_rows,omitempty"`
	MandatoryEOF                   bool                   `json:"mandatory_empty_eof"`
	SQLSnapshot                    bool                   `json:"sql_repeatable_read_snapshot"`
	SQLConnectionClosed            bool                   `json:"sql_connection_closed"`
	SQLPoolClosed                  bool                   `json:"sql_pool_closed"`
	MongoClosed                    bool                   `json:"mongo_closed"`
	Error                          string                 `json:"error_category"`
	ReleaseError                   string                 `json:"release_error_category,omitempty"`
	ProductionFreezeCertified      bool                   `json:"production_freeze_certified"`
	CrossDatabaseAtomicRR          bool                   `json:"cross_database_atomic_snapshot"`
	BudgetExceeded                 bool                   `json:"total_budget_exceeded"`
	TransactionReleaseUnproven     bool                   `json:"transaction_release_unproven"`
	RecoveryGateAcquired           bool                   `json:"recovery_gate_acquired,omitempty"`
	RecoveryGateReleased           bool                   `json:"recovery_gate_released,omitempty"`
	RecoveryGateReleaseUnproven    bool                   `json:"recovery_gate_release_unproven,omitempty"`
	ApprovalConsumed               bool                   `json:"approval_consumed"`
	DatabaseBoundary               *runtimeBoundaryState  `json:"runtime_database_boundary,omitempty"`
	DatabaseBoundaryRecord         *runtimeBoundaryRecord `json:"runtime_database_record,omitempty"`
	DatabaseBoundaryAdvanced       bool                   `json:"runtime_database_boundary_advanced,omitempty"`
	DatabaseBoundaryAdvanceUnknown bool                   `json:"runtime_database_boundary_advance_unknown,omitempty"`
	MarkerHash                     string                 `json:"runtime_marker_sha256,omitempty"`
	MarkerWritten                  bool                   `json:"runtime_marker_written,omitempty"`
	MarkerWriteUnknown             bool                   `json:"runtime_marker_write_unknown,omitempty"`
}

func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("m4-actor-column-protection", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.action, "action", "inspect", "inspect, capture, verify-target, restore, verify-restored, mark-first-runtime")
	f.StringVar(&a.binding.SQLName, "mysql-database", "", "explicit selected database")
	f.StringVar(&a.binding.MongoName, "mongo-database", "", "explicit selected database")
	f.StringVar(&a.binding.SQLIdentity, "mysql-identity-sha256", "", "independent target identity")
	f.StringVar(&a.binding.MongoIdentity, "mongo-identity-sha256", "", "independent target identity")
	f.StringVar(&a.binding.MongoUUID, "mongo-ledger-uuid-sha256", "", "independent post-B ledger UUID")
	f.StringVar(&a.capture, "capture-file", "", "new private first capture")
	f.StringVar(&a.material, "material-file", "", "private original material")
	f.StringVar(&a.materialHash, "material-sha256", "", "independently frozen original bytes hash")
	f.StringVar(&a.captureEvidence, "capture-evidence-file", "", "independently frozen original capture execution evidence")
	f.StringVar(&a.captureEvidenceHash, "capture-evidence-sha256", "", "independent capture execution evidence raw SHA")
	f.StringVar(&a.approval, "approval-file", "", "independent pre-first-runtime restore approval")
	f.StringVar(&a.approvalHash, "approval-sha256", "", "independent approval raw SHA")
	f.StringVar(&a.runtimeIntent, "runtime-intent-file", "", "independent first-runtime intent")
	f.StringVar(&a.runtimeIntentHash, "runtime-intent-sha256", "", "independent intent original SHA")
	f.DurationVar(&a.timeout, "timeout", 20*time.Minute, "finite total deadline, at most20m")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return a, errors.New("arguments_invalid")
	}
	captureEvidenceProvided, runtimeIntentProvided := false, false
	f.Visit(func(option *flag.Flag) {
		if option.Name == "runtime-intent-file" || option.Name == "runtime-intent-sha256" {
			runtimeIntentProvided = true
		}
		if option.Name == "capture-evidence-file" || option.Name == "capture-evidence-sha256" {
			captureEvidenceProvided = true
		}
	})
	if !validBinding(a.binding) || a.timeout <= 0 || a.timeout > 20*time.Minute {
		return a, errors.New("external_binding_or_deadline_required")
	}
	if a.action != "mark-first-runtime" && runtimeIntentProvided {
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
	case "verify-target", "restore", "verify-restored":
		if a.capture != "" || !absolutePath(a.material) || !nonzeroHash(a.materialHash) || a.action == "verify-target" && (a.approval != "" || a.approvalHash != "") || (a.action == "restore" || a.action == "verify-restored") && (!absolutePath(a.approval) || !nonzeroHash(a.approvalHash)) {
			return a, errors.New("independently_bound_material_required")
		}
	case "mark-first-runtime":
		if a.capture != "" || !absolutePath(a.material) || !nonzeroHash(a.materialHash) || !absolutePath(a.runtimeIntent) || !nonzeroHash(a.runtimeIntentHash) || (a.approval != "" || a.approvalHash != "") && (!absolutePath(a.approval) || !nonzeroHash(a.approvalHash)) {
			return a, errors.New("runtime_independent_intent_required")
		}
	default:
		return a, errors.New("action_invalid")
	}
	if a.action == "inspect" || a.action == "capture" {
		if captureEvidenceProvided {
			return a, errors.New("capture_evidence_action_mismatch")
		}
	} else if !absolutePath(a.captureEvidence) || !nonzeroHash(a.captureEvidenceHash) || a.captureEvidence == a.material {
		return a, errors.New("original_capture_evidence_required")
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

// The action already includes the CLI deadline and, for restore, its earlier
// independent approval deadline. Only aggregate reads/DDL may use this finite
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
	r = receipt{ToolSource: tool, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, Error: "arguments_invalid", ReadOnly: true}
	a, e := parseArgs(args)
	r.Action, r.ReadOnly = a.action, a.action != "restore" && a.action != "mark-first-runtime"
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
	var intentApproval *approval
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
		if e != nil {
			r.Error = e.Error()
			return r
		}
	}
	if a.action == "restore" || a.action == "verify-restored" || a.action == "mark-first-runtime" && intent.Mode == "restored101" {
		raw, e := readPrivateLimit(a.approval, a.approvalHash, 32768)
		if e != nil || len(raw) > 32768 || strictDecode(raw, &ap) != nil || validateApproval(ap, original, a.materialHash, tool, time.Now()) != nil {
			r.Error = "restore_approval_rejected"
			return r
		}
		if e := checkRecoveryBoundary(original, ap, a.materialHash, false); e != nil {
			r.Error = e.Error()
			return r
		}
		var c context.CancelFunc
		ctx, c = restoreActionContext(ctx, ap)
		defer c()
		if a.action == "verify-restored" || a.action == "mark-first-runtime" {
			if e := verifyConsumedRecoveryInputs(ctx, original, ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool); e != nil {
				r.Error = e.Error()
				return r
			}
		}
	}
	if a.action == "mark-first-runtime" {
		if intent.Mode == "restored101" {
			intentApproval = &ap
		} else if a.approval != "" || a.approvalHash != "" {
			r.Error = "runtime_unexpected_recovery_approval"
			return r
		}
		if e := rereadRuntimeInputs(ctx, intent, original, a, tool, intentApproval); e != nil {
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
			}
		}()
	}

	dsn, e := mysql.ParseDSN(getenv("QS_M4_C_OBJECT_MYSQL_DSN"))
	uri := getenv("QS_M4_C_OBJECT_MONGO_URI")
	if e != nil || dsn.DBName != a.binding.SQLName || uri == "" {
		r.Error = "selected_connection_input_rejected"
		return r
	}
	if !configureMySQLAction(ctx, dsn) {
		r.Error = "mysql_action_deadline_required"
		return r
	}
	if a.action == "restore" {
		// Original server JSON bytes may contain four-byte UTF-8, independent
		// of the old application connection charset. Do not parse/reencode them.
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
			if ok {
				if a.action == "mark-first-runtime" {
					if e := d.requireCompletedRuntimeMark(ctx, original, intent, a, tool, intentApproval, &r); e != nil {
						r.Error, ok = e.Error(), false
					}
				} else if e := d.requireCapturedRuntimeBoundary(ctx, original); e != nil {
					r.Error, ok = e.Error(), false
				} else if e := verifyConsumedRecoveryInputs(ctx, original, ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool); e != nil {
					r.Error, ok = e.Error(), false
				}
			}
			if e := d.gate.release(); e != nil {
				r.ReleaseError = e.Error()
				if ok {
					r.Error = e.Error()
				}
				released = false
			}
			r.RecoveryGateAcquired, r.RecoveryGateReleased, r.RecoveryGateReleaseUnproven = d.gate.acquired, d.gate.released, d.gate.releaseUnproven
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
		if a.action == "verify-target" || a.action == "restore" || a.action == "verify-restored" || a.action == "mark-first-runtime" {
			if e := readCaptureEvidence(original, a.materialHash, tool, a.captureEvidence, a.captureEvidenceHash); e != nil {
				r.Error = e.Error()
				return
			}
			if ctx.Err() != nil {
				r.Error, r.BudgetExceeded = "total_budget_exceeded", true
				return
			}
		}
		if a.action == "verify-restored" {
			if e := verifyConsumedRecoveryInputs(ctx, original, ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool); e != nil {
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
	if a.action == "restore" || a.action == "verify-restored" || a.action == "mark-first-runtime" {
		d.gate, e = acquireRecoveryGate(ctx, d.conn, a.binding)
		if e != nil {
			r.Error = e.Error()
			return r
		}
		if a.action == "verify-restored" {
			if e := verifyConsumedRecoveryInputs(ctx, original, ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool); e != nil {
				r.Error = e.Error()
				return r
			}
		}
	}
	if a.action == "mark-first-runtime" {
		e = d.markFirstRuntime(ctx, original, intent, a, tool, intentApproval, &r)
		if e != nil {
			r.Error = e.Error()
			var release *transactionReleaseFailure
			r.TransactionReleaseUnproven = errors.As(e, &release)
			return r
		}
		r.SQLSnapshot, r.MandatoryEOF = true, true
		r.TableCounts = map[string]int{}
		for _, t := range original.Tables {
			r.TableCounts[t.Table] = len(t.Rows)
		}
		ok = true
		return r
	}
	if a.action == "restore" {
		e = d.restore(ctx, original, ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool, &r.ApprovalConsumed)
		if e != nil {
			r.Error = e.Error()
			var release *transactionReleaseFailure
			r.TransactionReleaseUnproven = errors.As(e, &release)
			return r
		}
		r.SQLSnapshot, r.MandatoryEOF = true, true
		r.TableCounts = map[string]int{}
		for _, t := range original.Tables {
			r.TableCounts[t.Table] = len(t.Rows)
		}
		ok = true
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
	if a.action == "verify-restored" && verifyRestored(original, m) != nil {
		r.Error = "restored_original_facts_rejected"
		return r
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
