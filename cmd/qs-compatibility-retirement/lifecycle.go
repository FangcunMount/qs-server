package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
)

// This descriptor supplies expected bindings only. A physical archive, root
// Window, actual isolated restore, writer lease and original native journal
// are produced or inspected separately; no imported completion flag is used.
type lifecycleRequest struct {
	FormatVersion     int                                  `json:"format_version"`
	Kind              string                               `json:"kind"`
	ToolSourceSHA     string                               `json:"tool_source_sha"`
	OriginalSourceSHA string                               `json:"original_source_sha"`
	OperationID       string                               `json:"operation_id"`
	ActualRunID       string                               `json:"actual_run_id"`
	ManifestSHA256    string                               `json:"manifest_sha256"`
	ArchiveDirectory  string                               `json:"archive_directory"`
	SourceDirectory   string                               `json:"source_directory,omitempty"`
	WindowDirectory   string                               `json:"window_directory"`
	JournalDirectory  string                               `json:"journal_directory"`
	Approval          backup.Approval                      `json:"archive_approval"`
	Recovery          backup.TargetRecoveryRequest         `json:"recovery"`
	Resume            *backup.TargetBRecoveryResumeRequest `json:"resume,omitempty"`
	ResumeKind        string                               `json:"resume_kind,omitempty"`
	RestoreEngines    *lifecycleRestoreEngines             `json:"restore_engines,omitempty"`
	SourceFileSHA256  map[string]string                    `json:"source_file_sha256,omitempty"`
	ServiceControl    *lifecycleServiceControl             `json:"service_control,omitempty"`
	DeploymentControl *lifecycleAPIDeploymentControl       `json:"deployment_control,omitempty"`
	FinalHistory      *lifecycleFinalHistoryInput          `json:"final_history,omitempty"`
	requestSHA256     string                               `json:"-"`
	prepareRoot       string                               `json:"-"`
}

type lifecycleFrozenManifest struct {
	FormatVersion    int    `json:"format_version"`
	OperationID      string `json:"operation_id"`
	SourceSHA        string `json:"source_sha"`
	TargetHash       string `json:"target_hash"`
	DatabaseBindings map[string]struct {
		IdentityHash  string `json:"identity_hash"`
		Version       uint64 `json:"migration_version"`
		Dirty         bool   `json:"migration_dirty"`
		CatalogHash   string `json:"catalog_hash"`
		NonTargetHash string `json:"non_target_schema_hash"`
	} `json:"database_bindings"`
	Targets []struct {
		Database     string `json:"database"`
		Name         string `json:"name"`
		Kind         string `json:"kind"`
		IdentityHash string `json:"identity_hash"`
		SchemaHash   string `json:"schema_hash"`
		DataHash     string `json:"data_hash"`
		Records      uint64 `json:"records"`
	} `json:"targets"`
	Evidence map[string]struct {
		Filename string `json:"filename"`
		SHA256   string `json:"sha256"`
	} `json:"evidence"`
	Maintenance struct {
		MaxSeconds         int `json:"max_seconds"`
		ForwardStopSeconds int `json:"forward_stop_seconds"`
		RollbackSeconds    int `json:"rollback_seconds"`
	} `json:"maintenance"`
}

type lifecycleError string

func (e lifecycleError) Error() string { return string(e) }

type lifecycleReceipt struct {
	FormatVersion                  int      `json:"format_version"`
	Kind                           string   `json:"kind"`
	Operation                      string   `json:"operation"`
	SourceSHA                      string   `json:"source_sha"`
	OriginalSourceSHA              string   `json:"original_source_sha"`
	OperationID                    string   `json:"operation_id"`
	RunID                          string   `json:"run_id"`
	ManifestSHA256                 string   `json:"manifest_sha256"`
	RequestSHA256                  string   `json:"request_sha256"`
	ArchiveSHA256                  string   `json:"archive_sha256"`
	TargetHash                     string   `json:"target_hash"`
	TargetCount                    int      `json:"target_count"`
	Complete                       bool     `json:"complete"`
	ExecutionAllowed               bool     `json:"execution_allowed"`
	DropReady                      bool     `json:"drop_ready"`
	ArchiveBindingComplete         bool     `json:"archive_binding_complete"`
	IsolatedContentRestoreComplete bool     `json:"isolated_content_restore_complete"`
	RestoreElapsedMillis           int64    `json:"restore_elapsed_millis"`
	SQLRecoveryBaselineSHA256      string   `json:"mysql_recovery_non_target_sha256,omitempty"`
	RecoveryAttempted              bool     `json:"recovery_attempted"`
	RecoveryComplete               bool     `json:"recovery_complete"`
	AcceptanceComplete             bool     `json:"acceptance_complete"`
	PurgeComplete                  bool     `json:"purge_complete"`
	ErrorCategory                  string   `json:"error_category"`
	RecoveryErrorCategory          string   `json:"recovery_error_category,omitempty"`
	RequiredAdapters               []string `json:"required_adapters"`
}

