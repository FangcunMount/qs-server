package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
)

const lifecycleRootPrepareBase = "/opt/backups/qs-server/compatibility-retirement-root-prepare"
const lifecycleInvocationBase = "/opt/backups/qs-server/compatibility-retirement-invocations"

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
	if !runRE.MatchString(r.Approval.RunID) || !shaRE.MatchString(r.OriginalSourceSHA) || r.Approval.SourceSHA != r.OriginalSourceSHA || r.Approval.OperationID != operation || r.FormatVersion != 1 || r.Kind != "compatibility_retirement_lifecycle_request" || r.OperationID != operation || r.ActualRunID != actualRun || r.ToolSourceSHA != sourceSHA || r.Recovery.ArchiveSHA256 != "" || !r.RestoreEngines.valid() || r.SourceDirectory != filepath.Join(original, "inventory-"+r.Approval.RunID) || len(r.SourceFileSHA256) != 7 {
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
