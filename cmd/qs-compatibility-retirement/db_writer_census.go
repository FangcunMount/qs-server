package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	dbcensus "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementdbcensus"
	"go.mongodb.org/mongo-driver/bson"
)

type dbCensusPrivate struct {
	SourceSHA           string                 `json:"source_sha"`
	OperationID         string                 `json:"operation_id"`
	RunID               string                 `json:"run_id"`
	RequestSHA256       string                 `json:"request_sha256"`
	IdentityProducer    prepareFactsProducer   `json:"identity_producer"`
	SQLIdentitySHA256   string                 `json:"mysql_identity_sha256"`
	MongoIdentitySHA256 string                 `json:"mongodb_identity_sha256"`
	MongoAnchorSHA256   string                 `json:"mongodb_namespace_anchor_sha256"`
	ObservedAt          string                 `json:"observed_at"`
	WriterScopeComplete bool                   `json:"writer_scope_complete"`
	SQL                 map[string][][]*string `json:"mysql"`
	Mongo               map[string][]bson.M    `json:"mongodb"`
	Sections            []dbcensus.Section     `json:"sections"`
	Gaps                []string               `json:"unknown"`
}

type dbCensusRequest struct {
	FormatVersion             int                  `json:"format_version"`
	Kind                      string               `json:"kind"`
	SourceSHA                 string               `json:"source_sha"`
	OperationID               string               `json:"operation_id"`
	ActualRunID               string               `json:"actual_run_id"`
	TargetHash                string               `json:"target_hash"`
	ObservationApprovalSHA256 string               `json:"observation_approval_sha256"`
	Identity                  prepareFactsProducer `json:"identity_report"`
}
type dbCensusReceipt struct {
	MySQLAllConnectionsPermissionProven   bool                 `json:"mysql_all_connections_permission_proven"`
	MongoLocalAllSessionsPermissionProven bool                 `json:"mongodb_local_all_sessions_permission_proven"`
	AllNodesSessionsCoverageComplete      bool                 `json:"all_nodes_sessions_coverage_complete"`
	ExternalWriterCoverageComplete        bool                 `json:"external_writer_coverage_complete"`
	FormatVersion                         int                  `json:"format_version"`
	Kind                                  string               `json:"kind"`
	Operation                             string               `json:"operation"`
	PrepareMode                           string               `json:"prepare_mode"`
	SourceSHA                             string               `json:"source_sha"`
	OperationID                           string               `json:"operation_id"`
	RunID                                 string               `json:"run_id"`
	SourceUID                             uint32               `json:"source_uid"`
	TargetHash                            string               `json:"target_hash"`
	RequestSHA256                         string               `json:"request_sha256"`
	ApprovalSHA256                        string               `json:"observation_approval_sha256"`
	Identity                              prepareFactsProducer `json:"observed_identity_producer"`
	SQLIdentitySHA256                     string               `json:"mysql_identity_sha256"`
	MongoIdentitySHA256                   string               `json:"mongodb_identity_sha256"`
	MongoAnchorSHA256                     string               `json:"mongodb_namespace_anchor_sha256"`
	SQLVersion                            uint64               `json:"mysql_migration_version"`
	MongoVersion                          uint64               `json:"mongodb_migration_version"`
	Complete                              bool                 `json:"complete"`
	DiagnosticOnly                        bool                 `json:"diagnostic_only"`
	ExecutionAllowed                      bool                 `json:"execution_allowed"`
	DropReady                             bool                 `json:"drop_ready"`
	ObservationComplete                   bool                 `json:"db_census_observation_complete"`
	WriterScopeComplete                   bool                 `json:"writer_scope_complete"`
	PrivateCatalogSHA256                  string               `json:"db_census_private_catalog_sha256"`
	CatalogSHA256                         string               `json:"db_census_catalog_sha256"`
	Sections                              []dbcensus.Section   `json:"observed_sections"`
	Gaps                                  []string             `json:"unknown"`
	ElapsedMillis                         int64                `json:"observation_elapsed_millis"`
	ErrorCategory                         string               `json:"error_category"`
}