// The production implementation belongs to the fixed host caller, not a
// request decoder. Prepare must finish all read/CAS/readback transactions
// before obtaining this dedicated autocommit Conn (the pool is limited to one).
// Restore proofs come from the actual RestoreSQL/RestoreMongoWithOriginal
// producers and remain in this process; their JSON summaries are not inputs.
type lifecyclePreparation struct {
	Borrowed              backup.TargetRecoveryBorrowed
	SQLRestore            *backup.RestoreVerification
	MongoRestore          *backup.RestoreVerification
	combinedElapsedMillis int64
}

type lifecycleHost interface {
	Prepare(context.Context, lifecycleRequest, *backup.Archive) (*lifecyclePreparation, error)
	OpenRecoveryHandles(context.Context, lifecycleRequest, *backup.Archive) (backup.TargetRecoveryBorrowed, error)
	StopAndDrain(context.Context, lifecycleRequest, *fence.MaintenanceWindow) error
	OpenServiceManagement(context.Context, lifecycleRequest, *fence.MaintenanceWindow) error
	// Before any target DDL plan exists, recover only native service actions
	// already issued by this host; no reconstruction, database restore or deploy.
	RestoreStoppedServices(context.Context, lifecycleRequest, *fence.MaintenanceWindow) error
	CheckWholeWriterFence(context.Context, lifecycleRequest) error
	FinalDifferenceAndEOF(context.Context, lifecycleRequest, *backup.Archive) error
	DeployBInline(context.Context, lifecycleRequest, *migration.CompatibilityPairMigrationProof, *fence.MaintenanceWindow) error
	VerifyAcceptance(context.Context, lifecycleRequest, *backup.Archive) error
	CheckActualDDLStopped(context.Context, lifecycleRequest) error
	DeployRollbackInline(context.Context, lifecycleRequest, *lifecycleRecoveryReadback, *fence.MaintenanceWindow) error
	// PurgeTemporaryCopies includes isolated restore copies, full-source/CAS
	// spools and all original bodies registered to this batch. Receipts retain
	// only identity/digests/findings. Ordinary backups/old archives are excluded.
	PurgeTemporaryCopies(context.Context, lifecycleRequest) error
	VerifyTemporaryMaterialsZero(context.Context, lifecycleRequest) error
	// Successful B handoff uses ForwardContext and actual approved B identity;
	// it keeps B API while resuming original Collection/Worker/settings.
	ResumeAcceptedEntrypoints(context.Context, lifecycleRequest) error
	// Recovery restores only the approved rollback runtime under the original
	// RecoveryContext. It must not take the cancelled forward ctx as its parent.
	RestoreRollbackEntrypoints(context.Context, lifecycleRequest, *fence.MaintenanceWindow) error
	Close() error
}

func lifecycleCategory(err error) string {
	if err == nil {
		return "none"
	}
	var fixed lifecycleError
	if errors.As(err, &fixed) {
		return string(fixed)
	}
	var kernel backup.Error
	if errors.As(err, &kernel) {
		return string(kernel)
	}
	return "lifecycle_native_operation_failed"
}

func readLifecyclePrivate(path, expected string, dst any) error {
	return readLifecyclePrivateLimit(path, expected, dst, 256<<10)
}

func readLifecyclePrivateLimit(path, expected string, dst any, maximum int64) error {
	return readLifecyclePrivateAs(path, expected, dst, maximum, uint32(os.Getuid()))
}

