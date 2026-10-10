package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	hostscope "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementhostscope"
)

type hostWriterScopeRequest struct {
	FormatVersion             int    `json:"format_version"`
	Kind                      string `json:"kind"`
	SourceSHA                 string `json:"source_sha"`
	OperationID               string `json:"operation_id"`
	ActualRunID               string `json:"actual_run_id"`
	HostRole                  string `json:"host_role"`
	TargetHash                string `json:"target_hash"`
	ObservationApprovalSHA256 string `json:"observation_approval_sha256"`
}
type hostWriterScopeReceipt struct {
	FormatVersion             int               `json:"format_version"`
	Kind                      string            `json:"kind"`
	Operation                 string            `json:"operation"`
	PrepareMode               string            `json:"prepare_mode"`
	SourceSHA                 string            `json:"source_sha"`
	OperationID               string            `json:"operation_id"`
	RunID                     string            `json:"run_id"`
	HostRole                  string            `json:"host_role"`
	SourceUID                 uint32            `json:"source_uid"`
	RequestSHA256             string            `json:"request_sha256"`
	ObservationApprovalSHA256 string            `json:"observation_approval_sha256"`
	TargetHash                string            `json:"target_hash"`
	Complete                  bool              `json:"complete"`
	DiagnosticOnly            bool              `json:"diagnostic_only"`
	ExecutionAllowed          bool              `json:"execution_allowed"`
	DropReady                 bool              `json:"drop_ready"`
	ObservationComplete       bool              `json:"host_observation_complete"`
	WriterScopeComplete       bool              `json:"writer_scope_complete"`
	MachineIDSHA256           string            `json:"observed_machine_id_sha256"`
	BootIDSHA256              string            `json:"observed_boot_id_sha256"`
	NamespaceSHA256           string            `json:"observed_namespace_sha256"`
	PrivateObservationSHA256  string            `json:"host_scope_private_observation_sha256"`
	CatalogSHA256             string            `json:"host_scope_catalog_sha256"`
	ProcessCount              int               `json:"process_count"`
	FileCount                 int               `json:"file_count"`
	EntryCount                int               `json:"entry_count"`
	ElapsedMillis             int64             `json:"observation_elapsed_millis"`
	Scopes                    []hostscope.Scope `json:"observed_scopes"`
	Unknown                   []string          `json:"unknown"`
	ErrorCategory             string            `json:"error_category"`
}

func hostWriterScopeBoundPath(path, op, run string) bool {
	return runRE.MatchString(op) && runRE.MatchString(run) && path == filepath.Join("/opt/backups/qs-server/compatibility-retirement", op, "host-writer-scope-request-"+run+".json")
}
func validateHostWriterScopeRequest(r hostWriterScopeRequest, op, run string) error {
	if r.FormatVersion != 1 || r.Kind != "readonly_host_writer_scope_request" || r.SourceSHA != sourceSHA || !shaRE.MatchString(sourceSHA) || r.OperationID != op || r.ActualRunID != run || !runRE.MatchString(op) || !runRE.MatchString(run) || r.HostRole != "server_a" || r.TargetHash != digest(targets) || !hashRE.MatchString(r.ObservationApprovalSHA256) {
		return lifecycleError("host_scope_binding_rejected")
	}
	return nil
}

// This root-once A adapter owns only its new private metadata. It neither opens
// database handles nor imports the observer's catalog as a stop/fence capability.
// D calls the shared observer from its existing bound root service session.
func runHostWriterScope(ctx context.Context, path, expected, op, run string) (r hostWriterScopeReceipt, result error) {
	started := time.Now()
	defer func() {
		r.ElapsedMillis = time.Since(started).Milliseconds()
		if ctx != nil && ctx.Err() != nil {
			r.ObservationComplete = false
			r.ErrorCategory = "host_scope_read_budget_exceeded"
			result = lifecycleError(r.ErrorCategory)
		}
	}()
	r = hostWriterScopeReceipt{FormatVersion: 1, Kind: "readonly_host_writer_scope_observation", HostRole: "server_a", Operation: "prepare", PrepareMode: "host-writer-scope", SourceSHA: sourceSHA, OperationID: op, RunID: run, RequestSHA256: expected, TargetHash: digest(targets), DiagnosticOnly: true, Scopes: []hostscope.Scope{}, Unknown: []string{}, ErrorCategory: "host_scope_incomplete"}
	uid, e := strconv.ParseUint(os.Getenv("QS_RETIREMENT_SOURCE_UID"), 10, 32)
	if e != nil || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || !hostWriterScopeBoundPath(path, op, run) || privateDir(lifecycleRootBatch(op, run)) != nil {
		r.ErrorCategory = "host_scope_root_once_required"
		return r, lifecycleError(r.ErrorCategory)
	}
	raw, e := readLifecycleOwnedBytes(path, expected, uint32(uid), 256<<10)
	if e != nil {
		r.ErrorCategory = "host_scope_request_read_rejected"
		return r, e
	}
	var request hostWriterScopeRequest
	if e = decodePrepareFacts(raw, &request); e != nil {
		r.ErrorCategory = "host_scope_schema_rejected"
		return r, e
	}
	if e = validateHostWriterScopeRequest(request, op, run); e != nil {
		r.ErrorCategory = "host_scope_binding_rejected"
		return r, e
	}
	r.HostRole = request.HostRole
	r.SourceUID = uint32(uid)
	r.ObservationApprovalSHA256 = request.ObservationApprovalSHA256
	observation, e := hostscope.Observe(ctx, hostscope.Binding{SourceSHA: sourceSHA, OperationID: op, RunID: run, HostRole: request.HostRole, RequestSHA256: expected, ApprovalSHA256: request.ObservationApprovalSHA256})
	r.MachineIDSHA256 = observation.MachineIDSHA256
	r.BootIDSHA256 = observation.BootIDSHA256
	r.NamespaceSHA256 = observation.NamespaceSHA256
	r.CatalogSHA256 = observation.CatalogSHA256()
	r.Scopes = observation.Scopes
	r.Unknown = observation.Unknown
	r.ProcessCount = len(observation.Processes)
	r.FileCount = len(observation.Files)
	r.EntryCount = len(observation.Entries)
	r.ElapsedMillis = observation.ElapsedMillis
	r.ObservationComplete = observation.ObservationComplete
	// Preserve actual partial facts too. Failure never overwrites/adopts an older
	// file and never purges unknown material. The raw body is already body-free.
	encoded, marshalErr := json.Marshal(observation)
	if marshalErr != nil || writeJSON(filepath.Join(lifecycleRootBatch(op, run), "host-writer-scope.private.json"), observation) != nil {
		r.ObservationComplete = false
		r.ErrorCategory = "host_scope_private_observation_write_failed"
		return r, lifecycleError(r.ErrorCategory)
	}
	r.PrivateObservationSHA256 = digestRaw(append(encoded, '\n'))
	if e != nil {
		r.ErrorCategory = "host_scope_observation_incomplete"
		return r, e
	}
	r.ErrorCategory = "none"
	return r, nil
}
