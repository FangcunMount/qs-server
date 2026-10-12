package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	aibridge "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	jose "github.com/go-jose/go-jose/v4"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"text/template"
	"time"
)

type prepareFactsDirectoryOwnerInfo struct {
	os.FileInfo
	uid, gid uint32
}

func (i prepareFactsDirectoryOwnerInfo) Sys() any {
	st := *i.FileInfo.Sys().(*syscall.Stat_t)
	st.Uid, st.Gid = i.uid, i.gid
	return &st
}

func TestPrepareFactsSharedKeyAncestorOwnershipIsExact(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	foreign := prepareFactsDirectoryOwnerInfo{FileInfo: st, uid: 12345, gid: 12346}
	key := "/data/infra/qs-server-messaging/versions/mq-v1/qs.encrypt.v1.json"
	for _, test := range []struct {
		name, path, directory string
		allowed               bool
	}{
		{"shared_data", key, "/data", true},
		{"shared_infra", key, "/data/infra", true},
		{"messaging_root", key, "/data/infra/qs-server-messaging", false},
		{"versions", key, "/data/infra/qs-server-messaging/versions", false},
		{"version", key, "/data/infra/qs-server-messaging/versions/mq-v1", false},
		{"other_shared_parent", key, "/opt", false},
		{"other_key_tree", "/data/infra/other/versions/mq-v1/qs.encrypt.v1.json", "/data", false},
		{"other_role", "/data/infra/qs-server-messaging/versions/mq-v1/unrelated.json", "/data", false},
		{"bad_revision", "/data/infra/qs-server-messaging/versions/MQ-V1/qs.encrypt.v1.json", "/data", false},
		{"nested_leaf", "/data/infra/qs-server-messaging/versions/mq-v1/nested/qs.encrypt.v1.json", "/data", false},
		{"release_asset", "/opt/qs-server/qs-apiserver/ai-mq-releases/source/binding.json", "/data", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := prepareFactsKeyDirectoryAllowed(test.path, test.directory, 2000, foreign); got != test.allowed {
				t.Fatalf("directory owner decision=%v, want %v", got, test.allowed)
			}
		})
	}
	for _, mode := range []os.FileMode{0775, 0757, 0777 | os.ModeSticky} {
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		writable, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if prepareFactsKeyDirectoryAllowed(key, "/data", 2000, prepareFactsDirectoryOwnerInfo{FileInfo: writable, uid: 12345, gid: 12346}) {
			t.Fatal("writable shared ancestor accepted")
		}
	}
	link := filepath.Join(t.TempDir(), "ancestor")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	symlink, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if prepareFactsKeyDirectoryAllowed(key, "/data", 2000, symlink) || prepareFactsKeyDirectoryAllowed(key, "/data", 2000, nil) {
		t.Fatal("missing or symlink ancestor accepted")
	}
}

func TestPrepareFactsMQInspectIgnoresOnlyMountOrder(t *testing.T) {
	mount := func(source, destination string) map[string]any {
		return map[string]any{"Type": "bind", "Source": source, "Destination": destination, "RW": false, "Mode": "ro", "Propagation": "rprivate", "unknown": map[string]any{"retained": true}}
	}
	value := map[string]any{"id": strings.Repeat("1", 64), "image": "sha256:" + strings.Repeat("2", 64), "pid": 123, "user": "www", "running": true, "name": "/qs-apiserver", "service": "qs-apiserver", "entrypoint": []string{"/app/qs-apiserver"}, "command": []string{"--config=/app/configs/apiserver.prod.yaml"}, "source_sha": strings.Repeat("3", 40), "mounts": []any{mount("/original/config", "/app/configs/apiserver.prod.yaml"), mount("/original/key", "/run/qs-server-jose/qs.sign.v1.json")}}
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"mount_order", "source", "type", "rw", "propagation", "extra_mount_field", "destination", "duplicate_destination", "uid", "pid", "source_sha", "image", "running", "id", "command", "unknown_top_field", "missing_mounts", "null_mounts"} {
		t.Run(mutation, func(t *testing.T) {
			var after map[string]any
			if err := json.Unmarshal(before, &after); err != nil {
				t.Fatal(err)
			}
			mounts := after["mounts"].([]any)
			first := mounts[0].(map[string]any)
			switch mutation {
			case "mount_order":
				mounts[0], mounts[1] = mounts[1], mounts[0]
			case "source":
				first["Source"] = "/changed/key"
			case "type":
				first["Type"] = "volume"
			case "rw":
				first["RW"] = true
			case "propagation":
				first["Propagation"] = "rshared"
			case "extra_mount_field":
				first["unknown"].(map[string]any)["retained"] = false
			case "destination":
				first["Destination"] = "/changed/destination"
			case "duplicate_destination":
				first["Destination"] = mounts[1].(map[string]any)["Destination"]
			case "uid":
				after["user"] = "1002"
			case "pid":
				after["pid"] = 456
			case "source_sha":
				after["source_sha"] = strings.Repeat("4", 40)
			case "image":
				after["image"] = "sha256:" + strings.Repeat("5", 64)
			case "running":
				after["running"] = false
			case "id":
				after["id"] = strings.Repeat("6", 64)
			case "command":
				after["command"] = []string{"--different"}
			case "unknown_top_field":
				after["new_field"] = true
			case "missing_mounts":
				delete(after, "mounts")
			case "null_mounts":
				after["mounts"] = nil
			}
			raw, err := json.Marshal(after)
			if err != nil {
				t.Fatal(err)
			}
			if got := prepareFactsSameMQInspect(before, raw); got != (mutation == "mount_order") {
				t.Fatalf("inspect equality=%v", got)
			}
		})
	}
	for _, malformed := range [][]byte{[]byte(`{"mounts":[],"mounts":[]}`), []byte(`{"mounts":[{"Destination":"/a","Destination":"/b"}]}`), []byte(`{"mounts":[{}]}`), []byte(`{"mounts":null}`), []byte(`{"mounts":[]}{}`)} {
		if prepareFactsSameMQInspect(malformed, malformed) {
			t.Fatal("ambiguous inspect accepted")
		}
	}
}