func readLifecyclePrivateAs(path, expected string, dst any, maximum int64, owner uint32) error {
	if !hashRE.MatchString(expected) || lifecycleSourcePrivateDirectory(filepath.Dir(path), owner) != nil {
		return lifecycleError("lifecycle_private_input_rejected")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return lifecycleError("lifecycle_private_input_rejected")
	}
	st, statErr := f.Stat()
	if statErr != nil || st == nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() < 0 || st.Size() > maximum || st.Sys().(*syscall.Stat_t).Uid != owner || st.Sys().(*syscall.Stat_t).Nlink != 1 {
		_ = f.Close()
		return lifecycleError("lifecycle_private_input_rejected")
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, maximum+1))
	after, afterErr := f.Stat()
	named, namedErr := os.Lstat(path)
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || afterErr != nil || namedErr != nil || !lifecycleFinalFileSame(st, after) || !lifecycleFinalFileSame(st, named) || int64(len(raw)) > maximum || digestRaw(raw) != expected || rejectDuplicateJSON(raw) != nil {
		return lifecycleError("lifecycle_private_input_rejected")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	typ := reflect.TypeOf(dst)
	if typ == nil || typ.Kind() != reflect.Pointer || lifecycleExactJSONNames(raw, typ.Elem()) != nil {
		return lifecycleError("lifecycle_private_input_rejected")
	}
	if d.Decode(dst) != nil || d.Decode(new(any)) != io.EOF {
		return lifecycleError("lifecycle_private_input_rejected")
	}
	return nil
}

// encoding/json accepts case aliases even with DisallowUnknownFields. The
// independently bound request uses exact declared names at every nesting level.
func lifecycleExactJSONNames(raw []byte, t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return lifecycleError("lifecycle_private_input_rejected")
		}
		allowed := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = f.Name
			}
			allowed[tag] = f.Type
		}
		for key, value := range values {
			ft, ok := allowed[key]
			if !ok || lifecycleExactJSONNames(value, ft) != nil {
				return lifecycleError("lifecycle_private_input_rejected")
			}
		}
	case reflect.Array, reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return lifecycleError("lifecycle_private_input_rejected")
		}
		for _, value := range values {
			if lifecycleExactJSONNames(value, t.Elem()) != nil {
				return lifecycleError("lifecycle_private_input_rejected")
			}
		}
	case reflect.Map:
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return lifecycleError("lifecycle_private_input_rejected")
		}
		for _, value := range values {
			if lifecycleExactJSONNames(value, t.Elem()) != nil {
				return lifecycleError("lifecycle_private_input_rejected")
			}
		}
	}
	return nil
}

