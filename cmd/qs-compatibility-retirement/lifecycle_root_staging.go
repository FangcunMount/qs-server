package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
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
	for _, name := range []string{"resume", "resume_kind", "service_control", "deployment_control", "final_history", "writer_control"} {
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