func TestPrepareFactsKeyDirectoryIdentityRecheck(t *testing.T) {
	for _, mutation := range []string{"unchanged", "mode", "replaced", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			parent := t.TempDir()
			path := filepath.Join(parent, "ancestor")
			if err := os.Mkdir(path, 0755); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "mode":
				if err := os.Chmod(path, 0750); err != nil {
					t.Fatal(err)
				}
			case "replaced", "symlink":
				retained := filepath.Join(parent, "retained")
				if err := os.Rename(path, retained); err != nil {
					t.Fatal(err)
				}
				if mutation == "replaced" {
					if err := os.Mkdir(path, 0755); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Symlink(retained, path); err != nil {
					t.Fatal(err)
				}
			}
			if got := prepareFactsKeyDirectoriesUnchanged(map[string]os.FileInfo{path: before}); got != (mutation == "unchanged") {
				t.Fatalf("ancestor identity equal=%v", got)
			}
		})
	}
}

func TestPrepareFactsSharedKeyAncestorsLinuxRootFixture(t *testing.T) {
	const enabled = "QS_RETIREMENT_TEST_SHARED_KEY_ANCESTORS"
	if os.Getenv(enabled) == "" {
		t.Skip("explicit isolated Linux root fixture required")
	}
	if os.Getenv(enabled) != "1" || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Fatal("fixture requires explicit switch and Linux root")
	}
	mountInfo, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	isolated := map[string]bool{}
	for _, line := range strings.Split(string(mountInfo), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+1 >= len(fields) {
			continue
		}
		if fields[4] == "/" {
			isolated["root_readonly"] = strings.Contains(","+fields[5]+",", ",ro,")
		}
		if fields[4] == "/data" || fields[4] == "/tmp" {
			isolated[fields[4]] = fields[3] == "/" && fields[separator+1] == "tmpfs"
		}
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "CapEff:" {
			isolated["only_chown"] = fields[1] == "0000000000000001"
		}
	}
	contents, err := os.ReadDir("/data")
	if err != nil || len(contents) != 0 || !isolated["root_readonly"] || !isolated["/data"] || !isolated["/tmp"] || !isolated["only_chown"] {
		t.Fatal("fixture requires empty independent /data tmpfs, /tmp tmpfs, readonly rootfs and only CAP_CHOWN")
	}
	const readerUID = uint32(2000)
	const foreignUID, foreignGID = 12345, 12346
	version := "/data/infra/qs-server-messaging/versions/mq-v1"
	key := version + "/qs.encrypt.v1.json"
	if err := os.MkdirAll(version, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chown("/data", 0, 0); err != nil {
			t.Error(err)
		}
		for _, path := range []string{"/data/infra", "/data/retained-infra"} {
			if err := os.Chown(path, 0, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Error(err)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Error(err)
			}
		}
	})
	if err := os.WriteFile(key, []byte(`{"fixture":"synthetic-only"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/data", "/data/infra"} {
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, foreignUID, foreignGID); err != nil {
			t.Fatal(err)
		}
	}
	f, raw, err := prepareFactsKeyFile(key, readerUID, true, 16384)
	if err != nil || f == nil || string(raw) != `{"fixture":"synthetic-only"}` {
		t.Fatal("real cross-UID shared ancestors rejected", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	rejected := func() {
		t.Helper()
		f, raw, err := prepareFactsKeyFile(key, readerUID, true, 16384)
		if err == nil || f != nil || raw != nil {
			t.Fatal("unsafe real key ancestry accepted")
		}
	}
	if err := os.Chown(version, foreignUID, foreignGID); err != nil {
		t.Fatal(err)
	}
	rejected()
	if err := os.Chown(version, 0, 0); err != nil {
		t.Fatal(err)
	}
	// Regain ownership only within this isolated test tmpfs to alter the fixture.
	if err := os.Chown("/data", 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("/data", 0757); err != nil {
		t.Fatal(err)
	}
	rejected()
	if err := os.Chmod("/data", 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat("/data/infra")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename("/data/infra", "/data/retained-infra"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/data/retained-infra", "/data/infra"); err != nil {
		t.Fatal(err)
	}
	rejected()
	if prepareFactsKeyDirectoriesUnchanged(map[string]os.FileInfo{"/data/infra": before}) {
		t.Fatal("real symlink ancestor replacement accepted")
	}
	if err := os.Remove("/data/infra"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("/data/infra", 0755); err != nil {
		t.Fatal(err)
	}
	if prepareFactsKeyDirectoriesUnchanged(map[string]os.FileInfo{"/data/infra": before}) {
		t.Fatal("real directory inode replacement accepted")
	}
	rejected()
}

func prepareFactsRequestFixture(t *testing.T) prepareFactsRequest {
	t.Helper()
	previous := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = previous })
	return prepareFactsRequest{FormatVersion: 1, Kind: "readonly_prepare_facts_request", SourceSHA: sourceSHA,
		OperationID: "123-1", ActualRunID: "789-1", TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb",
		ObservationApprovalSHA256: strings.Repeat("b", 64), Inventory: prepareFactsProducer{OperationID: "123-1", RunID: "456-1", SourceSHA: strings.Repeat("c", 40), ReportSHA256: strings.Repeat("d", 64), RequestSHA256: strings.Repeat("e", 64)},
		RestoreEngines: &lifecycleRestoreEngines{MySQLImageID: "sha256:" + strings.Repeat("1", 64), MongoImageID: "sha256:" + strings.Repeat("2", 64), Architecture: runtime.GOARCH}, ArchiveDirectory: "/opt/backups/qs-server/compatibility-retirement/123-1/temporary-archive"}
}

func prepareFactsOriginalBoundaryFixture(t *testing.T) (request, report, string) {
	t.Helper()
	r := v2Request("readonly_inventory_request")
	r.BoundaryRunID = "456-1"
	r.SourceSHA = strings.Repeat("c", 40)
	observed := report{FormatVersion: 2, Kind: "readonly_inventory_boundaries", SourceSHA: r.SourceSHA,
		OperationID: r.OperationID, RunID: r.BoundaryRunID, TargetHash: r.TargetHash, Complete: true,
		DiagnosticOnly: true, DatabaseBindings: map[string]databaseInventory{}}
	for _, db := range []string{"mysql", "mongodb"} {
		observed.DatabaseBindings[db] = databaseInventory{IdentityHash: r.Identities[db], Version: r.Migrations[db],
			MetadataComplete: true, ExpectedIdentityMatch: true, ExpectedMigrationMatch: true}
	}
	for _, target := range targets {
		b := targetBoundary{Database: target[0], Name: target[1], Kind: target[2], SchemaHash: strings.Repeat("6", 64), IdentityHash: strings.Repeat("7", 64)}
		r.Boundaries = append(r.Boundaries, b)
		observed.Targets = append(observed.Targets, snapshot{Complete: true, ErrorCategory: "none", Boundary: &b})
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return r, observed, filepath.Join(dir, "inventory-request.json")
}

func writePrepareFactsOriginalBoundary(t *testing.T, r *request, observed report, requestPath string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(requestPath), "bounds-"+r.BoundaryRunID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "boundary.private.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	r.BoundaryReportHash = digestRaw(raw)
	return path
}

func TestPrepareFactsOriginalBoundaryUsesExactSourceOwner(t *testing.T) {
	r, observed, requestPath := prepareFactsOriginalBoundaryFixture(t)
	path := writePrepareFactsOriginalBoundary(t, &r, observed, requestPath)
	uid := uint32(os.Getuid())
	if uid == 0 {
		uid = 1002
		for _, p := range []string{filepath.Dir(path), path} {
			if err := os.Chown(p, int(uid), -1); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := validatePrepareFactsOriginalRequest(r, requestPath, uid); err != nil {
		t.Fatal("bound source owner rejected", err)
	}
	if err := validatePrepareFactsOriginalRequest(r, requestPath, uid+1); err == nil {
		t.Fatal("source owner substituted with a different UID")
	}
	if err := validateV2Request(r, requestPath); (err == nil) != (uid == uint32(os.Getuid())) {
		t.Fatal("default reader no longer enforces caller ownership", err)
	}
}

func TestPrepareFactsOriginalBoundaryPreservesReportSemantics(t *testing.T) {
	for _, change := range []string{"incomplete", "authority", "source", "operation", "run", "identity", "dirty", "boundary"} {
		t.Run(change, func(t *testing.T) {
			r, observed, requestPath := prepareFactsOriginalBoundaryFixture(t)
			switch change {
			case "incomplete":
				observed.Complete = false
			case "authority":
				observed.DropReady = true
			case "source":
				observed.SourceSHA = strings.Repeat("d", 40)
			case "operation":
				observed.OperationID = "999-1"
			case "run":
				observed.RunID = "999-1"
			case "identity", "dirty":
				binding := observed.DatabaseBindings["mongodb"]
				binding.Dirty = change == "dirty"
				if change == "identity" {
					binding.IdentityHash = strings.Repeat("d", 64)
				}
				observed.DatabaseBindings["mongodb"] = binding
			case "boundary":
				observed.Targets[0].Boundary.SchemaHash = strings.Repeat("d", 64)
			}
			writePrepareFactsOriginalBoundary(t, &r, observed, requestPath)
			if err := validatePrepareFactsOriginalRequest(r, requestPath, uint32(os.Getuid())); err == nil {
				t.Fatal("changed original report accepted")
			}
		})
	}
}

func TestPrepareFactsOriginalBoundaryKeepsOwnedReaderProtection(t *testing.T) {
	for _, change := range []string{"hash", "permissions", "symlink", "hardlink", "case-alias", "duplicate", "unknown-field"} {
		t.Run(change, func(t *testing.T) {
			r, observed, requestPath := prepareFactsOriginalBoundaryFixture(t)
			path := writePrepareFactsOriginalBoundary(t, &r, observed, requestPath)
			var err error
			switch change {
			case "hash":
				r.BoundaryReportHash = strings.Repeat("e", 64)
			case "permissions":
				err = os.Chmod(path, 0644)
			case "symlink":
				err = os.Rename(path, path+".original")
				if err == nil {
					err = os.Symlink(path+".original", path)
				}
			case "hardlink":
				err = os.Link(path, path+".alias")
			default:
				var raw []byte
				raw, err = os.ReadFile(path)
				if change == "case-alias" {
					raw = bytes.Replace(raw, []byte(`"source_sha"`), []byte(`"Source_SHA"`), 1)
				} else {
					field := `"unexpected":true`
					if change == "duplicate" {
						field = `"format_version":2`
					}
					raw = append(raw[:len(raw)-1], []byte(","+field+"}")...)
				}
				if err == nil {
					err = os.WriteFile(path, raw, 0600)
				}
				r.BoundaryReportHash = digestRaw(raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = validatePrepareFactsOriginalRequest(r, requestPath, uint32(os.Getuid())); err == nil {
				t.Fatal("unsafe original boundary accepted")
			}
		})
	}
}

func TestPrepareFactsKeepsOriginalProducerAndRejectsAuthorityFields(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	if r.SourceSHA == r.Inventory.SourceSHA || validatePrepareFactsRequest(r, "123-1", "789-1") != nil {
		t.Fatal("separate producer binding rejected")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`"drop_ready":true`, `"ordered_mongo_schema_sha256":"approved"`, `"manifest_sha256":"approved"`} {
		var got prepareFactsRequest
		if decodePrepareFacts(append(raw[:len(raw)-1], []byte(","+extra+"}")...), &got) == nil {
			t.Fatal("authority field accepted")
		}
	}
	var got prepareFactsRequest
	if decodePrepareFacts([]byte(strings.Replace(string(raw), `"source_sha"`, `"Source_SHA"`, 1)), &got) == nil {
		t.Fatal("case alias accepted")
	}
	if decodePrepareFacts([]byte(strings.Replace(string(raw), `"format_version":1`, `"format_version":1,"format_version":1`, 1)), &got) == nil {
		t.Fatal("duplicate accepted")
	}
	mutations := []func(*prepareFactsRequest){
		func(v *prepareFactsRequest) { v.Inventory.SourceSHA = "unknown" },
		func(v *prepareFactsRequest) { v.Inventory.RunID = v.ActualRunID },
		func(v *prepareFactsRequest) { v.Inventory.OperationID = "999-1" },
		func(v *prepareFactsRequest) {
			v.RestoreEngines = &lifecycleRestoreEngines{MySQLImageID: "mysql:8", MongoImageID: "mongo:7", Architecture: runtime.GOARCH}
		},
		func(v *prepareFactsRequest) {
			v.ArchiveDirectory = "/opt/backups/qs-server/compatibility-retirement/123-1/inventory-456-1/child"
		},
	}
	for _, mutate := range mutations {
		got := r
		mutate(&got)
		if validatePrepareFactsRequest(got, "123-1", "789-1") == nil {
			t.Fatal("invalid binding accepted")
		}
	}
}

func TestStorageBackupRequestSeparatesOriginalMetadataFromCaptureTool(t *testing.T) {
	facts := prepareFactsRequestFixture(t)
	r := storageBackupRequest{FormatVersion: 1, Kind: "readonly_storage_backup_request", SourceSHA: sourceSHA, OriginalSourceSHA: facts.Inventory.SourceSHA, OperationID: "123-1", ActualRunID: "789-1", TargetHash: digest(targets), ObservationApprovalSHA256: strings.Repeat("a", 64), InventoryRequestSHA256: facts.Inventory.RequestSHA256, BoundaryReport: facts.Inventory}
	if validateStorageBackupRequest(r, "123-1", "789-1") != nil {
		t.Fatal("actual original/tool separation rejected")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`"manifest_sha256":"approved"`, `"complete":true`, `"source_file_sha256":{}`} {
		var got storageBackupRequest
		if decodePrepareFacts(append(append([]byte(nil), raw[:len(raw)-1]...), []byte(","+extra+"}")...), &got) == nil {
			t.Fatal("supplied completion or content accepted")
		}
	}
	for _, mutate := range []func(*storageBackupRequest){
		func(v *storageBackupRequest) { v.SourceSHA = v.OriginalSourceSHA },
		func(v *storageBackupRequest) { v.BoundaryReport.SourceSHA = v.SourceSHA },
		func(v *storageBackupRequest) { v.BoundaryReport.RunID = v.ActualRunID },
		func(v *storageBackupRequest) { v.InventoryRequestSHA256 = "unknown" },
	} {
		got := r
		mutate(&got)
		if validateStorageBackupRequest(got, "123-1", "789-1") == nil {
			t.Fatal("origin conflict accepted")
		}
	}
	facts.StorageBackup = &lifecycleStorageBackup{Kind: "storage_backup_v1", ToolSourceSHA: sourceSHA, RunID: facts.Inventory.RunID, ArchiveSHA256: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64)}
	if validatePrepareFactsRequest(facts, "123-1", "789-1") != nil {
		t.Fatal("storage origin rejected")
	}
	// Reuse preserves the actual older capture producer; it does not become
	// the current observer or the original metadata producer.
	facts.StorageBackup.ToolSourceSHA = strings.Repeat("f", 40)
	if validatePrepareFactsRequest(facts, "123-1", "789-1") != nil || facts.StorageBackup.ToolSourceSHA == facts.SourceSHA || facts.StorageBackup.ToolSourceSHA == facts.Inventory.SourceSHA {
		t.Fatal("separate storage producer rejected or relabeled")
	}
	baseline := backup.StorageBaseline{Kind: "storage_backup_v1", ArchiveSHA256: facts.StorageBackup.ArchiveSHA256,
		Approval:        backup.Approval{SourceSHA: facts.Inventory.SourceSHA, OperationID: facts.OperationID, RunID: facts.Inventory.RunID, InventorySHA256: facts.Inventory.ReportSHA256, RequestHash: facts.Inventory.RequestSHA256},
		StorageApproval: backup.StorageApproval{ToolSourceSHA: facts.StorageBackup.ToolSourceSHA, RunID: facts.StorageBackup.RunID}}
	if !prepareFactsStorageBaselineMatches(facts, baseline) {
		t.Fatal("actual older storage baseline rejected")
	}
	for _, mutate := range []func(*backup.StorageBaseline){
		func(v *backup.StorageBaseline) { v.StorageApproval.ToolSourceSHA = facts.SourceSHA },
		func(v *backup.StorageBaseline) { v.StorageApproval.RunID = facts.ActualRunID },
		func(v *backup.StorageBaseline) { v.ArchiveSHA256 = strings.Repeat("9", 64) },
		func(v *backup.StorageBaseline) { v.Approval.RequestHash = strings.Repeat("9", 64) },
		func(v *backup.StorageBaseline) { v.Approval.InventorySHA256 = strings.Repeat("9", 64) },
		func(v *backup.StorageBaseline) { v.DropReady = true },
	} {
		got := baseline
		mutate(&got)
		if prepareFactsStorageBaselineMatches(facts, got) {
			t.Fatal("different native storage producer or bytes accepted")
		}
	}
	for _, mutate := range []func(*prepareFactsRequest){
		func(v *prepareFactsRequest) { v.SourceSHA = v.StorageBackup.ToolSourceSHA },
		func(v *prepareFactsRequest) { v.StorageBackup.ToolSourceSHA = "unknown" },
		func(v *prepareFactsRequest) { v.StorageBackup.RunID = v.ActualRunID },
		func(v *prepareFactsRequest) { v.StorageBackup.RequestSHA256 = "" },
		func(v *prepareFactsRequest) { v.StorageBackup.ArchiveSHA256 = "" },
	} {
		got := facts
		storage := *facts.StorageBackup
		got.StorageBackup = &storage
		mutate(&got)
		if validatePrepareFactsRequest(got, "123-1", "789-1") == nil {
			t.Fatal("invalid current observer or producer reference accepted")
		}
	}
}

func TestStorageBackupFailureRetainsBindingsAndClearsSuccessProof(t *testing.T) {
	deadline, stopDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stopDeadline()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want string
	}{
		{"parent deadline masks read", deadline, backup.ErrRead, "storage_backup_timeout"},
		{"parent cancellation masks budget", cancelled, backup.ErrBudget, "storage_backup_cancelled"},
		{"parent cancellation precedes returned timeout", cancelled, context.DeadlineExceeded, "storage_backup_cancelled"},
		{"wrapped timeout", context.Background(), fmt.Errorf("PRIVATE_NATIVE_MESSAGE: %w", context.DeadlineExceeded), "storage_backup_timeout"},
		{"wrapped cancellation", context.Background(), fmt.Errorf("PRIVATE_NATIVE_MESSAGE: %w", context.Canceled), "storage_backup_cancelled"},
		{"wrapped budget", context.Background(), fmt.Errorf("PRIVATE_NATIVE_MESSAGE: %w", backup.ErrBudget), "storage_backup_budget_exceeded"},
		{"wrapped read", context.Background(), fmt.Errorf("PRIVATE_NATIVE_MESSAGE: %w", backup.ErrRead), "storage_backup_read_failed"},
		{"wrapped content", context.Background(), fmt.Errorf("PRIVATE_NATIVE_MESSAGE: %w", backup.ErrContent), "storage_backup_content_rejected"},
		{"wrapped structure", context.Background(), fmt.Errorf("PRIVATE_NATIVE_MESSAGE: %w", backup.ErrStructure), "storage_backup_structure_rejected"},
		{"wrapped permission", context.Background(), &os.PathError{Op: "open", Path: "PRIVATE_NATIVE_MESSAGE", Err: os.ErrPermission}, "storage_backup_permission_denied"},
		{"private rejection is not permission", context.Background(), backup.ErrPrivate, "storage_backup_failed"},
		{"unmapped approval", context.Background(), backup.ErrApproval, "storage_backup_failed"},
		{"unmapped source", context.Background(), backup.ErrSource, "storage_backup_failed"},
		{"unknown error", context.Background(), errors.New("PRIVATE_NATIVE_MESSAGE"), "storage_backup_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := storageBackupReceipt{FormatVersion: 1, Kind: "readonly_storage_backup_result", Operation: "prepare", PrepareMode: "storage-backup", SourceSHA: strings.Repeat("a", 40), OriginalSourceSHA: strings.Repeat("b", 40), OperationID: "123-1", RunID: "789-1", RequestSHA256: strings.Repeat("c", 64), InventoryRequestSHA256: strings.Repeat("d", 64), ObservationApprovalSHA256: strings.Repeat("e", 64), TargetHash: digest(targets), Complete: true, ExecutionAllowed: true, DropReady: true, StorageBackupComplete: true, ArchiveSHA256: strings.Repeat("f", 64), ArchiveApproval: &backup.Approval{}, StorageBackup: &lifecycleStorageBackup{}, SQLRecoveryBaselineSHA256: strings.Repeat("1", 64), Files: []prepareFactsFile{{Name: "PRIVATE_NATIVE_MESSAGE"}}, Manifest: &lifecycleFrozenManifest{}, ErrorCategory: "none"}
			bindings := r
			r.setStorageBackupFailure(tc.ctx, tc.err)
			if r.ErrorCategory != tc.want || r.Complete || r.ExecutionAllowed || r.DropReady || r.StorageBackupComplete || !r.DiagnosticOnly {
				t.Fatalf("unsafe failure state: category=%s", r.ErrorCategory)
			}
			if r.SourceSHA != bindings.SourceSHA || r.OriginalSourceSHA != bindings.OriginalSourceSHA || r.OperationID != bindings.OperationID || r.RunID != bindings.RunID || r.RequestSHA256 != bindings.RequestSHA256 || r.InventoryRequestSHA256 != bindings.InventoryRequestSHA256 || r.ObservationApprovalSHA256 != bindings.ObservationApprovalSHA256 || r.TargetHash != bindings.TargetHash {
				t.Fatal("failure lost original producer/request bindings")
			}
			raw, err := json.Marshal(r)
			var public map[string]json.RawMessage
			if err != nil || json.Unmarshal(raw, &public) != nil || bytes.Contains(raw, []byte("PRIVATE_NATIVE_MESSAGE")) || string(public["storage_source_files"]) != "[]" {
				t.Fatal("failure disclosed private content or retained partial files")
			}
			for _, name := range []string{"storage_archive_sha256", "archive_approval", "storage_backup", "mysql_recovery_non_target_sha256", "inventory_manifest_seed"} {
				if _, ok := public[name]; ok {
					t.Fatalf("failure retained success field %s", name)
				}
			}
		})
	}
}

func TestStorageBackupDeferredFailureAndNonfailureCategories(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := runStorageBackup(ctx, "unapproved-path", strings.Repeat("a", 64), "123-1", "789-1", "unapproved-output")
	if err == nil || r.ErrorCategory != "storage_backup_cancelled" || r.StorageBackupComplete || len(r.Files) != 0 {
		t.Fatal("actual rejection defer did not retain cancellation")
	}
	for _, category := range []string{"none", "storage_backup_incomplete"} {
		r := storageBackupReceipt{ErrorCategory: category, StorageBackupComplete: category == "none", Files: []prepareFactsFile{}}
		r.setStorageBackupFailure(context.Background(), nil)
		if r.ErrorCategory != category || r.StorageBackupComplete != (category == "none") {
			t.Fatal("nonfailure state changed")
		}
	}
}

func TestPrepareFactsHashesPrivateBytesWithoutCopyOrDisclosure(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "source.private")
	body := []byte("PRIVATE_SOURCE_BODY_NOT_A_RECEIPT")
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	fact, kept, err := hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false)
	if err != nil || kept != nil || fact.SHA256 != digestRaw(body) || fact.Bytes != uint64(len(body)) {
		t.Fatalf("actual hash failed: %v", err)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), string(body)) {
		t.Fatal("body disclosed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("source copied")
	}
	if err = os.Link(path, filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("hardlink accepted")
	}
	if err = os.Remove(filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("public source accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = hashPrepareFactsFile(ctx, path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("cancelled source read accepted")
	}
}

func TestPrepareFactsReportsActualCapacityWithoutReadiness(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	fact, err := observePrepareFactsFS("archive", dir)
	if err != nil || fact.Scope != "archive" || fact.PathSHA256 != digestRaw([]byte(dir)) || fact.TotalBytes == 0 || fact.FreeBytes > fact.TotalBytes || fact.AvailableBytes > fact.TotalBytes {
		t.Fatalf("actual statfs observation failed: %v", err)
	}
	parent, err := prepareFactsArchiveParent(filepath.Join(dir, "new", "leaf"))
	if err != nil || parent != dir {
		t.Fatal("wrong actual archive parent")
	}
	if err = os.Symlink(dir, filepath.Join(dir, "redirect")); err != nil {
		t.Fatal(err)
	}
	if _, err = observePrepareFactsFS("archive", filepath.Join(dir, "redirect")); err == nil {
		t.Fatal("symlink accepted")
	}
	receipt := prepareFactsReceipt{FormatVersion: 1, Kind: "readonly_prepare_facts_observation", DiagnosticOnly: true, FactsObservationComplete: true}
	if receipt.Complete || receipt.DropReady || receipt.ExecutionAllowed || receipt.OrderedMongoSchemaSHA256 != "" {
		t.Fatal("capacity observation minted authority")
	}
}

func TestPrepareFactsRequiresOriginalCompleteInventoryAndExactFourSources(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	req := request{FormatVersion: 2, Kind: "readonly_inventory_request", OperationID: r.OperationID, SourceSHA: r.Inventory.SourceSHA,
		TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Limits: productionLimits(),
		Identities: map[string]string{"mysql": strings.Repeat("1", 64), "mongodb": strings.Repeat("2", 64)},
		Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}, BoundaryRunID: "222-1", BoundaryReportHash: strings.Repeat("3", 64)}
	observed := report{FormatVersion: 2, Kind: "readonly_compatibility_inventory", SourceSHA: req.SourceSHA, OperationID: r.OperationID,
		RunID: r.Inventory.RunID, RequestHash: r.Inventory.RequestSHA256, TargetHash: digest(targets), Complete: true, DiagnosticOnly: true,
		BoundaryReportHash: req.BoundaryReportHash, ErrorCategory: "none", DatabaseBindings: map[string]databaseInventory{}}
	for _, db := range []string{"mysql", "mongodb"} {
		observed.DatabaseBindings[db] = databaseInventory{IdentityHash: req.Identities[db], DatabaseAnchorHash: strings.Repeat("4", 64), Version: req.Migrations[db], MetadataComplete: true, ExpectedIdentityMatch: true, ExpectedMigrationMatch: true, CatalogHash: strings.Repeat("5", 64)}
	}
	for i, target := range targets {
		b := targetBoundary{Database: target[0], Name: target[1], Kind: target[2], Present: true, Empty: true, SchemaHash: strings.Repeat("6", 64), IdentityHash: strings.Repeat("7", 64)}
		req.Boundaries = append(req.Boundaries, b)
		observed.Targets = append(observed.Targets, snapshot{Database: target[0], Name: target[1], Kind: target[2], Present: true, Complete: true, ErrorCategory: "none", Boundary: &b, Passes: 2, SourceFile: lifecycleSourceNames[i+3]})
	}
	if e := validatePrepareFactsInventory(r, req, observed); e != nil {
		t.Fatal(e)
	}
	mutations := []func(*report){func(v *report) { v.Complete = false }, func(v *report) { v.SourceSHA = sourceSHA }, func(v *report) { v.RequestHash = strings.Repeat("8", 64) }, func(v *report) { v.Targets[3].SourceFile = "wrong.source" }, func(v *report) { b := v.DatabaseBindings["mongodb"]; b.Dirty = true; v.DatabaseBindings["mongodb"] = b }, func(v *report) { v.Targets[0].Passes = 1 }}
	for _, mutate := range mutations {
		encoded, e := json.Marshal(observed)
		if e != nil {
			t.Fatal(e)
		}
		var changed report
		if e = json.Unmarshal(encoded, &changed); e != nil {
			t.Fatal(e)
		}
		mutate(&changed)
		if validatePrepareFactsInventory(r, req, changed) == nil {
			t.Fatal("unproven inventory accepted")
		}
	}
	req.Migrations["mysql"] = 100
	if validatePrepareFactsInventory(r, req, observed) == nil {
		t.Fatal("different schema phase accepted")
	}
}

func TestPrepareFactsRequestPathBindsActualRunWithoutChangingProducer(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	path := "/opt/backups/qs-server/compatibility-retirement/123-1/prepare-facts-request-789-1.json"
	if !prepareFactsBoundRequestPath(path, r.OperationID, r.ActualRunID) {
		t.Fatal("exact actual run path rejected")
	}
	for _, invalid := range []struct{ path, op, run string }{
		{path, "123-1", "790-1"},
		{path, "124-1", "789-1"},
		{"/opt/backups/qs-server/compatibility-retirement/123-1/prepare-facts-request.json", "123-1", "789-1"},
		{path, "123-1", "../789-1"},
		{"/opt/backups/qs-server/compatibility-retirement/123-1/./prepare-facts-request-789-1.json", "123-1", "789-1"},
	} {
		if prepareFactsBoundRequestPath(invalid.path, invalid.op, invalid.run) {
			t.Fatal("conflicting run/path accepted")
		}
	}
	original := r.Inventory
	r.ActualRunID = "790-1"
	if validatePrepareFactsRequest(r, "123-1", "790-1") != nil || r.Inventory != original {
		t.Fatal("new actual run changed original inventory producer")
	}
	if validatePrepareFactsRequest(r, "123-1", "789-1") == nil {
		t.Fatal("request actual run conflict accepted")
	}
}

func TestPrepareFactsProtectionInspectFormatIncludesRuntimeArguments(t *testing.T) {
	tmpl, e := template.New("actual-inspect").Funcs(template.FuncMap{"json": func(v any) (string, error) {
		raw, e := json.Marshal(v)
		return string(raw), e
	}}).Parse(prepareFactsMQInspect)
	if e != nil {
		t.Fatal(e)
	}
	actual := map[string]any{"Id": strings.Repeat("1", 64), "Name": "/qs-apiserver", "Image": "sha256:" + strings.Repeat("2", 64), "State": map[string]any{"Running": true, "Pid": 123}, "Config": map[string]any{"User": "www", "Entrypoint": []string{"/app/qs-apiserver"}, "Cmd": []string{"--config=/app/configs/apiserver.prod.yaml"}, "Labels": map[string]string{"com.docker.compose.service": "qs-apiserver"}}, "Mounts": []any{}}
	var rendered bytes.Buffer
	var decoded prepareFactsMQContainer
	if e = tmpl.Execute(&rendered, actual); e != nil || json.Unmarshal(rendered.Bytes(), &decoded) != nil || len(decoded.Entrypoint) != 1 || decoded.Entrypoint[0] != "/app/qs-apiserver" || len(decoded.Command) != 1 || decoded.Command[0] != "--config=/app/configs/apiserver.prod.yaml" || decoded.User != "www" || decoded.PID != 123 {
		t.Fatal("actual fixed inspect format lost required runtime fields", e)
	}
}

func TestPrepareFactsProtectionUsesExistingHostLoaderAndClosedPrivatePacket(t *testing.T) {
	key := func(kid string) jose.JSONWebKey {
		t.Helper()
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		return jose.JSONWebKey{Key: k, KeyID: kid}
	}
	write := func(k jose.JSONWebKey) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), k.KeyID+".json")
		raw, e := json.Marshal(k)
		if e != nil || os.WriteFile(p, raw, 0600) != nil {
			t.Fatal("fixture creation")
		}
		return p
	}
	sign, crypt, old, aiSign, aiCrypt := key("qs.sign.v1"), key("qs.encrypt.v1"), key("qs.encrypt.old"), key("ai.sign.v1"), key("ai.encrypt.v1")
	options := opts.AIWorkflowMessagingOptions{SigningKeyFile: write(sign), AIRecipientKeyFile: write(aiCrypt.Public()), DecryptKeyFiles: map[string]string{crypt.KeyID: write(crypt), old.KeyID: write(old)}, AISignerFiles: map[string]string{aiSign.KeyID: write(aiSign.Public())}}
	keys, e := aibridge.LoadMessagingKeys(options)
	if e != nil {
		t.Fatal(e)
	}
	raw, publics, e := prepareFactsProtectionBytes(keys)
	if e != nil || len(publics) != 5 {
		t.Fatal("real host keys rejected")
	}
	var packet map[string]json.RawMessage
	if json.Unmarshal(raw, &packet) != nil || len(packet) != 2 || packet["decrypt_keys"] == nil || packet["trusted_signers"] == nil {
		t.Fatal("closed DTO rejected")
	}
	var decrypt map[string]map[string]any
	var signers map[string]map[string]json.RawMessage
	if json.Unmarshal(packet["decrypt_keys"], &decrypt) != nil || json.Unmarshal(packet["trusted_signers"], &signers) != nil || len(decrypt) != 2 || len(signers) != 1 {
		t.Fatal("ring scope wrong")
	}
	if decrypt[crypt.KeyID]["d"] == nil || decrypt[old.KeyID]["d"] == nil || len(signers[aiSign.KeyID]) != 2 || string(signers[aiSign.KeyID]["producer"]) != `"qs-ai"` {
		t.Fatal("private/producer role lost")
	}
	var public map[string]any
	if json.Unmarshal(signers[aiSign.KeyID]["key"], &public) != nil || public["d"] != nil {
		t.Fatal("private signer leaked")
	}
	for _, mutation := range []string{"empty", "public_decrypt", "private_signer", "wrong_producer", "wrong_kid"} {
		t.Run(mutation, func(t *testing.T) {
			clone, e := aibridge.LoadMessagingKeys(options)
			if e != nil {
				t.Fatal(e)
			}
			switch mutation {
			case "empty":
				clone.Ring.Decrypt = nil
			case "public_decrypt":
				clone.Ring.Decrypt[crypt.KeyID] = crypt.Public()
			case "private_signer":
				v := clone.Ring.Signers[aiSign.KeyID]
				v.Key = aiSign
				clone.Ring.Signers[aiSign.KeyID] = v
			case "wrong_producer":
				v := clone.Ring.Signers[aiSign.KeyID]
				v.Producer = "other"
				clone.Ring.Signers[aiSign.KeyID] = v
			case "wrong_kid":
				v := clone.Ring.Decrypt[crypt.KeyID]
				v.KeyID = "qs.encrypt.other"
				clone.Ring.Decrypt[crypt.KeyID] = v
			}
			if _, _, e = prepareFactsProtectionBytes(clone); e == nil {
				t.Fatal("unsafe packet accepted")
			}
		})
	}
	receipt := prepareFactsReceipt{Protection: &prepareFactsProtection{SHA256: digestRaw(raw), DecryptKeys: 2, TrustedSigners: 1, BindingSHA256: strings.Repeat("a", 64), SourceSHA: strings.Repeat("b", 40)}}
	publicReceipt, e := json.Marshal(receipt)
	if e != nil || strings.Contains(string(publicReceipt), `"decrypt_keys"`) || strings.Contains(string(publicReceipt), `"trusted_signers"`) || receipt.DropReady || receipt.Complete || receipt.ExecutionAllowed {
		t.Fatal("receipt leaked keys or authority")
	}
}

func TestPrepareFactsProtectionActualFDRejectsUnsafePrivateFiles(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("dir")
	}
	p := filepath.Join(dir, "key.json")
	raw := []byte("synthetic-key-bytes")
	if os.WriteFile(p, raw, 0600) != nil {
		t.Fatal("fixture")
	}
	f, got, e := prepareFactsKeyFile(p, uint32(os.Getuid()), true, 16384)
	if e != nil || string(got) != string(raw) || f == nil {
		t.Fatal("actual FD rejected", e)
	}
	if f.Close() != nil {
		t.Fatal("close")
	}
	if os.Chmod(p, 0644) != nil {
		t.Fatal("chmod")
	}
	if f, _, e = prepareFactsKeyFile(p, uint32(os.Getuid()), true, 16384); e == nil || f != nil {
		t.Fatal("public private key accepted")
	}
	if os.Chmod(p, 0600) != nil {
		t.Fatal("chmod")
	}
	link := filepath.Join(dir, "hard")
	if os.Link(p, link) != nil {
		t.Fatal("link")
	}
	if f, _, e = prepareFactsKeyFile(p, uint32(os.Getuid()), true, 16384); e == nil || f != nil {
		t.Fatal("hardlink accepted")
	}
	if os.Remove(link) != nil {
		t.Fatal("unlink")
	}
	if os.Symlink(p, link) != nil {
		t.Fatal("symlink")
	}
	if f, _, e = prepareFactsKeyFile(link, uint32(os.Getuid()), true, 16384); e == nil || f != nil {
		t.Fatal("symlink accepted")
	}
}

func TestPrepareFactsProtectionReaderUIDComesFromActualProcess(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("actual nonroot Linux /proc required")
	}
	uid, e := prepareFactsContainerReaderUID(os.Getpid(), strconv.Itoa(os.Getuid()))
	if e != nil || uid != uint32(os.Getuid()) {
		t.Fatal("actual process UID rejected", e)
	}
	if _, e = prepareFactsContainerReaderUID(os.Getpid(), strconv.Itoa(os.Getuid()+1)); e == nil {
		t.Fatal("Config.User/actual process mismatch accepted")
	}
	for _, user := range []string{"root", "0", "1000:1000:1000", "untrusted-user", ""} {
		if _, e = prepareFactsContainerReaderUID(os.Getpid(), user); e == nil {
			t.Fatal("unbound reader accepted")
		}
	}
	if _, e = prepareFactsContainerReaderUID(0, "www"); e == nil {
		t.Fatal("missing PID accepted")
	}
}
