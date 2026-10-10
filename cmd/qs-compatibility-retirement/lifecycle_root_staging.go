package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

const lifecycleInvocationBase = "/opt/backups/qs-server/compatibility-retirement-invocations"

const lifecycleRootPrepareBase = "/opt/backups/qs-server/compatibility-retirement-root-prepare"

var lifecycleSourceNames = []string{"inventory.private.json", "mysql-metadata.private.json", "mongodb-metadata.private.json", "mysql-domain_event_outbox.source.ndjson", "mysql-ai_bridge_commands.source.ndjson", "mysql-ai_messaging_legacy_commands.source.ndjson", "mongodb-domain_event_outbox.source.bsonframes"}

func lifecycleInvocationBatch(operation, run string) string {
	return filepath.Join(lifecycleInvocationBase, operation+"-"+run)
}

func lifecycleRootBatch(operation, run string) string {
	return filepath.Join(lifecycleRootPrepareBase, operation+"-"+run)
}
func lifecycleSourcePrivateDirectory(path string, uid uint32) error {
	for p := path; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		st, ok := infoStat(info)
		if e != nil || info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ok || (info.Mode().Perm()&022 != 0 && info.Mode()&os.ModeSticky == 0) {
			return lifecycleError("lifecycle_staging_source_directory_rejected")
		}
		if p == path && (info.Mode().Perm() != 0700 || st.Uid != uid) {
			return lifecycleError("lifecycle_staging_source_directory_rejected")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}
func readLifecycleOwnedBytes(path, expected string, uid uint32, maximum int64) (owned []byte, result error) {
	if !hashRE.MatchString(expected) {
		return nil, lifecycleError("lifecycle_staging_hash_rejected")
	}
	if e := lifecycleSourcePrivateDirectory(filepath.Dir(path), uid); e != nil {
		return nil, e
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, lifecycleError("lifecycle_staging_source_rejected")
	}
	defer func() {
		if e := f.Close(); result == nil && e != nil {
			result = lifecycleError("lifecycle_staging_source_close_failed")
		}
	}()
	info, err := f.Stat()
	st, ok := infoStat(info)
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ok || st.Uid != uid || st.Nlink != 1 || info.Size() < 1 || info.Size() > maximum {
		return nil, lifecycleError("lifecycle_staging_source_rejected")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maximum+1))
	after, e := f.Stat()
	if err != nil || e != nil || len(raw) > int(maximum) || !sameLifecycleFile(info, after) || digestRaw(raw) != expected {
		return nil, lifecycleError("lifecycle_staging_bytes_changed_or_hash_rejected")
	}
	return raw, nil
}
func sameLifecycleFile(before, after os.FileInfo) bool {
	a, ok := infoStat(before)
	b, other := infoStat(after)
	return ok && other && a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid && a.Nlink == b.Nlink && a.Mode == b.Mode && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) && reflect.DeepEqual(lifecycleStatChangeTime(a), lifecycleStatChangeTime(b))
}
func lifecycleStatChangeTime(st *syscall.Stat_t) any {
	v := reflect.ValueOf(st).Elem()
	for _, name := range []string{"Ctim", "Ctimespec"} {
		f := v.FieldByName(name)
		if f.IsValid() {
			return f.Interface()
		}
	}
	return nil
}
func decodeLifecycleStagingRequest(raw []byte) (lifecycleRequest, error) {
	var r lifecycleRequest
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return r, lifecycleError("lifecycle_staging_request_rejected")
	}
	for _, name := range []string{"resume", "resume_kind", "service_control", "deployment_control", "final_history", "writer_control", "source_copy_intent", "historical_write_report", "preparation_restore_zero"} {
		if _, exists := fields[name]; exists {
			return r, lifecycleError("lifecycle_staging_request_rejected")
		}
	}
	if rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(r)) != nil {
		return r, lifecycleError("lifecycle_staging_request_rejected")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
		return r, lifecycleError("lifecycle_staging_request_rejected")
	}
	return r, nil
}
func writeLifecycleRaw(path string, raw []byte, mode os.FileMode) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
	if e != nil {
		return lifecycleError("lifecycle_staging_output_exists_or_unknown")
	}
	_, e = f.Write(raw)
	sync := f.Sync()
	close := f.Close()
	if e != nil || sync != nil || close != nil {
		return lifecycleError("lifecycle_staging_output_failed")
	}
	return syncLifecycleDirectory(filepath.Dir(path))
}
func syncLifecycleDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return lifecycleError("lifecycle_staging_directory_sync_failed")
	}
	e = f.Sync()
	c := f.Close()
	if e != nil || c != nil {
		return lifecycleError("lifecycle_staging_directory_sync_failed")
	}
	return nil
}
func copyLifecycleRootSource(source, destination, expected string, uid uint32) (result error) {
	if !hashRE.MatchString(expected) {
		return lifecycleError("lifecycle_staging_hash_rejected")
	}
	if e := lifecycleSourcePrivateDirectory(filepath.Dir(source), uid); e != nil {
		return e
	}
	in, e := os.OpenFile(source, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return lifecycleError("lifecycle_staging_source_rejected")
	}
	defer func() {
		if e := in.Close(); result == nil && e != nil {
			result = lifecycleError("lifecycle_staging_source_close_failed")
		}
	}()
	before, e := in.Stat()
	st, ok := infoStat(before)
	if e != nil || before == nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || !ok || st.Uid != uid || st.Nlink != 1 || before.Size() < 0 || before.Size() > 16<<30 {
		return lifecycleError("lifecycle_staging_source_rejected")
	}
	out, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return lifecycleError("lifecycle_staging_output_exists_or_unknown")
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, (16<<30)+1))
	sync := out.Sync()
	close := out.Close()
	after, statErr := in.Stat()
	if e != nil || sync != nil || close != nil || statErr != nil || n != before.Size() || !sameLifecycleFile(before, after) || hex.EncodeToString(h.Sum(nil)) != expected {
		return lifecycleError("lifecycle_staging_bytes_changed_or_hash_rejected")
	}
	return syncLifecycleDirectory(filepath.Dir(destination))
}
func stageLifecycleRootInputs(ctx context.Context, path, expected, operation, actualRun string) (string, error) {
	uid64, e := strconv.ParseUint(os.Getenv("QS_RETIREMENT_SOURCE_UID"), 10, 32)
	if e != nil || os.Getuid() != 0 || os.Geteuid() != 0 || !runRE.MatchString(operation) || !runRE.MatchString(actualRun) || !shaRE.MatchString(sourceSHA) {
		return "", lifecycleError("lifecycle_staging_root_once_required")
	}
	original := filepath.Join("/opt/backups/qs-server/compatibility-retirement", operation)
	metadataUID := uint32(uid64)
	metadataRoot := original
	if path == filepath.Join(lifecycleInvocationBatch(operation, actualRun), "lifecycle-request.json") {
		// Only the exact current-run metadata is root-owned. The original
		// producer and all seven source bodies retain their source UID binding.
		metadataUID, metadataRoot = 0, lifecycleInvocationBatch(operation, actualRun)
	} else if path != filepath.Join(original, "lifecycle-request.json") {
		return "", lifecycleError("lifecycle_staging_path_rejected")
	}
	raw, e := readLifecycleOwnedBytes(path, expected, metadataUID, 256<<10)
	if e != nil {
		return "", e
	}
	r, e := decodeLifecycleStagingRequest(raw)
	if e != nil {
		return "", e
	}
	if !runRE.MatchString(r.Approval.RunID) || actualRun == r.Approval.RunID || actualRun == r.Recovery.OriginalRunID || !shaRE.MatchString(r.OriginalSourceSHA) || r.Approval.SourceSHA != r.OriginalSourceSHA || r.Approval.OperationID != operation || r.FormatVersion != 1 || r.Kind != "compatibility_retirement_lifecycle_request" || r.OperationID != operation || r.ActualRunID != actualRun || r.ToolSourceSHA != sourceSHA || r.Recovery.ArchiveSHA256 != "" || !r.RestoreEngines.valid() || r.SourceDirectory != filepath.Join(original, "inventory-"+r.Approval.RunID) || len(r.SourceFileSHA256) != 7 {
		return "", lifecycleError("lifecycle_staging_binding_rejected")
	}
	// Ordered raw BSON schema requires its distinct approval from an actual
	// ReadOrderedMongoSchema observation; an empty DTO never approves itself.
	if !hashRE.MatchString(r.Approval.OrderedMongoSchemaSHA256) {
		return "", lifecycleError("lifecycle_ordered_mongo_schema_approval_missing")
	}
	for _, name := range lifecycleSourceNames {
		if !hashRE.MatchString(r.SourceFileSHA256[name]) {
			return "", lifecycleError("lifecycle_staging_source_hash_missing")
		}
	}
	if r.SourceFileSHA256[lifecycleSourceNames[0]] != r.Approval.InventorySHA256 || r.SourceFileSHA256[lifecycleSourceNames[1]] != r.Approval.SQLMetadataSHA256 || r.SourceFileSHA256[lifecycleSourceNames[2]] != r.Approval.MongoMetadataSHA256 {
		return "", lifecycleError("lifecycle_staging_metadata_binding_rejected")
	}
	manifest, e := readLifecycleOwnedBytes(filepath.Join(metadataRoot, "manifest.json"), r.ManifestSHA256, metadataUID, 256<<10)
	if e != nil {
		return "", e
	}
	root := lifecycleRootBatch(operation, actualRun)
	if privateDir(root) != nil {
		return "", lifecycleError("lifecycle_staging_fixed_root_rejected")
	}
	paths := []string{r.ArchiveDirectory, r.WindowDirectory, r.JournalDirectory, r.SourceDirectory}
	for i, p := range paths {
		if !lifecycleOwnedPath(original, p) {
			return "", lifecycleError("lifecycle_staging_path_rejected")
		}
		for j := 0; j < i; j++ {
			if p == paths[j] || lifecycleOwnedPath(p, paths[j]) || lifecycleOwnedPath(paths[j], p) {
				return "", lifecycleError("lifecycle_staging_path_rejected")
			}
		}
	}
	// Actual pinned local image facts are observed before any source-body copy
	// or original DB connection. Starting the engines reobserves them again.
	if e = verifyLifecycleRestoreImages(ctx, r.RestoreEngines); e != nil {
		return "", e
	}
	// Root bootstrap has already installed only the approved binary here. No
	// original owner/path is modified. The exact per-run source-copy intent is
	// durable before even the first copied body, including partial failures.
	intent := map[string]any{"format_version": 1, "kind": "root_once_exact_source_copy_intent", "original_source_sha": r.OriginalSourceSHA, "tool_source_sha": r.ToolSourceSHA, "operation_id": operation, "actual_run_id": actualRun, "original_run_id": r.Approval.RunID, "request_sha256": expected, "manifest_sha256": r.ManifestSHA256, "source_file_sha256": r.SourceFileSHA256, "targets": targets, "source_uid": uid64, "archive_directory": r.ArchiveDirectory, "source_staging_directory": filepath.Join(root, "inventory-"+r.Approval.RunID), "drop_authority": false, "purge_after_acceptance_required": true}
	// The original approved root caller passes only its actual write-time hashes.
	// These transient inputs bind files; they never import a native capability.
	basisHash, resultHash := os.Getenv("QS_LIFECYCLE_BUDGET_KEY_DESCRIPTOR_SHA256"), os.Getenv("QS_LIFECYCLE_BUDGET_KEY_RESULT_SHA256")
	if (basisHash == "") != (resultHash == "") || basisHash != "" && (!hashRE.MatchString(basisHash) || !hashRE.MatchString(resultHash)) {
		return "", lifecycleError("lifecycle_preparation_budget_material_rejected")
	}
	if basisHash != "" {
		basis, e := readLifecycleOwnedBytes(filepath.Join(root, "budget-key.basis.private.json"), basisHash, 0, 256<<10)
		if e != nil {
			return "", e
		}
		result, e := readLifecycleOwnedBytes(filepath.Join(root, "budget-key.result.private.json"), resultHash, 0, 256<<10)
		if e != nil || validateLifecyclePreparationBudgetBytes(basis, result, lifecycleSourceCopyIntent{OriginalSourceSHA: r.OriginalSourceSHA, ToolSourceSHA: r.ToolSourceSHA, OperationID: operation, ActualRunID: actualRun, OriginalRunID: r.Approval.RunID, ManifestSHA256: r.ManifestSHA256}) != nil {
			return "", lifecycleError("lifecycle_preparation_budget_material_rejected")
		}
		intent["budget_key_descriptor_sha256"], intent["budget_key_result_sha256"] = basisHash, resultHash
	}
	if writeJSON(filepath.Join(root, "source-copy.intent.private.json"), intent) != nil {
		return "", lifecycleError("lifecycle_staging_once_exists_or_unknown")
	}
	for _, p := range []string{r.ArchiveDirectory, r.WindowDirectory, r.JournalDirectory, r.SourceDirectory} {
		if !lifecycleOwnedPath(original, p) {
			return "", lifecycleError("lifecycle_staging_path_rejected")
		}
	}
	// Archive stays at the exact approved path: a new root-owned leaf only.
	// Original operation/source directories and files keep their original owners.
	if os.MkdirAll(r.ArchiveDirectory, 0700) != nil || privateDir(r.ArchiveDirectory) != nil {
		return "", lifecycleError("lifecycle_staging_archive_directory_rejected")
	}
	srcDir := filepath.Join(root, "inventory-"+r.Approval.RunID)
	if os.Mkdir(srcDir, 0700) != nil || privateDir(srcDir) != nil {
		return "", lifecycleError("lifecycle_staging_directory_rejected")
	}
	if writeLifecycleRaw(filepath.Join(root, "manifest.json"), manifest, 0600) != nil {
		return "", lifecycleError("lifecycle_staging_manifest_copy_failed")
	}
	for _, name := range lifecycleSourceNames {
		if e = copyLifecycleRootSource(filepath.Join(r.SourceDirectory, name), filepath.Join(srcDir, name), r.SourceFileSHA256[name], uint32(uid64)); e != nil {
			return "", e
		}
	}
	staged := filepath.Join(root, "lifecycle-request.json")
	if e = writeLifecycleRaw(staged, raw, 0600); e != nil {
		return "", e
	}
	return staged, nil
}

// These are the existing inventory producer's two closed file schemas. They
// bind members to the independently approved report, not to an imported permit.
type lifecycleInventorySourceAsset struct {
	FormatVersion        int            `json:"format_version"`
	Kind                 string         `json:"kind"`
	Filename             string         `json:"filename"`
	SourceSHA            string         `json:"source_sha"`
	OperationID          string         `json:"operation_id"`
	RunID                string         `json:"run_id"`
	RequestHash          string         `json:"request_hash"`
	Protocol             string         `json:"protocol"`
	Boundary             targetBoundary `json:"boundary"`
	ContainsOriginalBody bool           `json:"contains_original_body"`
	RetirementProof      bool           `json:"retirement_proof"`
	PurgeRequired        bool           `json:"purge_required_after_acceptance"`
	ResumeExisting       bool           `json:"resume_existing_file_allowed"`
}
type lifecycleInventoryCheckpoint struct {
	FormatVersion  int    `json:"format_version"`
	Kind           string `json:"kind"`
	SourceSHA      string `json:"source_sha"`
	Pass           int    `json:"pass"`
	Page           int    `json:"page"`
	Cursor         string `json:"cursor_token"`
	Records        uint64 `json:"records"`
	SourceBytes    uint64 `json:"source_bytes"`
	PrefixHash     string `json:"prefix_hash"`
	DiagnosticOnly bool   `json:"diagnostic_only"`
	ResumeExisting bool   `json:"resume_existing_file_allowed"`
}