func lifecycleOwnedPath(root, path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func loadLifecycleRequest(ctx context.Context, path, expected, operation, actualRun, stage string) (lifecycleRequest, *backup.Archive, error) {
	var r lifecycleRequest
	if !runRE.MatchString(operation) || !runRE.MatchString(actualRun) || !shaRE.MatchString(sourceSHA) || filepath.Base(path) != "lifecycle-request.json" {
		return r, nil, lifecycleError("lifecycle_binding_rejected")
	}
	if err := readLifecyclePrivate(path, expected, &r); err != nil {
		return r, nil, err
	}
	root := filepath.Dir(path)
	wantRoot := filepath.Join("/opt/backups/qs-server/compatibility-retirement", operation)
	recovery := r.Recovery
	stagedRoot := lifecycleRootBatch(operation, actualRun)
	staged := root == stagedRoot && stage == "prepare" && r.Recovery.ArchiveSHA256 == "" && r.RestoreEngines.valid()
	invocationRoot := lifecycleInvocationBatch(operation, actualRun)
	if (stage == "prepare" && !staged || stage != "prepare" && root != wantRoot && root != invocationRoot) || r.FormatVersion != 1 || r.Kind != "compatibility_retirement_lifecycle_request" ||
		r.ToolSourceSHA != sourceSHA || !shaRE.MatchString(r.OriginalSourceSHA) || r.OperationID != operation || r.ActualRunID != actualRun ||
		!hashRE.MatchString(r.ManifestSHA256) || r.Approval.SourceSHA != r.OriginalSourceSHA || r.Approval.OperationID != operation ||
		recovery.SourceSHA != r.OriginalSourceSHA || recovery.OperationID != operation || recovery.OriginalRunID != r.Approval.RunID ||
		recovery.ManifestSHA256 != r.ManifestSHA256 || (recovery.ArchiveSHA256 != "" && !hashRE.MatchString(recovery.ArchiveSHA256)) ||
		(recovery.SQLNonTargetSHA256 != "" && !hashRE.MatchString(recovery.SQLNonTargetSHA256)) || !hashRE.MatchString(recovery.MongoNonTargetSHA256) ||
		recovery.SQLHead != 99 || recovery.MongoHead != 38 || !runRE.MatchString(recovery.ActualRunID) {
		return r, nil, lifecycleError("lifecycle_binding_rejected")
	}
	if stage != "prepare" && !r.DeploymentControl.valid() {
		return r, nil, lifecycleError("lifecycle_actual_inline_image_approval_missing")
	}
	if r.FinalHistory != nil && (stage == "prepare" || !r.FinalHistory.valid(r)) {
		return r, nil, lifecycleError("lifecycle_final_historical_input_rejected")
	}
	if stage == "prepare" && !r.RestoreEngines.valid() {
		return r, nil, lifecycleError("lifecycle_restore_engine_approval_missing")
	}
	capture := recovery.ArchiveSHA256 == ""
	if capture && (stage != "prepare" || r.SourceDirectory != filepath.Join(wantRoot, "inventory-"+r.Approval.RunID) || r.Resume != nil) || !capture && (r.SourceDirectory != "" || !hashRE.MatchString(recovery.SQLNonTargetSHA256)) {
		return r, nil, lifecycleError("lifecycle_capture_binding_rejected")
	}
	paths := []string{r.ArchiveDirectory, r.WindowDirectory, r.JournalDirectory}
	if capture {
		paths = append(paths, r.SourceDirectory)
	}
	for i, p := range paths {
		if !lifecycleOwnedPath(wantRoot, p) {
			return r, nil, lifecycleError("lifecycle_private_path_rejected")
		}
		for j := 0; j < i; j++ {
			if p == paths[j] || lifecycleOwnedPath(p, paths[j]) || lifecycleOwnedPath(paths[j], p) {
				return r, nil, lifecycleError("lifecycle_private_path_rejected")
			}
		}
	}
	if r.Resume == nil {
		if r.ResumeKind != "" || recovery.ActualRunID != actualRun {
			return r, nil, lifecycleError("lifecycle_binding_rejected")
		}
	} else if (r.ResumeKind != "original" && r.ResumeKind != "b_complete" && r.ResumeKind != "b_sql_only") ||
		r.Resume.Recovery.Original != recovery || r.Resume.Recovery.CurrentRunID != actualRun ||
		!hashRE.MatchString(r.Resume.Recovery.JournalSHA256) || !hashRE.MatchString(r.Resume.Recovery.WindowStartSHA256) ||
		(r.ResumeKind != "original" && (r.Resume.ApprovedBSourceSHA != sourceSHA || !hashRE.MatchString(r.Resume.MigrationIntentSHA256) || !hashRE.MatchString(r.Resume.MigrationResultSHA256))) {
		return r, nil, lifecycleError("lifecycle_resume_binding_rejected")
	}
	var manifest lifecycleFrozenManifest
	if err := readLifecyclePrivate(filepath.Join(root, "manifest.json"), r.ManifestSHA256, &manifest); err != nil {
		return r, nil, err
	}
	if manifest.FormatVersion != 1 || manifest.OperationID != operation || manifest.SourceSHA != r.OriginalSourceSHA || manifest.TargetHash != digest(targets) ||
		len(manifest.DatabaseBindings) != 2 || len(manifest.Targets) != 4 ||
		manifest.Maintenance.MaxSeconds != 1800 || manifest.Maintenance.ForwardStopSeconds != 1200 || manifest.Maintenance.RollbackSeconds != 600 {
		return r, nil, lifecycleError("lifecycle_original_manifest_rejected")
	}
	for database, binding := range manifest.DatabaseBindings {
		if (database != "mysql" && database != "mongodb") || binding.Dirty || !hashRE.MatchString(binding.IdentityHash) || !hashRE.MatchString(binding.CatalogHash) || !hashRE.MatchString(binding.NonTargetHash) ||
			(database == "mysql" && binding.Version != recovery.SQLHead) ||
			(database == "mongodb" && (binding.Version != recovery.MongoHead || binding.NonTargetHash != recovery.MongoNonTargetSHA256)) {
			return r, nil, lifecycleError("lifecycle_original_manifest_rejected")
		}
	}
	if manifest.DatabaseBindings["mysql"].IdentityHash == manifest.DatabaseBindings["mongodb"].IdentityHash {
		return r, nil, lifecycleError("lifecycle_original_manifest_rejected")
	}
	var expectedObjects [4]backup.HostObjectBinding
	for i, object := range manifest.Targets {
		if object.Database != targets[i][0] || object.Name != targets[i][1] || object.Kind != targets[i][2] ||
			!hashRE.MatchString(object.IdentityHash) || !hashRE.MatchString(object.SchemaHash) || !hashRE.MatchString(object.DataHash) {
			return r, nil, lifecycleError("lifecycle_original_manifest_rejected")
		}
		expectedObjects[i] = backup.HostObjectBinding{Database: object.Database, Name: object.Name, Kind: object.Kind, IdentityHash: object.IdentityHash, SchemaHash: object.SchemaHash, DataHash: object.DataHash, Records: object.Records}
	}
	r.prepareRoot = root
	r.requestSHA256 = expected
	if staged {
		translate := func(p string) string { rel, _ := filepath.Rel(wantRoot, p); return filepath.Join(root, rel) }
		// Preserve the exact approved archive/recovery paths. Only immutable
		// input bytes move into the root-owned once staging tree.
		r.SourceDirectory = translate(r.SourceDirectory)
	}
	if stage == "prepare" {
		if err := verifyLifecycleRestoreImages(ctx, r.RestoreEngines); err != nil {
			return r, nil, err
		}
	}
	var a *backup.Archive
	var err error
	if capture {
		a, err = captureLifecycleArchive(ctx, r)
	} else {
		a, err = backup.OpenArchive(ctx, r.ArchiveDirectory, recovery.ArchiveSHA256)
	}
	if err != nil {
		return r, nil, err
	}
	if capture {
		r.Recovery.ArchiveSHA256 = a.Summary().ArchiveSHA256
		baseline, baselineErr := backup.HostArchiveRecoveryBaseline(a)
		if baselineErr != nil {
			return r, nil, baselineErr
		}
		if r.Recovery.SQLNonTargetSHA256 != "" && r.Recovery.SQLNonTargetSHA256 != baseline {
			return r, nil, lifecycleError("lifecycle_recovery_projection_binding_rejected")
		}
		r.Recovery.SQLNonTargetSHA256 = baseline
		recovery = r.Recovery
	}
	// Inventory retains target-owned FK rows; B recovery excludes them. Bind
	// both original projections to the actual Archive instead of equating them.
	if err = backup.VerifyHostArchiveObjects(ctx, a, expectedObjects, recovery.SQLHead, recovery.MongoHead, manifest.DatabaseBindings["mysql"].NonTargetHash, recovery.SQLNonTargetSHA256, recovery.MongoNonTargetSHA256); err != nil {
		return r, nil, err
	}
	if err = backup.VerifyHostArchiveBinding(ctx, a, r.Approval); err != nil {
		return r, nil, err
	}
	return r, a, nil
}

func lifecyclePreparationMatches(a *backup.Archive, p *lifecyclePreparation) bool {
	if a == nil || p == nil || p.Borrowed.SQL == nil || p.Borrowed.Mongo == nil || p.SQLRestore == nil || p.MongoRestore == nil ||
		p.SQLRestore.Summary().StartedAt.IsZero() || p.MongoRestore.Summary().StartedAt.IsZero() {
		return false
	}
	sqlProof, mongoProof := p.SQLRestore.Summary(), p.MongoRestore.Summary()
	started, finished := sqlProof.StartedAt, sqlProof.FinishedAt
	if mongoProof.StartedAt.Before(started) {
		started = mongoProof.StartedAt
	}
	if mongoProof.FinishedAt.After(finished) {
		finished = mongoProof.FinishedAt
	}
	if sqlProof.FinishedAt.Before(sqlProof.StartedAt) || mongoProof.FinishedAt.Before(mongoProof.StartedAt) || finished.Sub(started) > backup.MaxRestoreSeconds*time.Second {
		return false
	}
	return sqlProof.Database == "mysql" && mongoProof.Database == "mongodb" && sqlProof.TargetCount == 3 && mongoProof.TargetCount == 1 &&
		sqlProof.ArchiveSHA256 == a.Summary().ArchiveSHA256 && mongoProof.ArchiveSHA256 == a.Summary().ArchiveSHA256 &&
		sqlProof.ContentEqual && mongoProof.ContentEqual && sqlProof.SchemaEqual && mongoProof.SchemaEqual &&
		sqlProof.ElapsedMillis >= 0 && mongoProof.ElapsedMillis >= 0 && sqlProof.ElapsedMillis <= backup.MaxRestoreSeconds*1000 && mongoProof.ElapsedMillis <= backup.MaxRestoreSeconds*1000
}

func lifecycleWindowBinding(r lifecycleRequest) fence.WindowBinding {
	return fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: r.OriginalSourceSHA,
		OperationID: r.OperationID, ManifestSHA256: r.ManifestSHA256, OriginalRunID: r.Recovery.OriginalRunID}
}