func dbCensusRequestValid(r dbCensusRequest, op, run string) bool {
	return r.FormatVersion == 1 && r.Kind == "readonly_db_writer_census_request" && r.SourceSHA == sourceSHA && shaRE.MatchString(sourceSHA) && r.OperationID == op && r.ActualRunID == run && runRE.MatchString(op) && runRE.MatchString(run) && r.TargetHash == digest(targets) && hashRE.MatchString(r.ObservationApprovalSHA256) && r.Identity.OperationID == op && runRE.MatchString(r.Identity.RunID) && r.Identity.RunID != run && shaRE.MatchString(r.Identity.SourceSHA) && hashRE.MatchString(r.Identity.ReportSHA256) && hashRE.MatchString(r.Identity.RequestSHA256)
}
func dbCensusIdentityValid(reference prepareFactsProducer, report identityReport) bool {
	if report.FormatVersion != 1 || report.Kind != "readonly_identity_discovery" || report.SourceSHA != reference.SourceSHA || report.OperationID != reference.OperationID || report.RunID != reference.RunID || report.RequestHash != reference.RequestSHA256 || report.TargetHash != digest(targets) || !report.DiagnosticOnly || report.DropReady || !report.Complete || report.ErrorCategory != "none" || digest(report.Protocols) != digest(identityProtocols()) || len(report.States) != 2 {
		return false
	}
	for name, version := range map[string]uint64{"mysql": 99, "mongodb": 38} {
		state, ok := report.States[name]
		if !ok || !state.IdentityObserved || !state.HeadObserved || state.Dirty == nil || *state.Dirty || !state.Clean || !state.PermissionsSufficient || state.PermissionScope != "identity_and_migration_head" || state.Version != version || state.ErrorCategory != "none" || !hashRE.MatchString(state.IdentityHash) || !hashRE.MatchString(state.DatabaseAnchorHash) {
			return false
		}
	}
	mongo := report.States["mongodb"]
	return mongo.NamespaceAnchor == nil || (mongo.NamespaceAnchor.Validate() == nil && mongo.NamespaceAnchor.Hash == mongo.DatabaseAnchorHash)
}
func runDBWriterCensus(ctx context.Context, path, expected, op, run string) (receipt dbCensusReceipt, result error) {
	started := time.Now()
	receipt = dbCensusReceipt{FormatVersion: 1, Kind: "readonly_db_writer_census_observation", Operation: "prepare", PrepareMode: "db-writer-census", SourceSHA: sourceSHA, OperationID: op, RunID: run, RequestSHA256: expected, TargetHash: digest(targets), DiagnosticOnly: true, Sections: []dbcensus.Section{}, Gaps: []string{}, ErrorCategory: "db_census_incomplete"}
	defer func() {
		receipt.ElapsedMillis = time.Since(started).Milliseconds()
		if ctx == nil || ctx.Err() != nil {
			receipt.ObservationComplete = false
			receipt.ErrorCategory = "db_census_read_budget_exceeded"
			result = lifecycleError(receipt.ErrorCategory)
		}
	}()
	uid, e := strconv.ParseUint(os.Getenv("QS_RETIREMENT_SOURCE_UID"), 10, 32)
	if e != nil || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || !runRE.MatchString(op) || !runRE.MatchString(run) || path != filepath.Join("/opt/backups/qs-server/compatibility-retirement", op, "db-writer-census-request-"+run+".json") || privateDir(lifecycleRootBatch(op, run)) != nil {
		receipt.ErrorCategory = "db_census_root_once_required"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	raw, e := readLifecycleOwnedBytes(path, expected, uint32(uid), 256<<10)
	if e != nil {
		receipt.ErrorCategory = "db_census_request_read_rejected"
		return receipt, e
	}
	var request dbCensusRequest
	if decodePrepareFacts(raw, &request) != nil || !dbCensusRequestValid(request, op, run) {
		receipt.ErrorCategory = "db_census_request_binding_rejected"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	original := filepath.Join("/opt/backups/qs-server/compatibility-retirement", op, "identity-"+request.Identity.RunID, "identity.private.json")
	_, identityRaw, e := hashPrepareFactsFile(ctx, original, uint32(uid), 16<<20, true)
	if e != nil || digestRaw(identityRaw) != request.Identity.ReportSHA256 {
		receipt.ErrorCategory = "db_census_original_identity_read_rejected"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	var identity identityReport
	if decodePrepareFacts(identityRaw, &identity) != nil || !dbCensusIdentityValid(request.Identity, identity) {
		receipt.ErrorCategory = "db_census_original_identity_binding_rejected"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	receipt.SourceUID = uint32(uid)
	receipt.ApprovalSHA256 = request.ObservationApprovalSHA256
	receipt.Identity = request.Identity
	receipt.SQLIdentitySHA256 = identity.States["mysql"].IdentityHash
	receipt.MongoIdentitySHA256 = identity.States["mongodb"].IdentityHash
	receipt.MongoAnchorSHA256 = identity.States["mongodb"].DatabaseAnchorHash
	receipt.SQLVersion = 99
	receipt.MongoVersion = 38
	owner, e := openLifecyclePreparationOwner(ctx, lifecycleRequest{})
	if e != nil {
		receipt.ErrorCategory = "db_census_original_connection_failed"
		return receipt, e
	}
	defer func() {
		if owner != nil {
			if closeErr := owner.Close(); result == nil && closeErr != nil {
				receipt.ObservationComplete = false
				receipt.ErrorCategory = "db_census_owner_close_failed"
				result = closeErr
			}
		}
	}()
	check := func() error {
		return observeBorrowedDatabaseIdentity(ctx, owner, map[string]string{"mysql": receipt.SQLIdentitySHA256, "mongodb": receipt.MongoIdentitySHA256}, map[string]uint64{"mysql": 99, "mongodb": 38}, identity.States["mongodb"].NamespaceAnchor, receipt.MongoAnchorSHA256)
	}
	if check() != nil {
		receipt.ErrorCategory = "db_census_actual_identity_rejected"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	actual, censusErr := dbcensus.Observe(ctx, owner.originalSQL, owner.originalMongo)
	receipt.Sections = actual.Sections
	receipt.Gaps = actual.Gaps
	// Actual identity/head/namespace and original producer bytes are reread after
	// all account/session reads. A past original report is not the new census.
	if check() != nil {
		receipt.ErrorCategory = "db_census_actual_identity_changed"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	_, after, e := hashPrepareFactsFile(ctx, original, uint32(uid), 16<<20, true)
	if e != nil || digestRaw(after) != request.Identity.ReportSHA256 {
		receipt.ErrorCategory = "db_census_original_identity_changed"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	private := dbCensusPrivate{SourceSHA: sourceSHA, OperationID: op, RunID: run, RequestSHA256: expected, IdentityProducer: request.Identity, SQLIdentitySHA256: receipt.SQLIdentitySHA256, MongoIdentitySHA256: receipt.MongoIdentitySHA256, MongoAnchorSHA256: receipt.MongoAnchorSHA256, ObservedAt: started.UTC().Format(time.RFC3339Nano), SQL: actual.SQL, Mongo: actual.Mongo, Sections: receipt.Sections, Gaps: receipt.Gaps}
	encoded, e := json.Marshal(private)
	if e != nil || len(encoded) > 64<<20 || writeJSON(filepath.Join(lifecycleRootBatch(op, run), "db-writer-census.private.json"), private) != nil {
		receipt.ErrorCategory = "db_census_private_catalog_write_failed"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	receipt.PrivateCatalogSHA256 = digestRaw(append(encoded, '\n'))
	receipt.CatalogSHA256 = actual.MembershipSHA256()
	if owner.closeHandles(ctx) != nil {
		receipt.ErrorCategory = "db_census_owner_close_failed"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	owner = nil
	receipt.MySQLAllConnectionsPermissionProven = false
	receipt.MongoLocalAllSessionsPermissionProven = true
	for _, section := range receipt.Sections {
		if section.Name == "mysql_connections" {
			receipt.MySQLAllConnectionsPermissionProven = section.EnumerationComplete
		}
		if section.Name == "mongodb_connections_and_idle_operations" || section.Name == "mongodb_local_logical_sessions" || section.Name == "mongodb_persisted_logical_sessions" {
			receipt.MongoLocalAllSessionsPermissionProven = receipt.MongoLocalAllSessionsPermissionProven && section.EnumerationComplete
		}
	}
	receipt.ObservationComplete = true
	for _, section := range receipt.Sections {
		if !section.EnumerationComplete {
			receipt.ObservationComplete = false
		}
	}
	if censusErr != nil || !receipt.ObservationComplete {
		receipt.ErrorCategory = "db_census_catalog_or_session_permissions_incomplete"
		return receipt, lifecycleError(receipt.ErrorCategory)
	}
	receipt.ErrorCategory = "none"
	return receipt, nil
}