// Open, read to bounded EOF, validate the original inode, and retain a real RO
// FD in the existing catalog. Unknown JSON fields never become named members.
// The caller validates the exact producer tuple/schema before returning an owner.
func readLifecycleProducerJSON(d *lifecycleMaterialDirectory, name, expected string, uid uint32, maximum int64, value any) (raw []byte, result error) {
	if d.unchanged() != nil || filepath.Base(name) != name || maximum < 1 || maximum > 4<<20 {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	f, e := os.OpenFile(filepath.Join(d.path, name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	defer func() {
		if e := f.Close(); e != nil && result == nil {
			result = lifecycleError("lifecycle_material_file_close_unknown")
		}
	}()
	before, e := f.Stat()
	st, ok := infoStat(before)
	if e != nil || !ok || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || st.Uid != uid || st.Nlink != 1 || before.Size() < 1 || before.Size() > maximum {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	raw, e = io.ReadAll(io.LimitReader(f, maximum+1))
	after, ae := f.Stat()
	if e != nil || ae != nil || int64(len(raw)) != before.Size() || !sameLifecycleFile(before, after) || expected != "" && digestRaw(raw) != expected || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(value).Elem()) != nil || json.Unmarshal(raw, value) != nil {
		return nil, lifecycleError("lifecycle_material_bytes_or_identity_changed")
	}
	if e = d.register(name, digestRaw(raw), uid, 0600); e != nil {
		return nil, e
	}
	if !sameLifecycleFile(before, d.files[name].info) || d.checkFile(d.files[name]) != nil {
		return nil, lifecycleError("lifecycle_material_bytes_or_identity_changed")
	}
	return raw, nil
}

func openLifecycleOriginalInventoryMaterials(ctx context.Context, r lifecycleRequest, a *backup.Archive, uid uint32) (*lifecycleMaterialDirectory, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || !runRE.MatchString(r.OperationID) || !runRE.MatchString(r.Approval.RunID) || r.Approval.SourceSHA != r.OriginalSourceSHA || r.Approval.OperationID != r.OperationID {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	// The only production caller has just verified this original native Archive
	// against Approval. Do not rescan its four bodies a second time here.
	if a.Summary().ArchiveSHA256 != r.Recovery.ArchiveSHA256 || !hashRE.MatchString(r.Recovery.ArchiveSHA256) {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	hashes := map[string]string{lifecycleSourceNames[0]: r.Approval.InventorySHA256, lifecycleSourceNames[1]: r.Approval.SQLMetadataSHA256, lifecycleSourceNames[2]: r.Approval.MongoMetadataSHA256}
	for i, asset := range a.TemporarySourceAssets() {
		if asset.Filename != lifecycleSourceNames[3+i] || !hashRE.MatchString(asset.SHA256) || asset.Bytes < 0 || asset.Bytes > 2<<30 {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		hashes[asset.Filename] = asset.SHA256
	}
	if len(r.SourceFileSHA256) != 0 && !reflect.DeepEqual(r.SourceFileSHA256, hashes) {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	path := filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID, "inventory-"+r.Approval.RunID)
	return openLifecycleInventoryMaterialFiles(ctx, path, uid, r.Approval, hashes)
}

// The complete name set comes from the approved producer report and its exact
// two-pass/page-size protocol. ReadDir is used only to reject extra members.
func openLifecycleInventoryMaterialFiles(ctx context.Context, path string, uid uint32, approval backup.Approval, hashes map[string]string) (owned *lifecycleMaterialDirectory, result error) {
	if ctx == nil || ctx.Err() != nil || len(hashes) != len(lifecycleSourceNames) || hashes[lifecycleSourceNames[0]] != approval.InventorySHA256 || hashes[lifecycleSourceNames[1]] != approval.SQLMetadataSHA256 || hashes[lifecycleSourceNames[2]] != approval.MongoMetadataSHA256 {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	d, e := openLifecycleMaterialDirectory(path, uid)
	if e != nil {
		return nil, e
	}
	defer func() {
		if result != nil {
			if e := d.close(); e != nil {
				result = e
			}
		}
	}()
	var inventory report
	if _, e = readLifecycleProducerJSON(d, lifecycleSourceNames[0], approval.InventorySHA256, uid, 4<<20, &inventory); e != nil {
		return nil, e
	}
	if inventory.FormatVersion != 2 || inventory.Kind != "readonly_compatibility_inventory" || inventory.SourceSHA != approval.SourceSHA || inventory.OperationID != approval.OperationID || inventory.RunID != approval.RunID || inventory.RequestHash != approval.RequestHash || inventory.TargetHash != digest(targets) || len(inventory.Targets) != 4 || !inventory.Complete || inventory.DropReady || inventory.ErrorCategory != "none" {
		return nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	for _, name := range lifecycleSourceNames[1:] {
		if !hashRE.MatchString(hashes[name]) {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		if e = d.register(name, hashes[name], uid, 0600); e != nil {
			return nil, e
		}
	}
	limits := productionLimits()
	for i, s := range inventory.Targets {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if s.Database != targets[i][0] || s.Name != targets[i][1] || s.Kind != targets[i][2] || s.SourceFile != lifecycleSourceNames[3+i] || s.Boundary == nil || !s.Present || !s.Complete || s.Passes != 2 || !hashRE.MatchString(s.DataHash) || s.Records > uint64(limits.MaxRecords) || s.Bytes > uint64(limits.MaxBytes) || s.Pages%2 != 0 {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		pages := s.Pages / 2
		wantPages := s.Records/uint64(limits.PageSize) + 1
		if s.Boundary.Empty {
			wantPages = 0
			if s.Records != 0 || s.Bytes != 0 {
				return nil, lifecycleError("lifecycle_material_registration_rejected")
			}
		}
		if pages != wantPages || pages > uint64(limits.MaxPages) {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		protocol := "mysql_cast_binary_columns_pk_order_v2"
		if i == 3 {
			protocol = "mongodb_server_bson_pk_order_v2"
		}
		var asset lifecycleInventorySourceAsset
		if _, e = readLifecycleProducerJSON(d, s.SourceFile+".asset.json", "", uid, 4<<20, &asset); e != nil {
			return nil, e
		}
		if asset.FormatVersion != 1 || asset.Kind != "temporary_inventory_source_copy" || asset.Filename != s.SourceFile || asset.SourceSHA != approval.SourceSHA || asset.OperationID != approval.OperationID || asset.RunID != approval.RunID || asset.RequestHash != approval.RequestHash || asset.Protocol != protocol || !reflect.DeepEqual(asset.Boundary, *s.Boundary) || !asset.ContainsOriginalBody || asset.RetirementProof || !asset.PurgeRequired || asset.ResumeExisting {
			return nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		var previous lifecycleInventoryCheckpoint
		for page := 1; uint64(page) <= pages; page++ {
			var pair [2]lifecycleInventoryCheckpoint
			for pass := 1; pass <= 2; pass++ {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				name := fmt.Sprintf("%s-%s-pass-%d-page-%06d.checkpoint.json", s.Database, s.Name, pass, page)
				if _, e = readLifecycleProducerJSON(d, name, "", uid, 4<<20, &pair[pass-1]); e != nil {
					return nil, e
				}
				p := pair[pass-1]
				records := min(uint64(page)*uint64(limits.PageSize), s.Records)
				if p.FormatVersion != 1 || p.Kind != "readonly_inventory_page_checkpoint" || p.SourceSHA != approval.SourceSHA || p.Pass != pass || p.Page != page || p.Records != records || p.SourceBytes > s.Bytes || p.SourceBytes < previous.SourceBytes || !hashRE.MatchString(p.PrefixHash) || !p.DiagnosticOnly || p.ResumeExisting || p.Cursor == "" || uint64(page) == pages && (p.Records != s.Records || p.SourceBytes != s.Bytes || p.PrefixHash != s.DataHash) {
					return nil, lifecycleError("lifecycle_material_registration_rejected")
				}
			}
			pair[1].Pass = 1
			if !reflect.DeepEqual(pair[0], pair[1]) {
				return nil, lifecycleError("lifecycle_material_registration_rejected")
			}
			if previous.Records < pair[0].Records && previous.Cursor == pair[0].Cursor {
				return nil, lifecycleError("lifecycle_material_registration_rejected")
			}
			previous = pair[0]
		}
	}
	if e = d.checkComplete(false); e != nil {
		return nil, e
	}
	return d, nil
}

type lifecycleSourceCopyIntent struct {
	FormatVersion          int               `json:"format_version"`
	Kind                   string            `json:"kind"`
	OriginalSourceSHA      string            `json:"original_source_sha"`
	ToolSourceSHA          string            `json:"tool_source_sha"`
	OperationID            string            `json:"operation_id"`
	ActualRunID            string            `json:"actual_run_id"`
	OriginalRunID          string            `json:"original_run_id"`
	RequestSHA256          string            `json:"request_sha256"`
	ManifestSHA256         string            `json:"manifest_sha256"`
	SourceFileSHA256       map[string]string `json:"source_file_sha256"`
	Targets                [][3]string       `json:"targets"`
	SourceUID              uint32            `json:"source_uid"`
	ArchiveDirectory       string            `json:"archive_directory"`
	SourceStagingDirectory string            `json:"source_staging_directory"`
	DropAuthority          bool              `json:"drop_authority"`
	PurgeRequired          bool              `json:"purge_after_acceptance_required"`
	BudgetDescriptorSHA256 string            `json:"budget_key_descriptor_sha256,omitempty"`
	BudgetResultSHA256     string            `json:"budget_key_result_sha256,omitempty"`
}

func decodeLifecycleSourceCopyIntent(raw []byte, i *lifecycleSourceCopyIntent) error {
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &fields) != nil {
		return lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	_, basis := fields["budget_key_descriptor_sha256"]
	_, result := fields["budget_key_result_sha256"]
	if basis != result {
		return lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	if !basis {
		fields["budget_key_descriptor_sha256"], fields["budget_key_result_sha256"] = json.RawMessage(`""`), json.RawMessage(`""`)
	}
	normalized, e := json.Marshal(fields)
	if e != nil || decodeLifecycleClosedProducer(normalized, i) != nil || (i.BudgetDescriptorSHA256 == "") != (i.BudgetResultSHA256 == "") || basis && (!hashRE.MatchString(i.BudgetDescriptorSHA256) || !hashRE.MatchString(i.BudgetResultSHA256)) {
		return lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	return nil
}

type lifecyclePreparationBudgetResult struct {
	Kind             string `json:"kind"`
	ToolSourceSHA    string `json:"tool_source_sha"`
	OperationID      string `json:"operation_id"`
	ActualRunID      string `json:"actual_run_id"`
	PublicKey        string `json:"public_key"`
	KeyAvailable     bool   `json:"key_available"`
	WholeWriterFence bool   `json:"whole_writer_fence_proven"`
	DropReady        bool   `json:"drop_ready"`
	ErrorCategory    string `json:"error_category"`
}

// This validates the original temporary bytes and their tuple, never creates
// an Approval, key/lease or writer fence from either persisted JSON record.
func validateLifecyclePreparationBudgetBytes(basis, result []byte, i lifecycleSourceCopyIntent) error {
	var descriptor stop.Descriptor
	var receipt lifecyclePreparationBudgetResult
	if rejectDuplicateJSON(basis) != nil || lifecycleExactJSONNames(basis, reflect.TypeOf(descriptor)) != nil || json.Unmarshal(basis, &descriptor) != nil || descriptor.Version != 1 || descriptor.SourceSHA != i.OriginalSourceSHA || !shaRE.MatchString(descriptor.RuntimeSourceSHA) || descriptor.ToolSourceSHA != i.ToolSourceSHA || descriptor.OriginalRunID != i.OriginalRunID || descriptor.OperationID != i.OperationID || descriptor.ManifestSHA256 != i.ManifestSHA256 || descriptor.HostRole != "server-a" || descriptor.RemoteDescriptorSHA256 != "" || descriptor.BudgetTrustSHA256 != "" || !hashRE.MatchString(descriptor.MachineIDSHA256) || !hashRE.MatchString(descriptor.DockerSHA256) || (descriptor.DockerPath != "/usr/bin/docker" && descriptor.DockerPath != "/usr/local/bin/docker") || len(descriptor.Containers) < 1 || len(descriptor.Containers) > 32 || decodeLifecycleClosedProducer(result, &receipt) != nil || receipt.Kind != "qs_native_temporary_budget_key_result" || receipt.ToolSourceSHA != i.ToolSourceSHA || receipt.OperationID != i.OperationID || receipt.ActualRunID != i.ActualRunID || !hashRE.MatchString(receipt.PublicKey) || !receipt.KeyAvailable || receipt.WholeWriterFence || receipt.DropReady || receipt.ErrorCategory != "none" {
		return lifecycleError("lifecycle_preparation_budget_material_rejected")
	}
	return nil
}

func registerLifecyclePreparationBudgetMaterials(ctx context.Context, d *lifecycleMaterialDirectory, i lifecycleSourceCopyIntent, uid uint32) error {
	if i.BudgetDescriptorSHA256 == "" && i.BudgetResultSHA256 == "" {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || !hashRE.MatchString(i.BudgetDescriptorSHA256) || !hashRE.MatchString(i.BudgetResultSHA256) {
		return lifecycleError("lifecycle_preparation_budget_material_rejected")
	}
	var value json.RawMessage
	basis, e := readLifecycleProducerJSON(d, "budget-key.basis.private.json", i.BudgetDescriptorSHA256, uid, 256<<10, &value)
	if e != nil {
		return e
	}
	result, e := readLifecycleProducerJSON(d, "budget-key.result.private.json", i.BudgetResultSHA256, uid, 256<<10, &value)
	if e != nil {
		return e
	}
	return validateLifecyclePreparationBudgetBytes(basis, result, i)
}

// The final A descriptor and SSH channel are two exact source-owned inputs of
// the existing approved service control, separate from the original key basis.
func registerLifecycleHistoricalServiceInputs(ctx context.Context, d *lifecycleMaterialDirectory, r lifecycleRequest, uid uint32) error {
	if ctx == nil || ctx.Err() != nil || d == nil {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	if r.ServiceControl == nil {
		return nil
	}
	var value json.RawMessage
	for _, ref := range []struct{ name, hash string }{{"approved-services.json", r.ServiceControl.LocalDescriptorSHA256}, {"ssh-channel.json", r.ServiceControl.SSHChannelSHA256}} {
		if !hashRE.MatchString(ref.hash) {
			return lifecycleError("lifecycle_history_original_material_rejected")
		}
		if _, err := readLifecycleProducerJSON(d, ref.name, ref.hash, uid, 256<<10, &value); err != nil {
			return err
		}
	}
	return nil
}

type lifecyclePreparationRestoreEngine struct {
	Kind        string   `json:"kind"`
	Owner       string   `json:"owner"`
	ContainerID string   `json:"container_id"`
	ImageID     string   `json:"image_id"`
	Namespace   string   `json:"namespace"`
	Volumes     []string `json:"volumes"`
}
type lifecyclePreparationRestoreMaterial struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Mode   uint32 `json:"mode"`
}
type lifecyclePreparationRestoreZero struct {
	FormatVersion     int                                   `json:"format_version"`
	Kind              string                                `json:"kind"`
	OriginalSourceSHA string                                `json:"original_source_sha"`
	ToolSourceSHA     string                                `json:"tool_source_sha"`
	OperationID       string                                `json:"operation_id"`
	OriginalRunID     string                                `json:"original_run_id"`
	ActualRunID       string                                `json:"actual_run_id"`
	ManifestSHA256    string                                `json:"manifest_sha256"`
	ArchiveSHA256     string                                `json:"archive_sha256"`
	RequestSHA256     string                                `json:"request_sha256"`
	ElapsedMillis     int64                                 `json:"elapsed_millis"`
	Engines           []lifecyclePreparationRestoreEngine   `json:"engines"`
	Files             []lifecyclePreparationRestoreMaterial `json:"files"`
}
type lifecyclePreparationRestoreIntent struct {
	FormatVersion     int               `json:"format_version"`
	Kind              string            `json:"kind"`
	OriginalSourceSHA string            `json:"original_source_sha"`
	ToolSourceSHA     string            `json:"tool_source_sha"`
	OperationID       string            `json:"operation_id"`
	ActualRunID       string            `json:"actual_run_id"`
	ManifestSHA256    string            `json:"manifest_sha256"`
	ArchiveSHA256     string            `json:"archive_sha256"`
	Namespace         string            `json:"namespace"`
	Owner             string            `json:"owner"`
	ContainerName     string            `json:"container_name"`
	ImageID           string            `json:"image_id"`
	Architecture      string            `json:"architecture"`
	Labels            map[string]string `json:"labels"`
	ContainerLabels   map[string]string `json:"container_labels"`
	Volumes           []string          `json:"volumes"`
	ToolSHA256        string            `json:"tool_sha256"`
	Network           string            `json:"network"`
	DropAuthority     bool              `json:"drop_authority"`
	PurgeRequired     bool              `json:"purge_after_acceptance_required"`
}
type lifecyclePreparationRestoreCreated struct {
	ContainerID   string `json:"container_id"`
	Owner         string `json:"owner"`
	OperationID   string `json:"operation_id"`
	ActualRunID   string `json:"actual_run_id"`
	DropAuthority bool   `json:"drop_authority"`
}
type lifecyclePreparationRestoreRegistration struct {
	FormatVersion        int    `json:"format_version"`
	Kind                 string `json:"kind"`
	OriginalSourceSHA    string `json:"original_source_sha"`
	ToolSourceSHA        string `json:"tool_source_sha"`
	OperationID          string `json:"operation_id"`
	OriginalRunID        string `json:"original_run_id"`
	ActualRunID          string `json:"actual_run_id"`
	ManifestSHA256       string `json:"manifest_sha256"`
	ArchiveSHA256        string `json:"archive_sha256"`
	Namespace            string `json:"namespace"`
	MySQLOriginalUUID    string `json:"mysql_original_server_uuid_sha256"`
	MySQLRestoreUUID     string `json:"mysql_restore_server_uuid_sha256"`
	MongoOriginalProcess string `json:"mongodb_original_process_sha256"`
	MongoRestoreProcess  string `json:"mongodb_restore_process_sha256"`
	PurgeRequired        bool   `json:"purge_after_acceptance_required"`
	DropAuthority        bool   `json:"drop_authority"`
}

// Exact native producer members only. The same-process preparation supplies its
// real engines; a later host may reopen approved unchanged metadata, but cannot
// deserialize either an engine owner or a purge capability from this receipt.
func registerLifecyclePreparationRestoreMetadata(ctx context.Context, d *lifecycleMaterialDirectory, z *lifecyclePreparationRestoreZero, toolHash, architecture string, uid uint32, originalHashes map[string]string) ([]lifecyclePreparationRestoreMaterial, error) {
	if ctx == nil || ctx.Err() != nil || d == nil || z == nil || len(z.Engines) != 2 || len(z.Files) != 0 && len(z.Files) != 5 || len(z.Files) == 0 && originalHashes == nil || originalHashes != nil && len(originalHashes) != 5 || architecture != "amd64" && architecture != "arm64" {
		return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
	}
	var files []lifecyclePreparationRestoreMaterial
	read := func(name string, value any) error {
		index := len(files)
		hash := ""
		if originalHashes != nil {
			hash = originalHashes[name]
			if !hashRE.MatchString(hash) {
				return lifecycleError("lifecycle_preparation_release_receipt_rejected")
			}
		}
		if len(z.Files) == 5 {
			if z.Files[index].Name != name || !hashRE.MatchString(z.Files[index].SHA256) {
				return lifecycleError("lifecycle_preparation_release_receipt_rejected")
			}
			if hash != "" && hash != z.Files[index].SHA256 {
				return lifecycleError("lifecycle_preparation_release_receipt_rejected")
			}
			hash = z.Files[index].SHA256
		}
		raw, err := readLifecycleProducerJSON(d, name, hash, uid, 256<<10, value)
		if err != nil || decodeLifecycleClosedProducer(raw, value) != nil {
			return lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
		f := d.files[name]
		st, ok := infoStat(f.info)
		if !ok || st.Ino == 0 || f.info.Size() < 1 || f.info.Size() > 256<<10 {
			return lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
		member := lifecyclePreparationRestoreMaterial{name, f.hash, f.info.Size(), st.Uid, st.Gid, uint64(st.Dev), uint64(st.Ino), uint32(f.info.Mode().Perm())}
		if len(z.Files) == 5 && member != z.Files[index] {
			return lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
		files = append(files, member)
		return nil
	}
	for index, engine := range z.Engines {
		kind := "mysql"
		volumes := []string{"qs-retirement-data-" + engine.Owner}
		if index == 1 {
			kind = "mongodb"
			volumes = append(volumes, "qs-retirement-config-"+engine.Owner)
		}
		owner, err := hex.DecodeString(engine.Owner)
		namespace := "qs_retirement_restore_" + digestRaw([]byte(z.OriginalSourceSHA + "\n" + z.OperationID + "\n" + z.ActualRunID + "\n" + z.ManifestSHA256))[:24]
		if err != nil || len(owner) != 16 || engine.Kind != kind || !hashRE.MatchString(engine.ContainerID) || !strings.HasPrefix(engine.ImageID, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(engine.ImageID, "sha256:")) || !reflect.DeepEqual(engine.Volumes, volumes) || engine.Namespace != namespace || index == 1 && (engine.Owner == z.Engines[0].Owner || engine.ContainerID == z.Engines[0].ContainerID || engine.Namespace != z.Engines[0].Namespace) {
			return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
		labels := map[string]string{"codex.task": "qs-compatibility-retirement", "codex.owner": engine.Owner, "qs.retirement.operation": z.OperationID, "qs.retirement.run": z.ActualRunID}
		var intent lifecyclePreparationRestoreIntent
		if err = read("restore-"+engine.Owner+".intent.private.json", &intent); err != nil {
			return nil, err
		}
		if intent.FormatVersion != 1 || intent.Kind != "temporary_network_none_restore_intent" || intent.OriginalSourceSHA != z.OriginalSourceSHA || intent.ToolSourceSHA != z.ToolSourceSHA || intent.OperationID != z.OperationID || intent.ActualRunID != z.ActualRunID || intent.ManifestSHA256 != z.ManifestSHA256 || intent.ArchiveSHA256 != z.ArchiveSHA256 || intent.Namespace != engine.Namespace || intent.Owner != engine.Owner || intent.ContainerName != "qs-retirement-restore-"+engine.Owner || intent.ImageID != engine.ImageID || intent.Architecture != architecture || !reflect.DeepEqual(intent.Labels, labels) || !reflect.DeepEqual(intent.Volumes, engine.Volumes) || !hashRE.MatchString(intent.ToolSHA256) || toolHash != "" && intent.ToolSHA256 != toolHash || intent.Network != "none" || intent.DropAuthority || !intent.PurgeRequired {
			return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
		for name, value := range labels {
			if intent.ContainerLabels[name] != value {
				return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
			}
		}
		if f := d.files["restore-native"]; f != nil {
			if f.hash != intent.ToolSHA256 {
				return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
			}
		} else if err = d.register("restore-native", intent.ToolSHA256, uid, 0700); err != nil {
			return nil, err
		}
		var created lifecyclePreparationRestoreCreated
		if err = read("restore-"+engine.Owner+".created.private.json", &created); err != nil {
			return nil, err
		}
		if created.ContainerID != engine.ContainerID || created.Owner != engine.Owner || created.OperationID != z.OperationID || created.ActualRunID != z.ActualRunID || created.DropAuthority {
			return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
	}
	var registration lifecyclePreparationRestoreRegistration
	if err := read("lifecycle-restore-"+z.ActualRunID+".registration.private.json", &registration); err != nil {
		return nil, err
	}
	if registration.FormatVersion != 1 || registration.Kind != "temporary_isolated_restore_registration" || registration.OriginalSourceSHA != z.OriginalSourceSHA || registration.ToolSourceSHA != z.ToolSourceSHA || registration.OperationID != z.OperationID || registration.OriginalRunID != z.OriginalRunID || registration.ActualRunID != z.ActualRunID || registration.ManifestSHA256 != z.ManifestSHA256 || registration.ArchiveSHA256 != z.ArchiveSHA256 || registration.Namespace != z.Engines[0].Namespace || !hashRE.MatchString(registration.MySQLOriginalUUID) || !hashRE.MatchString(registration.MySQLRestoreUUID) || registration.MySQLOriginalUUID == registration.MySQLRestoreUUID || !hashRE.MatchString(registration.MongoOriginalProcess) || !hashRE.MatchString(registration.MongoRestoreProcess) || registration.MongoOriginalProcess == registration.MongoRestoreProcess || !registration.PurgeRequired || registration.DropAuthority {
		return nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
	}
	return files, nil
}

type lifecycleOriginalRootToolIntent struct {
	FormatVersion  int    `json:"format_version"`
	Kind           string `json:"kind"`
	Stage          string `json:"stage"`
	OperationID    string `json:"operation_id"`
	ActualRunID    string `json:"actual_run_id"`
	ToolSourceSHA  string `json:"tool_source_sha"`
	RequestPath    string `json:"request_path"`
	RequestSHA256  string `json:"request_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
	PackageSHA256  string `json:"package_sha256"`
	NativeSHA256   string `json:"native_sha256"`
	SourceUID      uint32 `json:"source_uid"`
	DropAuthority  bool   `json:"drop_authority"`
	PurgeRequired  bool   `json:"purge_after_acceptance_required"`
}

func lifecycleSourceCopyReferenceValid(r lifecycleRequest) bool {
	if r.SourceCopyIntent == nil || !hashRE.MatchString(r.SourceCopyIntent.SHA256) || filepath.Base(r.SourceCopyIntent.Path) != "source-copy.intent.private.json" {
		return false
	}
	root := filepath.Dir(r.SourceCopyIntent.Path)
	run := strings.TrimPrefix(filepath.Base(root), r.OperationID+"-")
	return runRE.MatchString(run) && run != r.ActualRunID && run != r.Approval.RunID && r.SourceCopyIntent.Path == filepath.Join(lifecycleRootBatch(r.OperationID, run), "source-copy.intent.private.json")
}

func lifecyclePreparationRestoreReferenceValid(r lifecycleRequest) bool {
	if !lifecycleSourceCopyReferenceValid(r) || r.PreparationRestoreZero == nil || !hashRE.MatchString(r.PreparationRestoreZero.SHA256) {
		return false
	}
	root := filepath.Dir(r.SourceCopyIntent.Path)
	run := strings.TrimPrefix(filepath.Base(root), r.OperationID+"-")
	return r.PreparationRestoreZero.Path == filepath.Join(root, "lifecycle-restore-"+run+".zero.private.json")
}

// Both original root producers write closed, non-null schemas. Source tuples
// and expected bytes authorize only reopening their exact files, never effects.
func decodeLifecycleClosedProducer(raw []byte, value any) error {
	t := reflect.TypeOf(value).Elem()
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, t) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != t.NumField() {
		return lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	for _, v := range fields {
		if strings.TrimSpace(string(v)) == "null" {
			return lifecycleError("lifecycle_staging_original_intent_rejected")
		}
	}
	if json.Unmarshal(raw, value) != nil {
		return lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	return nil
}

func openLifecycleOriginalRootStaging(ctx context.Context, r lifecycleRequest, inventory *lifecycleMaterialDirectory, sourceUID uint32) (*lifecycleMaterialDirectory, *lifecycleMaterialDirectory, error) {
	if !lifecycleSourceCopyReferenceValid(r) || inventory == nil || inventory.unchanged() != nil {
		return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	hashes := map[string]string{}
	for _, name := range lifecycleSourceNames {
		if inventory.files[name] == nil {
			return nil, nil, lifecycleError("lifecycle_material_registration_rejected")
		}
		hashes[name] = inventory.files[name].hash
	}
	root := filepath.Dir(r.SourceCopyIntent.Path)
	run := strings.TrimPrefix(filepath.Base(root), r.OperationID+"-")
	return openLifecycleRootStagingMaterialFiles(ctx, root, lifecycleInvocationBatch(r.OperationID, run), r, hashes, sourceUID, 0)
}

// The caller supplies the approved original intent, and the actual native
// inventory's seven hashes. ReadDir only rejects unexpected members. This
// final host holds new RO FDs to the unchanged original objects after the
// original preparation process has exited; no FD is fabricated from a report.
func openLifecycleRootStagingMaterialFiles(ctx context.Context, root, invocation string, r lifecycleRequest, hashes map[string]string, sourceUID, rootUID uint32) (owned, companion *lifecycleMaterialDirectory, result error) {
	if ctx == nil || ctx.Err() != nil || r.SourceCopyIntent == nil || r.SourceCopyIntent.Path != filepath.Join(root, "source-copy.intent.private.json") || len(hashes) != 7 {
		return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	d, err := openLifecycleMaterialDirectory(root, rootUID)
	if err != nil {
		return nil, nil, err
	}
	var peer *lifecycleMaterialDirectory
	defer func() {
		if result != nil {
			result = errors.Join(result, d.close())
			if peer != nil {
				result = errors.Join(result, peer.close())
			}
		}
	}()
	var i lifecycleSourceCopyIntent
	raw, err := readLifecycleProducerJSON(d, "source-copy.intent.private.json", r.SourceCopyIntent.SHA256, rootUID, 256<<10, &i)
	if err != nil {
		return nil, nil, err
	}
	if decodeLifecycleSourceCopyIntent(raw, &i) != nil || i.FormatVersion != 1 || i.Kind != "root_once_exact_source_copy_intent" || i.OriginalSourceSHA != r.OriginalSourceSHA || !shaRE.MatchString(i.ToolSourceSHA) || i.OperationID != r.OperationID || !runRE.MatchString(i.ActualRunID) || i.ActualRunID == r.ActualRunID || filepath.Base(root) != i.OperationID+"-"+i.ActualRunID || i.OriginalRunID != r.Approval.RunID || i.SourceUID != sourceUID || i.ManifestSHA256 != r.ManifestSHA256 || !hashRE.MatchString(i.RequestSHA256) || !reflect.DeepEqual(i.SourceFileSHA256, hashes) || !reflect.DeepEqual(i.Targets, targets) || i.ArchiveDirectory != r.ArchiveDirectory || i.SourceStagingDirectory != filepath.Join(root, "inventory-"+i.OriginalRunID) || i.DropAuthority || !i.PurgeRequired {
		return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	if err = registerLifecyclePreparationBudgetMaterials(ctx, d, i, rootUID); err != nil {
		return nil, nil, err
	}
	var toolRaw json.RawMessage
	toolBytes, err := readLifecycleProducerJSON(d, "tool.intent.private.json", "", rootUID, 256<<10, &toolRaw)
	if err != nil {
		return nil, nil, err
	}
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(toolBytes, &kind) != nil {
		return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	var nativeHash string
	var windowTool bool
	switch kind.Kind {
	case "approved_root_once_tool_staging":
		var v lifecycleOriginalRootToolIntent
		if decodeLifecycleClosedProducer(toolBytes, &v) != nil || v.FormatVersion != 1 || v.Stage != "lifecycle" || v.OperationID != i.OperationID || v.ActualRunID != i.ActualRunID || v.ToolSourceSHA != i.ToolSourceSHA || v.RequestPath != filepath.Join("/opt/backups/qs-server/compatibility-retirement", i.OperationID, "lifecycle-request.json") || v.RequestSHA256 != i.RequestSHA256 || v.ManifestSHA256 != i.ManifestSHA256 || !hashRE.MatchString(v.PackageSHA256) || v.SourceUID != i.SourceUID || v.DropAuthority || !v.PurgeRequired {
			return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
		}
		nativeHash = v.NativeSHA256
	case "independent_window_tool_native_invocation":
		v, e := decodeLifecycleAPIInvocationIntent(toolBytes)
		bIdentityValid := strings.HasPrefix(v.BImageID, "sha256:") && hashRE.MatchString(strings.TrimPrefix(v.BImageID, "sha256:")) && hashRE.MatchString(v.BProgramSHA256)
		if e != nil || v.FormatVersion != 1 || v.Stage != "prepare" || v.OperationID != i.OperationID || v.ActualRunID != i.ActualRunID || v.OriginalSourceSHA != i.OriginalSourceSHA || v.OriginalRunID != i.OriginalRunID || v.ToolSourceSHA != i.ToolSourceSHA || v.DerivedSHA256 != i.RequestSHA256 || v.ManifestSHA256 != i.ManifestSHA256 || v.NativePath != filepath.Join(root, "restore-native") || v.SourceUID != sourceUID || !shaRE.MatchString(v.DispatcherSourceSHA) || !hashRE.MatchString(v.TemplateSHA256) || !hashRE.MatchString(v.PackageSHA256) || !hashRE.MatchString(v.ToolProgramSHA256) || v.DropAuthority || !bIdentityValid {
			return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
		}
		nativeHash, windowTool = v.NativeSHA256, true
	default:
		return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	if err = d.register("restore-native", nativeHash, rootUID, 0700); err != nil {
		return nil, nil, err
	}
	if d.files["restore-native"].info.Size() < 1 || d.files["restore-native"].info.Size() > 64<<20 {
		return nil, nil, lifecycleError("lifecycle_material_registration_rejected")
	}
	if err = d.register("manifest.json", r.ManifestSHA256, rootUID, 0600); err != nil {
		return nil, nil, err
	}
	var original lifecycleRequest
	requestBytes, err := readLifecycleProducerJSON(d, "lifecycle-request.json", i.RequestSHA256, rootUID, 256<<10, &original)
	if err != nil {
		return nil, nil, err
	}
	original, err = decodeLifecycleStagingRequest(requestBytes)
	if err != nil || original.FormatVersion != 1 || original.Kind != "compatibility_retirement_lifecycle_request" || original.OperationID != i.OperationID || original.ActualRunID != i.ActualRunID || original.OriginalSourceSHA != i.OriginalSourceSHA || original.ToolSourceSHA != i.ToolSourceSHA || original.ManifestSHA256 != i.ManifestSHA256 || original.Approval != r.Approval || original.Recovery.ArchiveSHA256 != "" || original.Recovery.SourceSHA != i.OriginalSourceSHA || original.Recovery.OperationID != i.OperationID || original.Recovery.OriginalRunID != i.OriginalRunID || original.Recovery.ActualRunID != i.ActualRunID || original.ArchiveDirectory != i.ArchiveDirectory || original.SourceDirectory != filepath.Join("/opt/backups/qs-server/compatibility-retirement", i.OperationID, "inventory-"+i.OriginalRunID) || !reflect.DeepEqual(original.SourceFileSHA256, hashes) {
		return nil, nil, lifecycleError("lifecycle_staging_original_intent_rejected")
	}
	// The standalone producer's native engines are already removed. Its receipt
	// grants only reopening the six exact metadata files, never engine adoption.
	if r.PreparationRestoreZero == nil || r.PreparationRestoreZero.Path != filepath.Join(root, "lifecycle-restore-"+i.ActualRunID+".zero.private.json") || !hashRE.MatchString(r.PreparationRestoreZero.SHA256) || original.RestoreEngines == nil {
		return nil, nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
	}
	var zero lifecyclePreparationRestoreZero
	zeroRaw, err := readLifecycleProducerJSON(d, filepath.Base(r.PreparationRestoreZero.Path), r.PreparationRestoreZero.SHA256, rootUID, 256<<10, &zero)
	if err != nil || decodeLifecycleClosedProducer(zeroRaw, &zero) != nil || zero.FormatVersion != 1 || zero.Kind != "original_preparation_isolated_restore_zero" || zero.OriginalSourceSHA != i.OriginalSourceSHA || zero.ToolSourceSHA != i.ToolSourceSHA || zero.OperationID != i.OperationID || zero.OriginalRunID != i.OriginalRunID || zero.ActualRunID != i.ActualRunID || zero.ManifestSHA256 != i.ManifestSHA256 || zero.ArchiveSHA256 != r.Recovery.ArchiveSHA256 || zero.RequestSHA256 != i.RequestSHA256 || zero.ElapsedMillis < 0 || zero.ElapsedMillis > 600000 || len(zero.Files) != 5 || len(zero.Engines) != 2 {
		return nil, nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
	}
	var originalMembers struct {
		Files   []json.RawMessage `json:"files"`
		Engines []json.RawMessage `json:"engines"`
	}
	if json.Unmarshal(zeroRaw, &originalMembers) != nil || len(originalMembers.Files) != 5 || len(originalMembers.Engines) != 2 {
		return nil, nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
	}
	for n := range zero.Files {
		if decodeLifecycleClosedProducer(originalMembers.Files[n], &zero.Files[n]) != nil {
			return nil, nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
	}
	for n := range zero.Engines {
		if decodeLifecycleClosedProducer(originalMembers.Engines[n], &zero.Engines[n]) != nil || zero.Engines[n].ImageID != []string{original.RestoreEngines.MySQLImageID, original.RestoreEngines.MongoImageID}[n] {
			return nil, nil, lifecycleError("lifecycle_preparation_release_receipt_rejected")
		}
	}
	if _, err = registerLifecyclePreparationRestoreMetadata(ctx, d, &zero, nativeHash, original.RestoreEngines.Architecture, rootUID, nil); err != nil {
		return nil, nil, err
	}
	child, err := openLifecycleMaterialDirectory(i.SourceStagingDirectory, rootUID)
	if err != nil {
		return nil, nil, err
	}
	if err = d.registerChild(child); err != nil {
		return nil, nil, errors.Join(err, child.close())
	}
	for _, name := range lifecycleSourceNames {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if err = child.register(name, hashes[name], rootUID, 0600); err != nil {
			return nil, nil, err
		}
	}
	if windowTool {
		peer, err = openLifecycleMaterialDirectory(invocation, rootUID)
		if err != nil {
			return nil, nil, err
		}
		for name, hash := range map[string]string{"native-call.intent.private.json": digestRaw(toolBytes), "lifecycle-request.json": i.RequestSHA256, "manifest.json": r.ManifestSHA256} {
			if err = peer.register(name, hash, rootUID, 0600); err != nil {
				return nil, nil, err
			}
		}
		if err = peer.checkComplete(false); err != nil {
			return nil, nil, err
		}
	}
	if err = d.checkComplete(false); err != nil {
		return nil, nil, err
	}
	return d, peer, nil
}

type lifecycleHistoricalWriteMaterialReport struct {
	Protocol                     string `json:"protocol"`
	SourceSHA                    string `json:"source_sha"`
	ToolSourceSHA                string `json:"tool_source_sha"`
	OperationID                  string `json:"operation_id"`
	ActualRunID                  string `json:"actual_run_id"`
	RequestSHA256                string `json:"request_sha256"`
	DescriptorSHA256             string `json:"descriptor_sha256"`
	CommitState                  string `json:"commit_state"`
	MongoCommitRequirement       string `json:"mongo_commit_requirement"`
	AIOriginalCommands           uint64 `json:"ai_original_commands"`
	AISourceReferences           uint64 `json:"ai_source_references"`
	PreparedPages                uint64 `json:"prepared_pages"`
	ReadBackPages                uint64 `json:"readback_pages"`
	EventReferences              uint64 `json:"event_references"`
	ActualSQLCommitResponse      bool   `json:"actual_sql_commit_response"`
	ActualMongoCommitResponse    bool   `json:"actual_mongo_commit_response"`
	EventPersistenceObserved     bool   `json:"event_persistence_observed"`
	AICommandPersistenceComplete bool   `json:"ai_command_persistence_complete"`
	EvidenceWriteFinished        bool   `json:"evidence_write_finished"`
	WholeWriterFence             bool   `json:"whole_writer_fence"`
	FullExternalAIClosure        bool   `json:"full_external_ai_closure"`
	DropReady                    bool   `json:"drop_ready"`
	ErrorCategory                string `json:"error_category"`
	MaterialManifestSHA256       string `json:"material_manifest_sha256,omitempty"`
}

type lifecycleHistoricalMaterial struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Mode   uint32 `json:"mode"`
}
type lifecycleHistoricalMaterialManifest struct {
	Version         int                           `json:"version"`
	SourceSHA       string                        `json:"source_sha"`
	ToolSourceSHA   string                        `json:"tool_source_sha"`
	OperationID     string                        `json:"operation_id"`
	RunID           string                        `json:"run_id"`
	MaxSpoolBytes   int64                         `json:"max_spool_bytes"`
	JournalSequence uint64                        `json:"journal_sequence"`
	Files           []lifecycleHistoricalMaterial `json:"files"`
}

func lifecycleHistoricalWriteReferenceValid(r lifecycleRequest) bool {
	if r.HistoricalWriteReport == nil || !hashRE.MatchString(r.HistoricalWriteReport.SHA256) || filepath.Base(r.HistoricalWriteReport.Path) != "history.write.json" {
		return false
	}
	dir := filepath.Dir(r.HistoricalWriteReport.Path)
	run := strings.TrimPrefix(filepath.Base(dir), "history-write-")
	return runRE.MatchString(run) && run != r.ActualRunID && r.HistoricalWriteReport.Path == filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID, "history-write-"+run, "history.write.json")
}

func openLifecycleOriginalHistoricalWriteMaterials(ctx context.Context, r lifecycleRequest, uid uint32) (*lifecycleMaterialDirectory, error) {
	if !lifecycleHistoricalWriteReferenceValid(r) {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	return openLifecycleHistoricalWriteMaterialFiles(ctx, filepath.Dir(r.HistoricalWriteReport.Path), r, uid)
}

func openLifecycleHistoricalWriteMaterialFiles(ctx context.Context, path string, r lifecycleRequest, uid uint32) (owned *lifecycleMaterialDirectory, result error) {
	if ctx == nil || ctx.Err() != nil || r.HistoricalWriteReport == nil || r.HistoricalWriteReport.Path != filepath.Join(path, "history.write.json") {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	d, err := openLifecycleMaterialDirectory(path, uid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, d.close())
		}
	}()
	var report lifecycleHistoricalWriteMaterialReport
	reportRaw, err := readLifecycleProducerJSON(d, "history.write.json", r.HistoricalWriteReport.SHA256, uid, 256<<10, &report)
	if err != nil {
		return nil, err
	}
	// These are the original producer's metadata and known terminal response,
	// never a capability for CAS, Q, the writer fence or business acceptance.
	if decodeLifecycleClosedProducer(reportRaw, &report) != nil || report.Protocol != "qs-compatibility-evidence-write/v1" || report.SourceSHA != r.OriginalSourceSHA || report.OperationID != r.OperationID || !shaRE.MatchString(report.ToolSourceSHA) || !runRE.MatchString(report.ActualRunID) || report.ActualRunID == r.ActualRunID || filepath.Base(path) != "history-write-"+report.ActualRunID || !hashRE.MatchString(report.RequestSHA256) || !hashRE.MatchString(report.DescriptorSHA256) || !hashRE.MatchString(report.MaterialManifestSHA256) || report.ErrorCategory != "none" || !report.EvidenceWriteFinished || !report.ActualSQLCommitResponse || report.DropReady || report.WholeWriterFence || report.FullExternalAIClosure {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	knownCommit := report.MongoCommitRequirement == "required" && report.CommitState == "both_responses_success_non_atomic" && report.ActualMongoCommitResponse && report.EventReferences > 0 && report.PreparedPages > 0 && report.EventPersistenceObserved || report.MongoCommitRequirement == "not_required" && report.CommitState == "sql_committed_mongo_not_required" && !report.ActualMongoCommitResponse && report.EventReferences == 0 && report.PreparedPages == 0 && !report.EventPersistenceObserved && report.AIOriginalCommands > 0
	if !knownCommit || report.PreparedPages != report.ReadBackPages || report.AIOriginalCommands > 0 && !report.AICommandPersistenceComplete {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if err = d.register("history.materials.private.json", report.MaterialManifestSHA256, uid, 0600); err != nil {
		return nil, err
	}
	f := d.files["history.materials.private.json"]
	if f.info.Size() < 1 || f.info.Size() > 64<<20 {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if _, err = f.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f.file, (64<<20)+1))
	if err != nil || int64(len(raw)) != f.info.Size() || digestRaw(raw) != f.hash || d.checkFile(f) != nil {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var manifest lifecycleHistoricalMaterialManifest
	if decodeLifecycleClosedProducer(raw, &manifest) != nil || manifest.SourceSHA != report.SourceSHA || manifest.ToolSourceSHA != report.ToolSourceSHA || manifest.OperationID != report.OperationID || manifest.RunID != report.ActualRunID || manifest.MaxSpoolBytes != 16<<30 || manifest.JournalSequence > 1<<20 {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	// The source-bound producer version determines the entire spool namespace.
	// Never adopt names from a directory or combine an old prepared epoch with
	// the new two-input/owner protocol. Reports still grant no write authority.
	var spoolNames []string
	switch manifest.Version {
	case 1:
		spoolNames = []string{"prepared-mongo-private.bin", "prepared-sql-private.bin"}
	case 2:
		spoolNames = []string{"input-mongo-1.private.bin", "input-source-1.private.bin", "input-ai-1.private.bin", "input-mongo-2.private.bin", "input-source-2.private.bin", "input-ai-2.private.bin", "input-owner-sql.private.bin"}
	default:
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if len(manifest.Files) != int(manifest.JournalSequence)+len(spoolNames) {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var rawMembers struct {
		Files []json.RawMessage `json:"files"`
	}
	if json.Unmarshal(raw, &rawMembers) != nil || len(rawMembers.Files) != len(manifest.Files) {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	for index, member := range manifest.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		name := "journal-" + strconv.Itoa(index-len(spoolNames)+1) + ".json"
		maximum := int64(64 << 10)
		if index < len(spoolNames) {
			name = spoolNames[index]
			maximum = 16 << 30
		}
		if decodeLifecycleClosedProducer(rawMembers.Files[index], &member) != nil || member.Name != name || !hashRE.MatchString(member.SHA256) || member.Bytes < 0 || member.Bytes > maximum || member.UID != uid || member.Mode != 0600 || member.Inode == 0 {
			return nil, lifecycleError("lifecycle_history_original_material_rejected")
		}
		if index < len(spoolNames) {
			err = d.registerHistoricalCASSpool(name, member.SHA256, uid)
		} else {
			err = d.register(name, member.SHA256, uid, 0600)
		}
		if err != nil {
			return nil, err
		}
		registered := d.files[name]
		st, ok := infoStat(registered.info)
		if !ok || registered.info.Size() != member.Bytes || st.Gid != member.GID || uint64(st.Dev) != member.Device || uint64(st.Ino) != member.Inode {
			return nil, lifecycleError("lifecycle_history_original_material_rejected")
		}
	}
	if err = d.checkComplete(false); err != nil {
		return nil, err
	}
	return d, nil
}

// These project only the existing original producer's schemas. They reopen
// exact hash-bound files; none of their flags is a fence or acceptance proof.
type lifecycleHistoricalRequestMaterial struct {
	FormatVersion    int                               `json:"format_version"`
	Kind             string                            `json:"kind"`
	SourceSHA        string                            `json:"source_sha"`
	OperationID      string                            `json:"operation_id"`
	RunID            string                            `json:"run_id"`
	InventoryRequest lifecycleFinalFileBinding         `json:"inventory_request"`
	InventoryReport  lifecycleFinalFileBinding         `json:"inventory_report"`
	Assets           []lifecycleHistoricalRequestAsset `json:"assets"`
}
type lifecycleHistoricalRequestAsset struct {
	Database string `json:"database"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA256   string `json:"full_file_sha256"`
	Bytes    uint64 `json:"full_file_bytes"`
}
type lifecycleHistoricalInputMaterial struct {
	FormatVersion          int                        `json:"format_version"`
	Kind                   string                     `json:"kind"`
	SourceSHA              string                     `json:"source_sha"`
	ToolSourceSHA          string                     `json:"tool_source_sha"`
	OperationID            string                     `json:"operation_id"`
	ActualRunID            string                     `json:"actual_run_id"`
	Mode                   string                     `json:"mode"`
	RequestSHA256          string                     `json:"request_sha256"`
	RuntimeSourceSHA       string                     `json:"runtime_source_sha"`
	ImageID                string                     `json:"image_id"`
	ContainerID            string                     `json:"container_id"`
	RuntimeBindingSHA256   string                     `json:"approved_runtime_binding_sha256"`
	AssetsDirectory        string                     `json:"assets_directory"`
	ExpectedAIIdentityHash string                     `json:"expected_ai_identity_hash"`
	ExpectedAIHead         string                     `json:"expected_ai_head"`
	AIBounds               *lifecycleFinalFileBinding `json:"ai_bounds,omitempty"`
	PeerBounds             *lifecycleFinalFileBinding `json:"peer_bounds,omitempty"`
	Protection             *lifecycleFinalFileBinding `json:"protection,omitempty"`
}
type lifecycleHistoricalInputRegistration struct {
	FormatVersion           int    `json:"format_version"`
	Kind                    string `json:"kind"`
	SourceSHA               string `json:"source_sha"`
	ToolSourceSHA           string `json:"tool_source_sha"`
	OperationID             string `json:"operation_id"`
	ActualRunID             string `json:"actual_run_id"`
	ApprovalSHA256          string `json:"approval_sha256"`
	ParentRunID             string `json:"parent_run_id"`
	ParentRequestSHA256     string `json:"parent_request_sha256"`
	DerivedRequestSHA256    string `json:"derived_request_sha256"`
	DescriptorSHA256        string `json:"descriptor_sha256"`
	BinarySHA256            string `json:"history_binary_sha256"`
	OnlyParentFieldReplaced string `json:"only_parent_field_replaced"`
}
type lifecycleHistoricalAIBoundsMaterial struct {
	Protocol               string                              `json:"protocol"`
	Mode                   string                              `json:"mode"`
	SourceSHA              string                              `json:"source_sha"`
	ToolSourceSHA          string                              `json:"tool_source_sha,omitempty"`
	OperationID            string                              `json:"operation_id"`
	ActualRunID            string                              `json:"actual_run_id"`
	ExternalRunID          string                              `json:"external_run_id"`
	RequestSHA256          string                              `json:"request_sha256"`
	DescriptorSHA256       string                              `json:"descriptor_sha256"`
	ExpectedRuntimeSHA256  string                              `json:"expected_runtime_binding_sha256"`
	RuntimeSHA256          string                              `json:"runtime_binding_sha256"`
	Bounds                 *retirement.AIExternalBoundsSummary `json:"bounds"`
	ErrorCategory          string                              `json:"error_category"`
	DiagnosticOnly         bool                                `json:"diagnostic_only"`
	DiagnosticReadComplete bool                                `json:"diagnostic_read_complete"`
	Complete               bool                                `json:"complete"`
	IndependentApproval    bool                                `json:"independent_approval"`
	WholeWriterFence       bool                                `json:"whole_writer_fence"`
	CASAuthority           bool                                `json:"cas_authority"`
	RetirementWritten      bool                                `json:"retirement_written"`
	DropReady              bool                                `json:"drop_ready"`
	RequiredAdapters       []string                            `json:"required_adapters"`
}
type lifecycleHistoricalAIBootstrapRegistration struct {
	FormatVersion           int    `json:"format_version"`
	Kind                    string `json:"kind"`
	SourceSHA               string `json:"source_sha"`
	OriginalSourceSHA       string `json:"original_source_sha,omitempty"`
	ToolSourceSHA           string `json:"tool_source_sha,omitempty"`
	OperationID             string `json:"operation_id"`
	ActualRunID             string `json:"actual_run_id"`
	Mode                    string `json:"mode"`
	ApprovalSHA256          string `json:"approval_sha256"`
	ParentRunID             string `json:"parent_run_id"`
	ParentRequestSHA256     string `json:"parent_request_sha256"`
	DerivedRequestSHA256    string `json:"derived_request_sha256"`
	DescriptorSHA256        string `json:"descriptor_sha256"`
	BinarySHA256            string `json:"history_binary_sha256"`
	OnlyParentFieldReplaced string `json:"only_parent_field_replaced"`
}

// The original AI producers have two known wire shapes: original source, or
// source-bound newer tool. Only the absent optional strings are normalized for
// decoding; the registered original bytes/hash/inode never change.
func decodeLifecycleHistoricalAIProducer(raw []byte, value any, optional ...string) error {
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &fields) != nil {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	found := 0
	for _, name := range optional {
		if _, ok := fields[name]; ok {
			found++
		}
	}
	if found != 0 && found != len(optional) {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	if found == 0 {
		for _, name := range optional {
			fields[name] = json.RawMessage(`""`)
		}
	}
	normalized, err := json.Marshal(fields)
	if err != nil || decodeLifecycleClosedProducer(normalized, value) != nil {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	return nil
}
func decodeLifecycleHistoricalRequest(raw []byte, q *lifecycleHistoricalRequestMaterial) error {
	if decodeLifecycleClosedProducer(raw, q) != nil {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	var members struct {
		InventoryRequest json.RawMessage   `json:"inventory_request"`
		InventoryReport  json.RawMessage   `json:"inventory_report"`
		Assets           []json.RawMessage `json:"assets"`
	}
	if json.Unmarshal(raw, &members) != nil || len(members.Assets) != 4 || decodeLifecycleClosedProducer(members.InventoryRequest, &q.InventoryRequest) != nil || decodeLifecycleClosedProducer(members.InventoryReport, &q.InventoryReport) != nil {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	for n := range members.Assets {
		if decodeLifecycleClosedProducer(members.Assets[n], &q.Assets[n]) != nil {
			return lifecycleError("lifecycle_history_original_material_rejected")
		}
	}
	return nil
}
func decodeLifecycleHistoricalBoundsInput(raw []byte, v *lifecycleHistoricalInputMaterial) error {
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(*v)) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 14 && len(fields) != 15 {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	for _, name := range []string{"format_version", "kind", "source_sha", "operation_id", "actual_run_id", "mode", "request_sha256", "runtime_source_sha", "image_id", "container_id", "approved_runtime_binding_sha256", "assets_directory", "expected_ai_identity_hash", "expected_ai_head"} {
		if b, ok := fields[name]; !ok || strings.TrimSpace(string(b)) == "null" {
			return lifecycleError("lifecycle_history_original_material_rejected")
		}
	}
	for name, b := range fields {
		if strings.TrimSpace(string(b)) == "null" || name == "ai_bounds" || name == "peer_bounds" || name == "protection" {
			return lifecycleError("lifecycle_history_original_material_rejected")
		}
	}
	if json.Unmarshal(raw, v) != nil {
		return lifecycleError("lifecycle_history_original_material_rejected")
	}
	return nil
}
func readLifecycleHistoricalHeldJSON(d *lifecycleMaterialDirectory, name string, maximum int64, value any) ([]byte, error) {
	if d == nil || d.files[name] == nil || d.files[name].info.Size() < 1 || d.files[name].info.Size() > maximum {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	f := d.files[name]
	if d.checkFile(f) != nil {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if _, err := f.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f.file, maximum+1))
	if err != nil || int64(len(raw)) > maximum || digestRaw(raw) != f.hash || json.Unmarshal(raw, value) != nil || d.checkFile(f) != nil {
		return nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	return raw, nil
}
func lifecycleHistoricalRequestMatches(q lifecycleHistoricalRequestMaterial, r lifecycleRequest, inventory *lifecycleMaterialDirectory, operationRoot, run string) bool {
	if inventory == nil || inventory.unchanged() != nil || q.FormatVersion != 1 || q.Kind != "readonly_compatibility_history_request" || q.SourceSHA != r.OriginalSourceSHA || q.OperationID != r.OperationID || q.RunID != run || q.InventoryRequest != (lifecycleFinalFileBinding{filepath.Join(operationRoot, "inventory-request.json"), r.Approval.RequestHash}) || q.InventoryReport != (lifecycleFinalFileBinding{filepath.Join(operationRoot, "inventory-"+r.Approval.RunID, "inventory.private.json"), r.Approval.InventorySHA256}) || len(q.Assets) != 4 {
		return false
	}
	for n, asset := range q.Assets {
		f := inventory.files[lifecycleSourceNames[3+n]]
		if f == nil || inventory.checkFile(f) != nil || asset.Database != targets[n][0] || asset.Name != targets[n][1] || asset.Path != filepath.Join(operationRoot, "inventory-"+r.Approval.RunID, lifecycleSourceNames[3+n]) || asset.SHA256 != f.hash || asset.Bytes != uint64(f.info.Size()) {
			return false
		}
	}
	return true
}
func openLifecycleOriginalHistoricalWriteInputs(ctx context.Context, r lifecycleRequest, writer, inventory *lifecycleMaterialDirectory, uid uint32) (*lifecycleMaterialDirectory, []*lifecycleMaterialDirectory, error) {
	if !lifecycleHistoricalWriteReferenceValid(r) || writer == nil || writer.path != filepath.Dir(r.HistoricalWriteReport.Path) {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	return openLifecycleHistoricalWriteInputFiles(ctx, r, writer, inventory, filepath.Dir(writer.path), uid)
}

// The operation parent is deliberately partial. Its three original inputs are
// held by real FDs, but other approved run directories, locks and native journals
// must be composed by their actual owners before the full catalog can succeed.
func openLifecycleHistoricalWriteInputFiles(ctx context.Context, r lifecycleRequest, writer, inventory *lifecycleMaterialDirectory, operationRoot string, uid uint32) (registration *lifecycleMaterialDirectory, previous []*lifecycleMaterialDirectory, result error) {
	if ctx == nil || ctx.Err() != nil || writer == nil || writer.unchanged() != nil || inventory == nil || r.HistoricalWriteReport == nil || writer.files["history.write.json"] == nil || writer.files["history.write.json"].hash != r.HistoricalWriteReport.SHA256 || filepath.Dir(writer.path) != operationRoot {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var report lifecycleHistoricalWriteMaterialReport
	raw, err := readLifecycleHistoricalHeldJSON(writer, "history.write.json", 256<<10, &report)
	if err != nil || decodeLifecycleClosedProducer(raw, &report) != nil || report.SourceSHA != r.OriginalSourceSHA || report.OperationID != r.OperationID || !runRE.MatchString(report.ActualRunID) || filepath.Base(writer.path) != "history-write-"+report.ActualRunID {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	opened := []*lifecycleMaterialDirectory{}
	defer func() {
		if result != nil {
			for _, d := range opened {
				result = errors.Join(result, d.close())
			}
			registration = nil
			previous = nil
		}
	}()
	open := func(path string) (*lifecycleMaterialDirectory, error) {
		d, e := openLifecycleMaterialDirectory(path, uid)
		if e == nil {
			opened = append(opened, d)
		}
		return d, e
	}
	registration, err = open(filepath.Join(operationRoot, "history-write-registration-"+report.ActualRunID))
	if err != nil {
		return nil, nil, err
	}
	var request lifecycleHistoricalRequestMaterial
	requestRaw, err := readLifecycleProducerJSON(registration, "history.request.json", report.RequestSHA256, uid, 256<<10, &request)
	if err != nil || decodeLifecycleHistoricalRequest(requestRaw, &request) != nil || !lifecycleHistoricalRequestMatches(request, r, inventory, operationRoot, report.ActualRunID) {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var input lifecycleHistoricalInputMaterial
	inputRaw, err := readLifecycleProducerJSON(registration, "write.input.json", report.DescriptorSHA256, uid, 256<<10, &input)
	if err != nil || decodeLifecycleClosedProducer(inputRaw, &input) != nil || input.FormatVersion != 1 || input.Kind != "historical_evidence_write_host_input" || input.Mode != "write" || input.SourceSHA != report.SourceSHA || input.ToolSourceSHA != report.ToolSourceSHA || input.OperationID != r.OperationID || input.ActualRunID != report.ActualRunID || input.RequestSHA256 != report.RequestSHA256 || !shaRE.MatchString(input.RuntimeSourceSHA) || !strings.HasPrefix(input.ImageID, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(input.ImageID, "sha256:")) || !hashRE.MatchString(input.ContainerID) || !hashRE.MatchString(input.RuntimeBindingSHA256) || !filepath.IsAbs(input.AssetsDirectory) || filepath.Clean(input.AssetsDirectory) != input.AssetsDirectory || input.ExpectedAIIdentityHash != "" || input.ExpectedAIHead != "" || input.AIBounds == nil || input.PeerBounds == nil || input.Protection == nil {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var inputRefs map[string]json.RawMessage
	if json.Unmarshal(inputRaw, &inputRefs) != nil || decodeLifecycleClosedProducer(inputRefs["ai_bounds"], input.AIBounds) != nil || decodeLifecycleClosedProducer(inputRefs["peer_bounds"], input.PeerBounds) != nil || decodeLifecycleClosedProducer(inputRefs["protection"], input.Protection) != nil {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var record lifecycleHistoricalInputRegistration
	recordRaw, err := readLifecycleProducerJSON(registration, "write.registration.json", "", uid, 256<<10, &record)
	if err != nil || decodeLifecycleClosedProducer(recordRaw, &record) != nil || record.FormatVersion != 1 || record.Kind != "historical_evidence_write_registration" || record.SourceSHA != report.SourceSHA || record.ToolSourceSHA != report.ToolSourceSHA || record.OperationID != r.OperationID || record.ActualRunID != report.ActualRunID || !hashRE.MatchString(record.ApprovalSHA256) || record.ParentRunID != r.Approval.RunID || !hashRE.MatchString(record.ParentRequestSHA256) || record.DerivedRequestSHA256 != report.RequestSHA256 || record.DescriptorSHA256 != report.DescriptorSHA256 || !hashRE.MatchString(record.BinarySHA256) || record.OnlyParentFieldReplaced != "run_id" {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if err = registration.checkComplete(false); err != nil {
		return nil, nil, err
	}
	for _, ref := range []*lifecycleFinalFileBinding{input.AIBounds, input.PeerBounds, input.Protection} {
		if !hashRE.MatchString(ref.SHA256) {
			return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
		}
	}
	boundsRoot := filepath.Dir(input.AIBounds.Path)
	boundsRun := strings.TrimPrefix(filepath.Base(boundsRoot), "ai-host-bounds-")
	if !runRE.MatchString(boundsRun) || boundsRun == report.ActualRunID || input.AIBounds.Path != filepath.Join(operationRoot, "ai-host-bounds-"+boundsRun, "ai.bounds.json") || input.PeerBounds.Path != filepath.Join(boundsRoot, "peer.bounds.json") || input.Protection.Path != filepath.Join(operationRoot, "ai-message-protection.json") {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	bounds, err := open(boundsRoot)
	if err != nil {
		return nil, nil, err
	}
	previous = append(previous, bounds)
	if err = bounds.register("ai.bounds.json", input.AIBounds.SHA256, uid, 0600); err != nil {
		return nil, nil, err
	}
	if err = bounds.register("peer.bounds.json", input.PeerBounds.SHA256, uid, 0600); err != nil {
		return nil, nil, err
	}
	if bounds.files["ai.bounds.json"].info.Size() > 4<<20 || bounds.files["peer.bounds.json"].info.Size() > 4<<20 {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var ai lifecycleHistoricalAIBoundsMaterial
	aiRaw, err := readLifecycleProducerJSON(bounds, "ai-host.readiness.json", "", uid, 32768, &ai)
	if err != nil || decodeLifecycleHistoricalAIProducer(aiRaw, &ai, "tool_source_sha") != nil || ai.Protocol != "qs-compatibility-ai-host-readonly/v1" || ai.Mode != "bounds" || ai.SourceSHA != r.OriginalSourceSHA || ai.ToolSourceSHA != "" && !shaRE.MatchString(ai.ToolSourceSHA) || ai.OperationID != r.OperationID || ai.ActualRunID != boundsRun || ai.ExternalRunID != strings.Split(boundsRun, "-")[0] || !hashRE.MatchString(ai.RequestSHA256) || !hashRE.MatchString(ai.DescriptorSHA256) || ai.ExpectedRuntimeSHA256 != input.RuntimeBindingSHA256 || ai.RuntimeSHA256 != input.RuntimeBindingSHA256 || ai.ErrorCategory != "none" || !ai.DiagnosticOnly || !ai.DiagnosticReadComplete || ai.Complete || ai.IndependentApproval || ai.WholeWriterFence || ai.CASAuthority || ai.RetirementWritten || ai.DropReady || ai.Bounds == nil {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var aiMembers map[string]json.RawMessage
	if json.Unmarshal(aiRaw, &aiMembers) != nil || decodeLifecycleClosedProducer(aiMembers["bounds"], ai.Bounds) != nil {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	b := ai.Bounds
	if b.Scope != "diagnostic-unapproved-bounds-only" || !hashRE.MatchString(b.SourceSHA256) || b.RuntimeBindingSHA256 != input.RuntimeBindingSHA256 || b.AIBoundsSHA256 != input.AIBounds.SHA256 || b.PeerBoundsSHA256 != input.PeerBounds.SHA256 || b.AIPhysicalObjects != 44 && b.AIPhysicalObjects != 53 || b.AILogicalObjects != 53 || b.PeerObjects != 14 || b.IndependentEpochs != 2 || b.IndependentApproval || b.BusinessClosure || b.WriterFence || b.BrokerCoverage || b.CASAuthority || b.RetirementWritten || b.RecoveryAuthority || b.DropReady {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if err = bounds.checkComplete(false); err != nil {
		return nil, nil, err
	}
	bootstrap, err := open(filepath.Join(operationRoot, "ai-bootstrap-bounds-"+boundsRun))
	if err != nil {
		return nil, nil, err
	}
	previous = append(previous, bootstrap)
	var aiRequest lifecycleHistoricalRequestMaterial
	aiRequestRaw, err := readLifecycleProducerJSON(bootstrap, "history.request.json", ai.RequestSHA256, uid, 256<<10, &aiRequest)
	if err != nil || decodeLifecycleHistoricalRequest(aiRequestRaw, &aiRequest) != nil || !lifecycleHistoricalRequestMatches(aiRequest, r, inventory, operationRoot, boundsRun) {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var aiInput lifecycleHistoricalInputMaterial
	aiInputRaw, err := readLifecycleProducerJSON(bootstrap, "ai-host.input.json", ai.DescriptorSHA256, uid, 256<<10, &aiInput)
	if err != nil || decodeLifecycleHistoricalBoundsInput(aiInputRaw, &aiInput) != nil || aiInput.FormatVersion != 1 || aiInput.Kind != "readonly_ai_external_host_input" || aiInput.SourceSHA != ai.SourceSHA || aiInput.ToolSourceSHA != ai.ToolSourceSHA || aiInput.OperationID != r.OperationID || aiInput.ActualRunID != boundsRun || aiInput.Mode != "bounds" || aiInput.RequestSHA256 != ai.RequestSHA256 || aiInput.RuntimeSourceSHA != input.RuntimeSourceSHA || aiInput.ImageID != input.ImageID || aiInput.ContainerID != input.ContainerID || aiInput.RuntimeBindingSHA256 != input.RuntimeBindingSHA256 || !filepath.IsAbs(aiInput.AssetsDirectory) || filepath.Clean(aiInput.AssetsDirectory) != aiInput.AssetsDirectory || aiInput.AIBounds != nil || aiInput.PeerBounds != nil || aiInput.Protection != nil || (aiInput.ExpectedAIIdentityHash == "") != (aiInput.ExpectedAIHead == "") || aiInput.ExpectedAIIdentityHash != "" && (!hashRE.MatchString(aiInput.ExpectedAIIdentityHash) || aiInput.ExpectedAIHead != "0038_messaging_observations" && aiInput.ExpectedAIHead != "0040_module_table_names") {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	var aiRecord lifecycleHistoricalAIBootstrapRegistration
	aiRecordRaw, err := readLifecycleProducerJSON(bootstrap, "ai-host.registration.json", "", uid, 256<<10, &aiRecord)
	if err != nil || decodeLifecycleHistoricalAIProducer(aiRecordRaw, &aiRecord, "original_source_sha", "tool_source_sha") != nil || aiRecord.FormatVersion != 1 || aiRecord.Kind != "readonly_ai_host_derivation_registration" || aiRecord.SourceSHA != ai.SourceSHA || aiRecord.ToolSourceSHA != ai.ToolSourceSHA || aiRecord.OriginalSourceSHA != "" && aiRecord.OriginalSourceSHA != ai.SourceSHA || aiRecord.OperationID != r.OperationID || aiRecord.ActualRunID != boundsRun || aiRecord.Mode != "bounds" || !hashRE.MatchString(aiRecord.ApprovalSHA256) || aiRecord.ParentRunID != record.ParentRunID || aiRecord.ParentRequestSHA256 != record.ParentRequestSHA256 || aiRecord.DerivedRequestSHA256 != ai.RequestSHA256 || aiRecord.DescriptorSHA256 != ai.DescriptorSHA256 || !hashRE.MatchString(aiRecord.BinarySHA256) || aiRecord.OnlyParentFieldReplaced != "run_id" {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if err = bootstrap.checkComplete(false); err != nil {
		return nil, nil, err
	}
	parent, err := open(operationRoot)
	if err != nil {
		return nil, nil, err
	}
	previous = append(previous, parent)
	var original lifecycleHistoricalRequestMaterial
	parentRaw, err := readLifecycleProducerJSON(parent, "history-request.json", record.ParentRequestSHA256, uid, 256<<10, &original)
	if err != nil || decodeLifecycleHistoricalRequest(parentRaw, &original) != nil || !lifecycleHistoricalRequestMatches(original, r, inventory, operationRoot, record.ParentRunID) {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	if err = parent.register("inventory-request.json", r.Approval.RequestHash, uid, 0600); err != nil {
		return nil, nil, err
	}
	if err = parent.register("ai-message-protection.json", input.Protection.SHA256, uid, 0600); err != nil {
		return nil, nil, err
	}
	if parent.files["ai-message-protection.json"].info.Size() < 1 || parent.files["ai-message-protection.json"].info.Size() > 256<<10 {
		return nil, nil, lifecycleError("lifecycle_history_original_material_rejected")
	}
	for _, d := range opened {
		for _, f := range d.files {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if err = d.checkFile(f); err != nil {
				return nil, nil, err
			}
		}
	}
	// Only registered objects are returned. The parent retains unregistered
	// responsibilities; its complete-scope check remains mandatory and closed.
	return registration, previous, nil
}

// Complete only the original, source-owned producer namespace. References are
// followed from already approved inventory/write inputs, never from a directory
// listing. This creates no execution, business, fence or deletion permission.
func completeLifecycleOriginalOperationMaterials(ctx context.Context, r lifecycleRequest, parent, inventory, writer, registration *lifecycleMaterialDirectory, previous []*lifecycleMaterialDirectory, archive *lifecycleMaterialDirectory, uid uint32) error {
	if ctx == nil || ctx.Err() != nil || parent == nil || parent.path != filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID) || parent.unchanged() != nil || inventory == nil || writer == nil || registration == nil || parent.files["history-request.json"] == nil || parent.files["inventory-request.json"] == nil {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	if e := registerLifecycleOriginalOperationLock(parent, uid); e != nil {
		return e
	}
	for _, d := range append([]*lifecycleMaterialDirectory{inventory, writer, registration, archive}, previous...) {
		if d == nil || d == parent {
			continue
		}
		if filepath.Dir(d.path) != parent.path {
			if d.externalArchive {
				continue
			}
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		if parent.children[filepath.Base(d.path)] == d {
			continue
		}
		if e := parent.registerChild(d); e != nil {
			return e
		}
	}
	if e := completeLifecycleOriginalInventoryInputs(ctx, r, parent, uid); e != nil {
		return e
	}
	if e := completeLifecycleOriginalMetadataInputs(ctx, r, parent, uid); e != nil {
		return e
	}
	if e := registerLifecycleOriginalAIExecMaterials(ctx, r, parent, uid); e != nil {
		return e
	}
	if e := registerLifecycleOriginalDatabaseSourceInput(ctx, r, parent, uid); e != nil {
		return e
	}
	if r.SourceCopyIntent == nil {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	raw, e := readLifecycleOwnedBytes(r.SourceCopyIntent.Path, r.SourceCopyIntent.SHA256, 0, 256<<10)
	if e != nil {
		return e
	}
	var intent lifecycleSourceCopyIntent
	if decodeLifecycleSourceCopyIntent(raw, &intent) != nil || intent.SourceUID != uid || intent.OperationID != r.OperationID || intent.OriginalSourceSHA != r.OriginalSourceSHA || intent.ManifestSHA256 != r.ManifestSHA256 {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	if e = registerLifecycleOriginalFrozenFile(parent, "lifecycle-request.json", intent.RequestSHA256, uid); e != nil {
		return e
	}
	if r.WriterControl != nil {
		if !r.WriterControl.valid() {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		if e = registerLifecycleOriginalFrozenFile(parent, "approved-workflow-scope.json", r.WriterControl.WorkflowScopeSHA256, uid); e != nil {
			return e
		}
	}
	if e = registerLifecycleOriginalFrozenFile(parent, "manifest.json", r.ManifestSHA256, uid); e != nil {
		return e
	}
	var manifest lifecycleFrozenManifest
	raw, e = readLifecycleHistoricalHeldJSON(parent, "manifest.json", 256<<10, &manifest)
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(manifest)) != nil || manifest.FormatVersion != 1 || manifest.OperationID != r.OperationID || manifest.SourceSHA != r.OriginalSourceSHA || manifest.TargetHash != digest(targets) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for kind, ref := range manifest.Evidence {
		switch kind {
		case "inventory", "history", "fence", "backup_restore", "release", "acceptance":
		default:
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		if filepath.Base(ref.Filename) != ref.Filename || ref.Filename == "manifest.json" || !hashRE.MatchString(ref.SHA256) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		var proof map[string]json.RawMessage
		b, e := readLifecycleOriginalProducerJSON(ctx, parent, ref.Filename, ref.SHA256, uid, "format_version kind operation_id source_sha target_hash producer complete observed_at valid_until summary")
		if e != nil {
			return e
		}
		proof = b
		if originalMaterialText(proof, "kind") != kind || originalMaterialText(proof, "operation_id") != r.OperationID || originalMaterialText(proof, "source_sha") != r.OriginalSourceSHA || originalMaterialText(proof, "target_hash") != digest(targets) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		// These are the existing closed, body-free verification proofs. Inputs,
		// credentials, source copies and bootstrap records remain purge required.
		// Proof bytes are bounded and hash-bound, but this catalog does not
		// reclassify opaque nested summary fields as body-free. Purge them.
		parent.files[ref.Filename].retained = false
		if kind == "history" {
			if e = completeLifecycleOriginalReadOnlyHistory(ctx, r, parent, inventory, proof, uid); e != nil {
				return e
			}
		}
	}
	return parent.checkComplete(false)
}

func registerLifecycleOriginalOperationLock(d *lifecycleMaterialDirectory, uid uint32) error {
	if e := registerLifecycleOriginalFrozenFile(d, "operation.lock", digestRaw(nil), uid); e != nil {
		return e
	}
	f := d.files["operation.lock"]
	if f == nil || f.info.Size() != 0 || syscall.Flock(int(f.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil || d.checkFile(f) != nil {
		return lifecycleError("lifecycle_original_operation_lock_rejected")
	}
	// The same registered FD remains locked through purge and is released only
	// when the original catalog owner closes it. No business fence is implied.
	return nil
}
func registerLifecycleOriginalFrozenFile(d *lifecycleMaterialDirectory, name, hash string, uid uint32) error {
	if d == nil || filepath.Base(name) != name || !hashRE.MatchString(hash) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	if f := d.files[name]; f != nil {
		if f.hash != hash || d.checkFile(f) != nil {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		return nil
	}
	return d.register(name, hash, uid, 0600)
}

// This is a bounded projection of fixed producer schemas, not an ownership
// manifest: callers supply exact names and follow immutable reference hashes.
func readLifecycleOriginalProducerJSON(ctx context.Context, d *lifecycleMaterialDirectory, name, hash string, uid uint32, closed string) (map[string]json.RawMessage, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var raw []byte
	var e error
	var value json.RawMessage
	if f := d.files[name]; f != nil {
		if hash != "" && hash != f.hash {
			return nil, lifecycleError("lifecycle_original_operation_material_rejected")
		}
		raw, e = readLifecycleHistoricalHeldJSON(d, name, 256<<10, &value)
	} else {
		raw, e = readLifecycleProducerJSON(d, name, hash, uid, 256<<10, &value)
	}
	if e != nil {
		return nil, e
	}
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != len(strings.Fields(closed)) {
		return nil, lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for _, key := range strings.Fields(closed) {
		if _, ok := fields[key]; !ok {
			return nil, lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	return fields, nil
}
func originalMaterialText(fields map[string]json.RawMessage, key string) string {
	var v string
	_ = json.Unmarshal(fields[key], &v)
	return v
}
func originalMaterialBool(fields map[string]json.RawMessage, key string, want bool) bool {
	var v *bool
	return json.Unmarshal(fields[key], &v) == nil && v != nil && *v == want
}
func originalMaterialVersion(fields map[string]json.RawMessage) bool {
	var n int
	return json.Unmarshal(fields["format_version"], &n) == nil && n == 1
}

type lifecycleOriginalReportReference struct {
	RunID     string `json:"run_id"`
	SourceSHA string `json:"source_sha,omitempty"`
	SHA256    string `json:"sha256"`
}

func originalMaterialReference(raw json.RawMessage, source string, hasSource bool) (lifecycleOriginalReportReference, error) {
	var v lifecycleOriginalReportReference
	var keys map[string]json.RawMessage
	n := 2
	if hasSource {
		n = 3
	}
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &keys) != nil || len(keys) != n || keys["run_id"] == nil || keys["sha256"] == nil || hasSource && keys["source_sha"] == nil || json.Unmarshal(raw, &v) != nil || !runRE.MatchString(v.RunID) || !hashRE.MatchString(v.SHA256) || hasSource && v.SourceSHA != source {
		return v, lifecycleError("lifecycle_original_operation_material_rejected")
	}
	return v, nil
}
func openLifecycleOriginalChild(parent *lifecycleMaterialDirectory, name string, uid uint32) (*lifecycleMaterialDirectory, error) {
	if filepath.Base(name) != name || parent == nil {
		return nil, lifecycleError("lifecycle_original_operation_material_rejected")
	}
	if d := parent.children[name]; d != nil {
		return d, nil
	}
	d, e := openLifecycleMaterialDirectory(filepath.Join(parent.path, name), uid)
	if e != nil {
		return nil, e
	}
	if e = parent.registerChild(d); e != nil {
		_ = d.close()
		return nil, e
	}
	return d, nil
}
func originalMaterialBootstrapBinding(m map[string]json.RawMessage, r lifecycleRequest, mode, requestHash string) bool {
	return originalMaterialVersion(m) && originalMaterialText(m, "kind") == "private_request_bootstrap_binding" && originalMaterialText(m, "source_sha") == r.OriginalSourceSHA && originalMaterialText(m, "operation_id") == r.OperationID && originalMaterialText(m, "prepare_mode") == mode && originalMaterialText(m, "request_sha256") == requestHash && runRE.MatchString(originalMaterialText(m, "created_run_id")) && hashRE.MatchString(originalMaterialText(m, "approval_sha256"))
}
func completeLifecycleOriginalInventoryInputs(ctx context.Context, r lifecycleRequest, parent *lifecycleMaterialDirectory, uid uint32) error {
	const fields = "format_version kind source_sha operation_id created_run_id prepare_mode approval_sha256 request_sha256 identity_report boundary_report"
	m, e := readLifecycleOriginalProducerJSON(ctx, parent, "inventory-request-bootstrap.json", "", uid, fields)
	if e != nil || !originalMaterialBootstrapBinding(m, r, "bootstrap-inventory", r.Approval.RequestHash) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	identityRef, e := originalMaterialReference(m["identity_report"], r.OriginalSourceSHA, true)
	if e != nil {
		return e
	}
	boundaryRef, e := originalMaterialReference(m["boundary_report"], r.OriginalSourceSHA, true)
	if e != nil || boundaryRef.RunID == identityRef.RunID || originalMaterialText(m, "created_run_id") == identityRef.RunID || originalMaterialText(m, "created_run_id") == boundaryRef.RunID {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var q request
	raw, e := readLifecycleHistoricalHeldJSON(parent, "inventory-request.json", 256<<10, &q)
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(q)) != nil || q.FormatVersion != 2 || q.Kind != "readonly_inventory_request" || q.OperationID != r.OperationID || q.SourceSHA != r.OriginalSourceSHA || q.BoundaryRunID != boundaryRef.RunID || q.BoundaryReportHash != boundaryRef.SHA256 {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	identity, e := openLifecycleOriginalChild(parent, "identity-"+identityRef.RunID, uid)
	if e != nil {
		return e
	}
	var observed identityReport
	raw, e = readLifecycleProducerJSON(identity, "identity.private.json", identityRef.SHA256, uid, 256<<10, &observed)
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(observed)) != nil || observed.FormatVersion != 1 || observed.Kind != "readonly_identity_discovery" || observed.SourceSHA != r.OriginalSourceSHA || observed.OperationID != r.OperationID || observed.RunID != identityRef.RunID || !hashRE.MatchString(observed.RequestHash) || observed.TargetHash != digest(targets) || !observed.DiagnosticOnly || observed.DropReady || !observed.Complete || observed.ErrorCategory != "none" {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var identityRequestValue identityRequest
	_, e = readLifecycleProducerJSON(parent, "identity-request.json", observed.RequestHash, uid, 256<<10, &identityRequestValue)
	if e != nil || identityRequestValue.SourceSHA != r.OriginalSourceSHA || identityRequestValue.OperationID != r.OperationID || identityRequestValue.FormatVersion != 1 || identityRequestValue.Kind != "readonly_identity_discovery_request" {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	identity.files["identity.private.json"].retained = true
	if e = identity.checkComplete(false); e != nil {
		return e
	}
	bounds, e := openLifecycleOriginalChild(parent, "bounds-"+boundaryRef.RunID, uid)
	if e != nil {
		return e
	}
	var b report
	raw, e = readLifecycleProducerJSON(bounds, "boundary.private.json", boundaryRef.SHA256, uid, 256<<10, &b)
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(b)) != nil || b.FormatVersion != 2 || b.Kind != "readonly_inventory_boundaries" || b.SourceSHA != r.OriginalSourceSHA || b.OperationID != r.OperationID || b.RunID != boundaryRef.RunID || !hashRE.MatchString(b.RequestHash) || b.TargetHash != digest(targets) || !b.DiagnosticOnly || b.DropReady || !b.Complete || b.ErrorCategory != "none" || b.SourceBytesProtocol != "no_source_body_copy" {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var boundaryRequest request
	_, e = readLifecycleProducerJSON(parent, "boundary-request.json", b.RequestHash, uid, 256<<10, &boundaryRequest)
	if e != nil || boundaryRequest.FormatVersion != 2 || boundaryRequest.Kind != "readonly_inventory_boundary_request" || boundaryRequest.SourceSHA != r.OriginalSourceSHA || boundaryRequest.OperationID != r.OperationID || boundaryRequest.BoundaryRunID != "" || len(boundaryRequest.Boundaries) != 0 || digest(boundaryRequest.Identities) != digest(q.Identities) || digest(boundaryRequest.Migrations) != digest(q.Migrations) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	bootstrap, e := readLifecycleOriginalProducerJSON(ctx, parent, "boundary-request-bootstrap.json", "", uid, fields)
	if e != nil || !originalMaterialBootstrapBinding(bootstrap, r, "bootstrap-bounds", b.RequestHash) || strings.TrimSpace(string(bootstrap["boundary_report"])) != "null" {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	originalIdentity, e := originalMaterialReference(bootstrap["identity_report"], r.OriginalSourceSHA, true)
	if e != nil || originalIdentity != identityRef || originalMaterialText(bootstrap, "created_run_id") == identityRef.RunID {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	bounds.files["boundary.private.json"].retained = true
	return bounds.checkComplete(false)
}
func completeLifecycleOriginalMetadataInputs(ctx context.Context, r lifecycleRequest, parent *lifecycleMaterialDirectory, uid uint32) error {
	m, e := readLifecycleOriginalProducerJSON(ctx, parent, "history-request-bootstrap.json", "", uid, "format_version kind source_sha operation_id created_run_id approval_sha256 approval request_sha256 parent_run_id metadata_report inventory_report")
	if e != nil || !originalMaterialVersion(m) || originalMaterialText(m, "kind") != "readonly_history_parent_registration" || originalMaterialText(m, "source_sha") != r.OriginalSourceSHA || originalMaterialText(m, "operation_id") != r.OperationID || !runRE.MatchString(originalMaterialText(m, "created_run_id")) || originalMaterialText(m, "request_sha256") != parent.files["history-request.json"].hash || originalMaterialText(m, "parent_run_id") != r.Approval.RunID || digestRaw(append(append([]byte(nil), m["approval"]...), '\n')) != originalMaterialText(m, "approval_sha256") {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	inventoryRef, e := originalMaterialReference(m["inventory_report"], r.OriginalSourceSHA, false)
	if e != nil || inventoryRef.RunID != r.Approval.RunID || inventoryRef.SHA256 != r.Approval.InventorySHA256 {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	metadataRef, e := originalMaterialReference(m["metadata_report"], r.OriginalSourceSHA, false)
	if e != nil {
		return e
	}
	d, e := openLifecycleOriginalChild(parent, "history-metadata-"+metadataRef.RunID, uid)
	if e != nil {
		return e
	}
	metadata, e := readLifecycleOriginalProducerJSON(ctx, d, "history-metadata.json", metadataRef.SHA256, uid, "format_version kind source_sha operation_id run_id metadata_limits approved_inventory_report inventory_request_sha256 approval_sha256 parent_proposal_run_id parent_proposal_sha256 assets equal_full_physical_passes metadata_complete input_baseline_sha256 semantic_source_coverage_verified production_process_budget_proven complete execution_allowed drop_ready cas_complete")
	if e != nil || !originalMaterialVersion(metadata) || originalMaterialText(metadata, "kind") != "readonly_history_file_metadata" || originalMaterialText(metadata, "source_sha") != r.OriginalSourceSHA || originalMaterialText(metadata, "operation_id") != r.OperationID || originalMaterialText(metadata, "run_id") != metadataRef.RunID || originalMaterialText(metadata, "inventory_request_sha256") != r.Approval.RequestHash || originalMaterialText(metadata, "parent_proposal_sha256") != parent.files["history-request.json"].hash || originalMaterialText(metadata, "parent_proposal_run_id") != r.Approval.RunID || !originalMaterialBool(metadata, "metadata_complete", true) || !originalMaterialBool(metadata, "drop_ready", false) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	metaInventory, e := originalMaterialReference(metadata["approved_inventory_report"], r.OriginalSourceSHA, false)
	if e != nil || metaInventory != inventoryRef {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	if e = registerLifecycleOriginalFrozenFile(d, "history-parent-proposal.json", parent.files["history-request.json"].hash, uid); e != nil {
		return e
	}
	bootstrap, e := readLifecycleOriginalProducerJSON(ctx, d, "history-metadata-bootstrap.json", "", uid, "format_version kind source_sha operation_id created_run_id approval_sha256 approval parent_proposal_run_id parent_proposal_sha256")
	if e != nil || !originalMaterialVersion(bootstrap) || originalMaterialText(bootstrap, "kind") != "readonly_history_metadata_binding" || originalMaterialText(bootstrap, "source_sha") != r.OriginalSourceSHA || originalMaterialText(bootstrap, "operation_id") != r.OperationID || originalMaterialText(bootstrap, "created_run_id") != metadataRef.RunID || originalMaterialText(bootstrap, "approval_sha256") != originalMaterialText(metadata, "approval_sha256") || originalMaterialText(bootstrap, "parent_proposal_run_id") != r.Approval.RunID || originalMaterialText(bootstrap, "parent_proposal_sha256") != parent.files["history-request.json"].hash || digestRaw(append(append([]byte(nil), bootstrap["approval"]...), '\n')) != originalMaterialText(bootstrap, "approval_sha256") {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	d.files["history-metadata.json"].retained = true
	return d.checkComplete(false)
}

// The fixed original AI journals contain hashes/IDs only. Registration binds
// their original immutable source/runtime tuple and prefix chain; fresh native
// GET quiescence and execution outcome checks remain with their existing owner.
type lifecycleOriginalAIExecBinding struct {
	SourceSHA        string `json:"source_sha"`
	OperationID      string `json:"operation_id"`
	RunID            string `json:"run_id"`
	RuntimeSourceSHA string `json:"runtime_source_sha"`
	ImageID          string `json:"image_id"`
	ContainerID      string `json:"container_id"`
	PythonSHA256     string `json:"python_sha256"`
	InputSHA256      string `json:"input_sha256"`
	DeadlineUnixNano int64  `json:"deadline_unix_nano"`
}
type lifecycleOriginalAIExecRecord struct {
	Protocol            string                         `json:"protocol"`
	Sequence            uint64                         `json:"sequence"`
	PreviousSHA256      string                         `json:"previous_sha256"`
	Binding             lifecycleOriginalAIExecBinding `json:"binding"`
	Stage               string                         `json:"stage"`
	ExecID              string                         `json:"exec_id"`
	EngineVersionSHA256 string                         `json:"engine_version_sha256"`
	Running             *bool                          `json:"running"`
	ExitCode            *int                           `json:"exit_code"`
	AttachComplete      bool                           `json:"attach_complete"`
	OutputBytes         uint64                         `json:"output_bytes"`
	OutputSHA256        string                         `json:"output_sha256"`
}

func registerLifecycleOriginalAIExecMaterials(ctx context.Context, r lifecycleRequest, parent *lifecycleMaterialDirectory, uid uint32) error {
	if r.FinalHistory == nil || !r.FinalHistory.valid(r) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for _, name := range []string{"qs-ai-external-bounds.exec.jsonl", "qs-ai-external-verify.exec.jsonl", "qs-ai-external-final-verify.exec.jsonl"} {
		if ctx == nil || ctx.Err() != nil {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		visible, e := os.Lstat(filepath.Join(parent.path, name))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		fd, e := syscall.Open(filepath.Join(parent.path, name), syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if e != nil {
			return e
		}
		f := os.NewFile(uintptr(fd), name)
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 1 || info.Size() > 64<<10 || info.Sys().(*syscall.Stat_t).Uid != uid || info.Sys().(*syscall.Stat_t).Nlink != 1 || !sameLifecycleFile(visible, info) {
			_ = f.Close()
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		raw, e := io.ReadAll(io.LimitReader(f, (64<<10)+1))
		after, se := f.Stat()
		closeErr := f.Close()
		if e != nil || se != nil || closeErr != nil || !sameLifecycleFile(info, after) || int64(len(raw)) != info.Size() {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		if e = validateLifecycleOriginalAIExecMaterial(raw, r); e != nil {
			return e
		}
		if e = parent.registerWithMaximum(name, digestRaw(raw), uid, 0600, 64<<10); e != nil {
			return e
		}
		held := parent.files[name]
		if !sameLifecycleFile(info, held.info) || parent.checkFile(held) != nil {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		held.retained = true
	}
	return nil
}
func validateLifecycleOriginalAIExecMaterial(raw []byte, r lifecycleRequest) error {
	if len(raw) == 0 || len(raw) > 64<<10 || raw[len(raw)-1] != '\n' || r.FinalHistory == nil {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	lines := strings.Split(string(raw[:len(raw)-1]), "\n")
	var original lifecycleOriginalAIExecBinding
	offset := 0
	for n, line := range lines {
		var v lifecycleOriginalAIExecRecord
		var fields map[string]json.RawMessage
		if rejectDuplicateJSON([]byte(line)) != nil || json.Unmarshal([]byte(line), &fields) != nil || len(fields) != 12 || lifecycleExactJSONNames([]byte(line), reflect.TypeOf(v)) != nil || json.Unmarshal([]byte(line), &v) != nil || v.Protocol != "qs-ai-exec-lifecycle/v1" || v.Sequence != uint64(n+1) || v.PreviousSHA256 != func() string {
			if n == 0 {
				return ""
			}
			return digestRaw(raw[:offset])
		}() {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		b := v.Binding
		if n == 0 {
			original = b
		}
		if b != original || b.SourceSHA != r.OriginalSourceSHA || b.OperationID != r.OperationID || !validLifecycleOriginalAIExecRunID(b.RunID) || b.RuntimeSourceSHA != r.FinalHistory.RuntimeSourceSHA || b.ImageID != r.FinalHistory.ImageID || b.ContainerID != r.FinalHistory.ContainerID || !hashRE.MatchString(b.PythonSHA256) || !hashRE.MatchString(b.InputSHA256) || b.DeadlineUnixNano <= 0 || !hashRE.MatchString(v.EngineVersionSHA256) || v.ExecID != "" && !hashRE.MatchString(v.ExecID) || v.OutputSHA256 != "" && !hashRE.MatchString(v.OutputSHA256) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		switch v.Stage {
		case "create_intent", "created", "start_intent", "attached", "unknown", "observed":
		default:
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		offset += len(line) + 1
	}
	return nil
}
func validLifecycleOriginalAIExecRunID(run string) bool {
	if len(run) < 1 || len(run) > 20 {
		return false
	}
	for _, c := range run {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// The history proof's immutable producer run is the only directory selector.
// Container receipts bind the existing producer's original files; their flags
// never replace current native liveness, CAS, fence or business acceptance.
func completeLifecycleOriginalReadOnlyHistory(ctx context.Context, r lifecycleRequest, parent, inventory *lifecycleMaterialDirectory, proof map[string]json.RawMessage, uid uint32) error {
	producer, e := originalMaterialClosedFields(proof["producer"], "protocol source_sha run_id")
	if e != nil {
		return e
	}
	run := originalMaterialText(producer, "run_id")
	if originalMaterialText(producer, "protocol") != "qs_compatibility_retirement_history_v1" || originalMaterialText(producer, "source_sha") != r.OriginalSourceSHA || !runRE.MatchString(run) || run == r.Approval.RunID || run == r.ActualRunID {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	summary, e := originalMaterialClosedFields(proof["summary"], "classified verified_live verified_retired unverifiable_closed unresolved ambiguous hash_conflicts unknown_execution unexplained_high retirement_references references_hash")
	if e != nil {
		return e
	}
	for name, raw := range summary {
		if name == "references_hash" {
			if !hashRE.MatchString(originalMaterialText(summary, name)) {
				return lifecycleError("lifecycle_original_operation_material_rejected")
			}
		} else {
			var n uint64
			if json.Unmarshal(raw, &n) != nil {
				return lifecycleError("lifecycle_original_operation_material_rejected")
			}
		}
	}
	d, e := openLifecycleOriginalChild(parent, "history-"+run, uid)
	if e != nil {
		return e
	}
	terminal, e := readLifecycleOriginalProducerJSON(ctx, d, "history.terminal.json", "", uid, "id name image actual_run_id source_sha request_sha256 creation_nonce owner_uid owner_gid status exit_code container_removed private_readiness_sha256")
	if e != nil {
		return e
	}
	cid, image, nonce, requestHash, readinessHash := originalMaterialText(terminal, "id"), originalMaterialText(terminal, "image"), originalMaterialText(terminal, "creation_nonce"), originalMaterialText(terminal, "request_sha256"), originalMaterialText(terminal, "private_readiness_sha256")
	var owner uint32
	var code int
	if !hashRE.MatchString(cid) || !strings.HasPrefix(image, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(image, "sha256:")) || !hashRE.MatchString(nonce) || !hashRE.MatchString(requestHash) || !hashRE.MatchString(readinessHash) || originalMaterialText(terminal, "name") != "qs-compatibility-history-"+run || originalMaterialText(terminal, "actual_run_id") != run || originalMaterialText(terminal, "source_sha") != r.OriginalSourceSHA || originalMaterialText(terminal, "status") != "exited" || !originalMaterialBool(terminal, "container_removed", true) || json.Unmarshal(terminal["owner_uid"], &owner) != nil || owner != uid || json.Unmarshal(terminal["exit_code"], &code) != nil || code != 0 {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var q lifecycleHistoricalRequestMaterial
	raw, e := readLifecycleProducerJSON(d, "history.request.json", requestHash, uid, 256<<10, &q)
	if e != nil || decodeLifecycleHistoricalRequest(raw, &q) != nil || !lifecycleHistoricalRequestMatches(q, r, inventory, parent.path, run) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	bootstrap, e := readLifecycleOriginalProducerJSON(ctx, d, "history.bootstrap.json", "", uid, "format_version kind source_sha operation_id actual_run_id approval_sha256 approval parent_request_sha256 parent_run_id derived_request_sha256 only_changed_field binary_sha256")
	if e != nil || !originalMaterialVersion(bootstrap) || originalMaterialText(bootstrap, "kind") != "immutable_history_run_derivation" || originalMaterialText(bootstrap, "source_sha") != r.OriginalSourceSHA || originalMaterialText(bootstrap, "operation_id") != r.OperationID || originalMaterialText(bootstrap, "actual_run_id") != run || originalMaterialText(bootstrap, "parent_request_sha256") != parent.files["history-request.json"].hash || originalMaterialText(bootstrap, "parent_run_id") != r.Approval.RunID || originalMaterialText(bootstrap, "derived_request_sha256") != requestHash || originalMaterialText(bootstrap, "only_changed_field") != "run_id" || !hashRE.MatchString(originalMaterialText(bootstrap, "binary_sha256")) || digestRaw(append(append([]byte(nil), bootstrap["approval"]...), '\n')) != originalMaterialText(bootstrap, "approval_sha256") {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var original lifecycleHistoricalRequestMaterial
	originalRaw, e := readLifecycleHistoricalHeldJSON(parent, "history-request.json", 256<<10, &original)
	if e != nil || decodeLifecycleHistoricalRequest(originalRaw, &original) != nil {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	original.RunID = run
	if !reflect.DeepEqual(q, original) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	created, e := readLifecycleOriginalProducerJSON(ctx, d, "history.container.json", "", uid, "id name image labels mounts limits owner_uid owner_gid")
	if e != nil {
		return e
	}
	intent, e := readLifecycleOriginalProducerJSON(ctx, d, "history.creation.intent.json", "", uid, "name image labels declared_image_volumes mounts limits owner_uid owner_gid")
	if e != nil {
		return e
	}
	if originalMaterialText(created, "id") != cid || originalMaterialText(created, "name") != originalMaterialText(terminal, "name") || originalMaterialText(created, "image") != image || originalMaterialText(intent, "name") != originalMaterialText(terminal, "name") || originalMaterialText(intent, "image") != image {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for _, name := range []string{"labels", "mounts", "limits", "owner_uid", "owner_gid"} {
		if !reflect.DeepEqual(created[name], intent[name]) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	if !reflect.DeepEqual(created["owner_uid"], terminal["owner_uid"]) || !reflect.DeepEqual(created["owner_gid"], terminal["owner_gid"]) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var labels map[string]string
	if json.Unmarshal(created["labels"], &labels) != nil || labels["qs.compatibility-retirement.operation"] != r.OperationID || labels["qs.compatibility-retirement.run"] != run || labels["qs.compatibility-retirement.source"] != r.OriginalSourceSHA || labels["qs.compatibility-retirement.request"] != requestHash || labels["qs.compatibility-retirement.creation"] != nonce || labels["qs.compatibility-retirement.kind"] != "history-readonly" {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	shadow, e := openLifecycleOriginalChild(d, "mysql-volume-shadow", uid)
	if e != nil {
		return e
	}
	if e = shadow.checkComplete(false); e != nil {
		return e
	}
	output, e := openLifecycleOriginalChild(d, "output", uid)
	if e != nil {
		return e
	}
	readiness, e := readLifecycleOriginalProducerJSON(ctx, output, "history.readiness.json", readinessHash, uid, "protocol source_sha operation_id run_id request_sha256 inventory_request_sha256 inventory_report_sha256 completed_readonly_pipeline independent_epochs source_files_and_actual_origins_matched whole_four_source_coverage_complete business_and_responsibility_facts_unchanged sources full_source_file_sha256 whole_source_index_sha256 candidate_sha256 sql_current_facts_sha256 mongo_current_facts_sha256 local_candidates locally_qualified blocked_local joint_event_pages ai_blocked_pages sql_ledger_count mongo_collection_count sql_global mongo_global ai_reverse_global blocking_reasons required_adapters independent_production_approval_verified ordered_mongo_source_metadata_approved host_process_budget_proven full_external_ai_closure_verified distributed_atomic_snapshot writer_fence_proven cas_complete post_cas_readback_complete backup_restore_qualified mutation_backend_enabled drop_ready error_category elapsed_milliseconds")
	if e != nil || originalMaterialText(readiness, "protocol") != "qs-compatibility-history-readonly/v1" || originalMaterialText(readiness, "source_sha") != r.OriginalSourceSHA || originalMaterialText(readiness, "operation_id") != r.OperationID || originalMaterialText(readiness, "run_id") != run || originalMaterialText(readiness, "request_sha256") != requestHash || originalMaterialText(readiness, "inventory_request_sha256") != r.Approval.RequestHash || originalMaterialText(readiness, "inventory_report_sha256") != r.Approval.InventorySHA256 || originalMaterialText(readiness, "error_category") != "none" {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for _, name := range []string{"completed_readonly_pipeline", "source_files_and_actual_origins_matched", "whole_four_source_coverage_complete", "business_and_responsibility_facts_unchanged"} {
		if !originalMaterialBool(readiness, name, true) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	for _, name := range []string{"independent_production_approval_verified", "ordered_mongo_source_metadata_approved", "host_process_budget_proven", "full_external_ai_closure_verified", "distributed_atomic_snapshot", "writer_fence_proven", "cas_complete", "post_cas_readback_complete", "backup_restore_qualified", "mutation_backend_enabled", "drop_ready"} {
		if !originalMaterialBool(readiness, name, false) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	var epochs int
	var hashes []string
	var sources []retirement.SourceCopyReceipt
	if json.Unmarshal(readiness["independent_epochs"], &epochs) != nil || epochs != 2 || json.Unmarshal(readiness["full_source_file_sha256"], &hashes) != nil || len(hashes) != 4 || json.Unmarshal(readiness["sources"], &sources) != nil || len(sources) != 4 {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var inventoryReport report
	if _, e = readLifecycleHistoricalHeldJSON(inventory, "inventory.private.json", 256<<10, &inventoryReport); e != nil || len(inventoryReport.Targets) != 4 {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for n, source := range sources {
		protocol := retirement.SQLSourceProtocol
		if targets[n][0] == "mongodb" {
			protocol = retirement.MongoSourceProtocol
		}
		if hashes[n] != inventory.files[lifecycleSourceNames[3+n]].hash || source.Protocol != protocol || !source.Complete || source.BusinessClosureVerified || source.DropReady || source.Records != inventoryReport.Targets[n].Records || source.Bytes != inventoryReport.Targets[n].Bytes || source.DataHash != inventoryReport.Targets[n].DataHash {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	// Only the existing native body's-free diagnostic and terminal hash receipt
	// survive. Original requests, mount/creation inputs and bootstrap bytes purge.
	output.files["history.readiness.json"].retained = true
	d.files["history.terminal.json"].retained = true
	if e = output.checkComplete(false); e != nil {
		return e
	}
	return d.checkComplete(false)
}
func originalMaterialClosedFields(raw json.RawMessage, closed string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != len(strings.Fields(closed)) {
		return nil, lifecycleError("lifecycle_original_operation_material_rejected")
	}
	for _, name := range strings.Fields(closed) {
		if fields[name] == nil || strings.TrimSpace(string(fields[name])) == "null" {
			return nil, lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	return fields, nil
}

// This exact root-owned observation is outside the source-owned parent tree.
// The caller must keep the returned leaf in its catalog, never attach it under
// the source parent or delete another root preparation batch by enumeration.
func openLifecycleOriginalCensusMaterials(ctx context.Context, r lifecycleRequest, uid uint32) (*lifecycleMaterialDirectory, error) {
	if ctx == nil || ctx.Err() != nil || r.WriterControl == nil || r.WriterControl.DatabaseInput == nil {
		return nil, lifecycleError("lifecycle_original_operation_material_rejected")
	}
	ref, run, e := lifecycleOriginalCensusReference(r, uid)
	if e != nil {
		return nil, e
	}
	d, e := openLifecycleMaterialDirectory(filepath.Dir(ref.Path), 0)
	if e != nil {
		return nil, e
	}
	reject := func(e error) (*lifecycleMaterialDirectory, error) { _ = d.close(); return nil, e }
	if e = d.registerWithMaximum("db-writer-census.private.json", ref.SHA256, 0, 0600, 64<<20); e != nil {
		return reject(e)
	}
	var c dbCensusPrivate
	raw, e := readLifecycleHistoricalHeldJSON(d, "db-writer-census.private.json", 64<<20, &c)
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(c)) != nil || !lifecycleDBCensusProducerValid(c, r, run) {
		return reject(lifecycleError("lifecycle_original_operation_material_rejected"))
	}
	var tool lifecycleOriginalRootToolIntent
	raw, e = readLifecycleProducerJSON(d, "tool.intent.private.json", "", 0, 256<<10, &tool)
	if e != nil || decodeLifecycleClosedProducer(raw, &tool) != nil || tool.FormatVersion != 1 || tool.Kind != "approved_root_once_tool_staging" || tool.Stage != "db-writer-census" || tool.OperationID != r.OperationID || tool.ActualRunID != run || tool.ToolSourceSHA != r.OriginalSourceSHA || tool.RequestPath != filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID, "db-writer-census-request-"+run+".json") || tool.RequestSHA256 != c.RequestSHA256 || tool.ManifestSHA256 != "" || !hashRE.MatchString(tool.PackageSHA256) || !hashRE.MatchString(tool.NativeSHA256) || tool.SourceUID != uid || tool.DropAuthority || !tool.PurgeRequired {
		return reject(lifecycleError("lifecycle_original_operation_material_rejected"))
	}
	if e = d.register("restore-native", tool.NativeSHA256, 0, 0700); e != nil {
		return reject(e)
	}
	var request dbCensusRequest
	raw, e = readLifecycleOwnedBytes(tool.RequestPath, c.RequestSHA256, uid, 256<<10)
	if e == nil {
		e = json.Unmarshal(raw, &request)
	}
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(request)) != nil || request.FormatVersion != 1 || request.Kind != "readonly_db_writer_census_request" || request.SourceSHA != r.OriginalSourceSHA || request.OperationID != r.OperationID || request.ActualRunID != run || request.TargetHash != digest(targets) || request.Identity != c.IdentityProducer {
		return reject(lifecycleError("lifecycle_original_operation_material_rejected"))
	}
	if e = d.checkComplete(false); e != nil {
		return reject(e)
	}
	return d, nil
}

func lifecycleOriginalCensusReference(r lifecycleRequest, uid uint32) (lifecycleFinalFileBinding, string, error) {
	if r.WriterControl == nil || r.WriterControl.DatabaseInput == nil {
		return lifecycleFinalFileBinding{}, "", lifecycleError("lifecycle_original_operation_material_rejected")
	}
	ref := *r.WriterControl.DatabaseInput
	if run, native := lifecycleDBInputCensusRun(ref.Path, r.OperationID); native {
		return ref, run, nil
	}
	if filepath.Dir(ref.Path) != filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID) {
		return ref, "", lifecycleError("lifecycle_original_operation_material_rejected")
	}
	raw, e := readLifecycleOwnedBytes(ref.Path, ref.SHA256, uid, 256<<10)
	var input lifecycleDBWriterInput
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(input)) != nil || json.Unmarshal(raw, &input) != nil || !lifecycleDBInputValid(input, r) || input.CensusSourceSHA != r.OriginalSourceSHA {
		return ref, "", lifecycleError("lifecycle_original_operation_material_rejected")
	}
	run, native := lifecycleDBInputCensusRun(input.Census.Path, r.OperationID)
	if !native || run != input.CensusRunID {
		return ref, "", lifecycleError("lifecycle_original_operation_material_rejected")
	}
	return input.Census, run, nil
}
func registerLifecycleOriginalDatabaseSourceInput(ctx context.Context, r lifecycleRequest, parent *lifecycleMaterialDirectory, uid uint32) error {
	if r.WriterControl == nil || r.WriterControl.DatabaseInput == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	originalRef := *r.WriterControl.DatabaseInput
	if _, native := lifecycleDBInputCensusRun(originalRef.Path, r.OperationID); !native {
		if filepath.Dir(originalRef.Path) != parent.path || !hashRE.MatchString(originalRef.SHA256) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
		var input lifecycleDBWriterInput
		raw, e := readLifecycleProducerJSON(parent, filepath.Base(originalRef.Path), originalRef.SHA256, uid, 256<<10, &input)
		if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(input)) != nil || !lifecycleDBInputValid(input, r) {
			return lifecycleError("lifecycle_original_operation_material_rejected")
		}
	}
	ref, run, e := lifecycleOriginalCensusReference(r, uid)
	if e != nil {
		return e
	}
	raw, e := readLifecycleOwnedBytes(ref.Path, ref.SHA256, 0, 64<<20)
	var c dbCensusPrivate
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(c)) != nil || json.Unmarshal(raw, &c) != nil || !lifecycleDBCensusProducerValid(c, r, run) {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	var q dbCensusRequest
	raw, e = readLifecycleProducerJSON(parent, "db-writer-census-request-"+run+".json", c.RequestSHA256, uid, 256<<10, &q)
	if e != nil || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(q)) != nil || q.FormatVersion != 1 || q.Kind != "readonly_db_writer_census_request" || q.SourceSHA != r.OriginalSourceSHA || q.OperationID != r.OperationID || q.ActualRunID != run || q.TargetHash != digest(targets) || q.Identity != c.IdentityProducer {
		return lifecycleError("lifecycle_original_operation_material_rejected")
	}
	return nil
}