func lifecycleRecover(ctx context.Context, r lifecycleRequest, a *backup.Archive, p *lifecyclePreparation, w *fence.MaintenanceWindow, host lifecycleHost) error {
	if r.Resume == nil {
		return lifecycleError("lifecycle_resume_material_missing")
	}
	q, cancel, err := w.RecoveryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	if err = host.CheckWholeWriterFence(q, r); err != nil {
		return err
	}
	// Context cancellation alone never proves a remote DDL/driver stopped.
	if err = host.CheckActualDDLStopped(q, r); err != nil {
		return err
	}
	var reconciliation *backup.TargetRecoveryReconciliation
	switch r.ResumeKind {
	case "original":
		reconciliation, err = backup.ReconcileTargetRecovery(q, a, p.Borrowed, r.Resume.Recovery, r.JournalDirectory, w)
	case "b_complete":
		reconciliation, err = backup.ReconcileTargetBRecovery(q, a, p.Borrowed, *r.Resume, r.JournalDirectory, w)
	case "b_sql_only":
		reconciliation, err = backup.ReconcileTargetBSQLOnlyRecovery(q, a, p.Borrowed, *r.Resume, r.JournalDirectory, w)
	default:
		return lifecycleError("lifecycle_resume_binding_rejected")
	}
	if err != nil {
		return err
	}
	recovered, err := backup.ResumeTargetRecovery(q, reconciliation, p.Borrowed, w)
	if err != nil {
		return err
	}
	if err = host.DeployRollbackInline(q, r, &lifecycleRecoveryReadback{resumed: recovered}, w); err != nil {
		return err
	}
	return host.RestoreRollbackEntrypoints(q, r, w)
}

func runLifecycleCLI(ctx context.Context, mode, requestPath, requestHash, operation, actualRun string) (receipt lifecycleReceipt, result error) {
	stage := strings.TrimPrefix(mode, "lifecycle-")
	receipt = lifecycleReceipt{FormatVersion: 1, Kind: "compatibility_retirement_lifecycle_result", Operation: stage, SourceSHA: sourceSHA,
		OperationID: operation, RunID: actualRun, RequestSHA256: requestHash, TargetHash: digest(targets), TargetCount: 4,
		RequiredAdapters: []string{"actual_four_source_historical_persistence_and_readback", "actual_production_bound_isolated_restore", "server_a_and_server_d_stop_drain_lease", "whole_writer_and_old_ref_fence", "prepared_inline_b_and_no_automigration_rollback", "actual_runtime_acceptance_and_private_purge"}}
	defer func() { receipt.ErrorCategory = lifecycleCategory(result) }()
	if ctx == nil || ctx.Err() != nil || (stage != "prepare" && stage != "apply" && stage != "verify" && stage != "recover" && stage != "purge") {
		return receipt, lifecycleError("lifecycle_operation_rejected")
	}
	if stage != "prepare" {
		if err := lifecycleEffectsPreflight(ctx); err != nil {
			return receipt, err
		}
	}
	r, archive, err := loadLifecycleRequest(ctx, requestPath, requestHash, operation, actualRun, stage)
	receipt.OriginalSourceSHA, receipt.ManifestSHA256, receipt.ArchiveSHA256 = r.OriginalSourceSHA, r.ManifestSHA256, r.Recovery.ArchiveSHA256
	if err != nil {
		return receipt, err
	}
	receipt.ArchiveBindingComplete = true
	receipt.SQLRecoveryBaselineSHA256 = r.Recovery.SQLNonTargetSHA256
	if stage == "prepare" {
		prepared, owner, err := prepareLifecycleNative(ctx, r, archive)
		if err != nil {
			return receipt, err
		}
		if !lifecyclePreparationMatches(archive, prepared) {
			_ = owner.Close()
			return receipt, lifecycleError("lifecycle_actual_restore_proof_missing_or_budget_rejected")
		}
		// No success receipt until every actual handle, wire exec and final runtime
		// inspection finishes under the unchanged original600-second deadline.
		elapsed, err := owner.finishPreparation()
		if err != nil {
			return receipt, err
		}
		receipt.RestoreElapsedMillis = elapsed
		receipt.IsolatedContentRestoreComplete, receipt.Complete = true, true
		return receipt, nil
	}

	host, err := newLifecycleFixedHost(ctx, r, archive)
	if err != nil || host == nil {
		if err == nil {
			err = lifecycleError("lifecycle_actual_host_adapters_missing")
		}
		return receipt, err
	}
	defer func() {
		if err := host.Close(); result == nil && err != nil {
			result = err
			receipt.Complete = false
		}
	}()
	var prepared *lifecyclePreparation
	if stage == "prepare" || stage == "apply" {
		prepared, err = host.Prepare(ctx, r, archive)
		if err != nil {
			return receipt, err
		}
		if !lifecyclePreparationMatches(archive, prepared) {
			return receipt, lifecycleError("lifecycle_actual_restore_proof_missing_or_budget_rejected")
		}
	} else {
		borrowed, openErr := host.OpenRecoveryHandles(ctx, r, archive)
		if openErr != nil {
			return receipt, openErr
		}
		if borrowed.SQL == nil || borrowed.Mongo == nil {
			return receipt, lifecycleError("lifecycle_native_handles_missing")
		}
		prepared = &lifecyclePreparation{Borrowed: borrowed}
	}
	if stage == "prepare" {
		receipt.Complete = true
		return receipt, nil
	}
	var window *fence.MaintenanceWindow
	if stage == "apply" {
		if r.Resume != nil {
			return receipt, lifecycleError("lifecycle_apply_cannot_resume_or_redrop")
		}
		window, err = fence.StartMaintenanceWindow(ctx, r.WindowDirectory, lifecycleWindowBinding(r))
	} else {
		window, err = fence.OpenMaintenanceWindow(ctx, r.WindowDirectory, lifecycleWindowBinding(r))
	}
	if err != nil {
		return receipt, err
	}
	defer func() {
		if err := window.Close(); result == nil && err != nil {
			result = err
			receipt.Complete = false
		}
	}()
	if stage == "recover" {
		receipt.RecoveryAttempted = true
		err = lifecycleRecover(ctx, r, archive, prepared, window, host)
		receipt.RecoveryComplete, receipt.Complete = err == nil, err == nil
		return receipt, err
	}
	forward, cancel, err := window.ForwardContext(ctx)
	if err != nil {
		return receipt, err
	}
	defer cancel()
	// Establish and verify the actual D native stdio while the existing pinned
	// SSH entrypoint remains available. The complete writer fence comes next;
	// it cannot make this control binding into writer/DDL authority.
	if err = host.OpenServiceManagement(forward, r, window); err != nil {
		return receipt, err
	}
	if err = host.CheckWholeWriterFence(forward, r); err != nil {
		return receipt, err
	}
	// Arm before Stop: a failed call can already have stopped one side. This
	// service-only recovery covers Stop/check/drain/final-read/plan failures and
	// borrows the original parent, never the cancelled forward context.
	serviceRecoveryPending := true
	defer func() {
		if !serviceRecoveryPending || result == nil || receipt.AcceptanceComplete {
			return
		}
		receipt.RecoveryAttempted = true
		q, c, recoveryErr := window.RecoveryContext(ctx)
		if recoveryErr == nil {
			defer c()
			recoveryErr = host.RestoreStoppedServices(q, r, window)
		}
		receipt.RecoveryComplete = recoveryErr == nil
		receipt.RecoveryErrorCategory = lifecycleCategory(recoveryErr)
	}()
	if err = host.StopAndDrain(forward, r, window); err != nil {
		return receipt, err
	}
	if stage == "verify" || stage == "purge" {
		// The host acceptance producer must inspect the original physical journal,
		// four actual missing targets, schema heads/non-target facts and services.
		// Absence or a saved receipt alone is insufficient in this new process.
		if err = host.VerifyAcceptance(forward, r, archive); err != nil {
			return receipt, err
		}
		receipt.AcceptanceComplete = true
		if stage == "purge" {
			if err = host.PurgeTemporaryCopies(forward, r); err != nil {
				return receipt, err
			}
			if err = backup.PurgeRegistered(r.ArchiveDirectory, r.Approval); err != nil {
				return receipt, err
			}
			if err = host.VerifyTemporaryMaterialsZero(forward, r); err != nil {
				return receipt, err
			}
			receipt.PurgeComplete = true
			if err = host.ResumeAcceptedEntrypoints(forward, r); err != nil {
				return receipt, err
			}
		}
		receipt.Complete = true
		return receipt, nil
	}
	if err = host.FinalDifferenceAndEOF(forward, r, archive); err != nil {
		return receipt, err
	}
	plan, err := backup.PrepareTargetRecovery(forward, archive, prepared.Borrowed, r.Recovery, r.JournalDirectory, window)
	if err != nil {
		return receipt, err
	}
	// Keep the process-bound native plan alive through DROP, migration, inline
	// deploy and acceptance. If forward work fails, begin original-window
	// recovery only after observing actual DDL completion; never down/force dirty.
	defer func() {
		if result == nil || receipt.AcceptanceComplete {
			return
		}
		receipt.RecoveryAttempted = true
		q, c, recoveryErr := window.RecoveryContext(ctx)
		if recoveryErr == nil {
			defer c()
			if recoveryErr = host.CheckWholeWriterFence(q, r); recoveryErr == nil {
				recoveryErr = host.CheckActualDDLStopped(q, r)
			}
			if recoveryErr == nil {
				var recovered *backup.TargetRecoveryVerification
				recovered, recoveryErr = backup.RecoverTargets(q, plan)
				if recoveryErr == nil {
					recoveryErr = host.DeployRollbackInline(q, r, &lifecycleRecoveryReadback{direct: recovered}, window)
				}
			}

			if recoveryErr == nil {
				recoveryErr = host.RestoreRollbackEntrypoints(q, r, window)
			}
		}
		receipt.RecoveryComplete = recoveryErr == nil
		receipt.RecoveryErrorCategory = lifecycleCategory(recoveryErr)
	}()
	// The complete target plan's existing DDL recovery now owns this failure.
	// Never restore the same services a second time through the pre-DDL path.
	serviceRecoveryPending = false
	if _, err = backup.ApplyTargets(forward, plan); err != nil {
		return receipt, err
	}
	migrated, err := backup.RunTargetBMigration(forward, plan, r.ToolSourceSHA)
	if err != nil {
		return receipt, err
	}
	// Deploy directly under this run's existing production-deploy lock. Calling
	// another same-lock workflow and waiting for it would deadlock the window.
	if err = host.DeployBInline(forward, r, migrated, window); err != nil {
		return receipt, err
	}
	if _, err = backup.VerifyDroppedTargets(forward, plan); err != nil {
		return receipt, err
	}
	if err = host.VerifyAcceptance(forward, r, archive); err != nil {
		return receipt, err
	}
	receipt.AcceptanceComplete = true // Accepted deletion proceeds with cleanup/forward repair.
	if err = host.PurgeTemporaryCopies(forward, r); err != nil {
		return receipt, err
	}
	if err = backup.PurgeRegistered(r.ArchiveDirectory, r.Approval); err != nil {
		return receipt, err
	}
	if err = host.VerifyTemporaryMaterialsZero(forward, r); err != nil {
		return receipt, err
	}
	receipt.PurgeComplete = true
	if err = host.ResumeAcceptedEntrypoints(forward, r); err != nil {
		return receipt, err
	}
	receipt.Complete = true
	receipt.RequiredAdapters = []string{}
	return receipt, nil
}
