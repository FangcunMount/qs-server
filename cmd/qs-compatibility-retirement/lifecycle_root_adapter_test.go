package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

func TestLifecycleRootCopyPreservesBytesOwnerAndRejectsRebinding(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory")
	}
	src := filepath.Join(dir, "source")
	raw := []byte("{\"raw\":\"ordered\\nbytes\"}\n")
	if os.WriteFile(src, raw, 0600) != nil {
		t.Fatal("fixture")
	}
	before, _ := os.Stat(src)
	dst := filepath.Join(dir, "copy")
	if err := copyLifecycleRootSource(src, dst, digestRaw(raw), uint32(os.Getuid())); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	after, _ := os.Stat(src)
	if !bytes.Equal(raw, got) || !sameLifecycleFile(before, after) {
		t.Fatal("original modified")
	}
	for _, row := range []struct {
		name string
		run  func() error
	}{{"different_digest", func() error {
		return copyLifecycleRootSource(src, filepath.Join(dir, "different"), strings.Repeat("a", 64), uint32(os.Getuid()))
	}}, {"owner", func() error {
		return copyLifecycleRootSource(src, filepath.Join(dir, "owner"), digestRaw(raw), uint32(os.Getuid()+1))
	}}, {"existing", func() error { return copyLifecycleRootSource(src, dst, digestRaw(raw), uint32(os.Getuid())) }}} {
		t.Run(row.name, func(t *testing.T) {
			if row.run() == nil {
				t.Fatal("rebind accepted")
			}
		})
	}
	link := filepath.Join(dir, "symbolic")
	if os.Symlink(src, link) != nil {
		t.Fatal("link")
	}
	if copyLifecycleRootSource(link, filepath.Join(dir, "linkcopy"), digestRaw(raw), uint32(os.Getuid())) == nil {
		t.Fatal("symlink accepted")
	}
	hard := filepath.Join(dir, "hard")
	if os.Link(src, hard) != nil {
		t.Fatal("hard link")
	}
	if copyLifecycleRootSource(src, filepath.Join(dir, "hardcopy"), digestRaw(raw), uint32(os.Getuid())) == nil {
		t.Fatal("hardlink accepted")
	}
}
func TestLifecycleRootCopyAcceptsActualEmptyRawSource(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory")
	}
	src := filepath.Join(dir, "empty")
	if os.WriteFile(src, nil, 0600) != nil {
		t.Fatal("fixture")
	}
	if err := copyLifecycleRootSource(src, filepath.Join(dir, "copy"), digestRaw(nil), uint32(os.Getuid())); err != nil {
		t.Fatal(err)
	}
}
func TestLifecycleRootRequestKeepsOriginalAndToolSeparate(t *testing.T) {
	for _, raw := range []string{`{"original_source_sha":"a","tool_source_sha":"b","source_file_sha256":{"x":"y"}}`, `{"original_source_sha":"a","tool_source_sha":"b","restore_engines":{"mysql_image_id":"a","mongodb_image_id":"b","architecture":"arm64"}}`} {
		r, e := decodeLifecycleStagingRequest([]byte(raw))
		if e != nil || r.OriginalSourceSHA != "a" || r.ToolSourceSHA != "b" {
			t.Fatal("binding collapsed")
		}
	}
	for _, raw := range []string{`{"restore_engines":{"architecture":"arm64","Architecture":"amd64"}}`, `{"source_file_sha256":{},"source_file_sha256":{}}`, `{"prepareRoot":"/fake"}`, `{"source_file_sha256":{}} {}`} {
		if _, e := decodeLifecycleStagingRequest([]byte(raw)); e == nil {
			t.Fatal("alias/proof field accepted")
		}
	}
}
func TestLifecycleRestoreImagesNeedActualImmutableArchitectureBinding(t *testing.T) {
	valid := lifecycleRestoreEngines{MySQLImageID: "sha256:" + strings.Repeat("a", 64), MongoImageID: "sha256:" + strings.Repeat("b", 64), Architecture: runtime.GOARCH}
	if !valid.valid() {
		t.Fatal("supported arch fixture")
	}
	for _, mutate := range []func(*lifecycleRestoreEngines){func(v *lifecycleRestoreEngines) { v.MySQLImageID = "mysql:8" }, func(v *lifecycleRestoreEngines) { v.MongoImageID = "mongo:7" }, func(v *lifecycleRestoreEngines) { v.Architecture = "wrong" }} {
		v := valid
		mutate(&v)
		if v.valid() {
			t.Fatal("mutable/foreign image accepted")
		}
	}
}
func TestLifecycleWireUsesActualChildPipesAndReapsOnDeadline(t *testing.T) {
	// Offline pipe-process boundary only: no Docker/database/socket is used and
	// this process cannot mint any RestoreVerification or production isolation.
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory")
	}
	program := filepath.Join(dir, "pipe-child")
	if os.WriteFile(program, []byte("#!/bin/sh\nexec cat\n"), 0700) != nil {
		t.Fatal("pipe fixture")
	}
	c, e := openLifecycleWireConn(context.Background(), program, strings.Repeat("a", 64), "mysql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Write([]byte("exactwire")); e != nil {
		t.Fatal(e)
	}
	got := make([]byte, 9)
	if _, e = io.ReadFull(c, got); e != nil || string(got) != "exactwire" {
		t.Fatal("pipe bytes changed")
	}
	if c.SetReadDeadline(time.Now().Add(20*time.Millisecond)) != nil {
		t.Fatal("deadline")
	}
	_, e = c.Read(make([]byte, 1))
	var ne net.Error
	if e == nil {
		t.Fatal("deadline did not unblock")
	}
	ne, _ = e.(net.Error)
	if ne == nil || !ne.Timeout() {
		t.Fatal("deadline category")
	}
	if c.Close() != nil {
		t.Fatal("close")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("child not reaped")
	}
}
func TestLifecycleWireRejectsUnknownEndpointBeforeExec(t *testing.T) {
	for _, kind := range []string{"", "tcp", "mongodb-other"} {
		if _, e := openLifecycleWireConn(context.Background(), "not-executed", strings.Repeat("a", 64), kind); e == nil {
			t.Fatal("kind accepted")
		}
	}
	if _, e := openLifecycleWireConn(context.Background(), "not-executed", "short", "mysql"); e == nil {
		t.Fatal("unknown container accepted")
	}
}

func TestLifecycleWireDirectionDeadlinesAndLiveExtensions(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "pipe-child")
	if os.WriteFile(program, []byte("#!/bin/sh\nexec cat\n"), 0700) != nil {
		t.Fatal("pipe fixture")
	}
	c, e := openLifecycleWireConn(context.Background(), program, strings.Repeat("a", 64), "mysql")
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = c.Close() }()
	if _, e = c.Write([]byte("a")); e != nil {
		t.Fatal(e)
	}
	if c.SetWriteDeadline(time.Now().Add(-time.Second)) != nil {
		t.Fatal("write deadline")
	}
	p := make([]byte, 1)
	if _, e = io.ReadFull(c, p); e != nil || string(p) != "a" {
		t.Fatal("expired idle write deadline interrupted read")
	}
	if c.SetWriteDeadline(time.Time{}) != nil {
		t.Fatal("clear write deadline")
	}
	if c.SetReadDeadline(time.Now().Add(300*time.Millisecond)) != nil {
		t.Fatal("read deadline")
	}
	result := make(chan error, 1)
	go func() { _, e := io.ReadFull(c, p); result <- e }()
	time.Sleep(10 * time.Millisecond)
	if c.SetReadDeadline(time.Now().Add(time.Second)) != nil {
		t.Fatal("extend deadline")
	}
	if _, e = c.Write([]byte("b")); e != nil {
		t.Fatal(e)
	}
	if e = <-result; e != nil || string(p) != "b" {
		t.Fatal("active deadline update changed wire bytes")
	}
}

func TestLifecyclePreparationDeadlineIncludesActualPipeCleanup(t *testing.T) {
	// Real local child + actual join, near an expired parent deadline. It cannot
	// construct a database proof; this isolates the cleanup-inclusive success gate.
	dir := t.TempDir()
	program := filepath.Join(dir, "pipe-child")
	if os.WriteFile(program, []byte("#!/bin/sh\nexec cat\n"), 0700) != nil {
		t.Fatal("pipe fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	c, e := openLifecycleWireConn(ctx, program, strings.Repeat("a", 64), "mysql")
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	owner := &lifecyclePreparationOwner{restoreContext: ctx, restoreCancel: cancel, combinedStarted: time.Now(), engines: []*lifecycleOwnedEngine{{wires: []*lifecycleWireConn{c}}}}
	<-ctx.Done() // original parent budget expires; no new background success budget
	if _, e = owner.finishPreparation(); e != lifecycleError("lifecycle_combined_restore_budget_exceeded") {
		t.Fatal("cleanup did not obey original deadline", e)
	}
	select {
	case <-c.done:
	default:
		t.Fatal("actual child not joined before failure")
	}
}

// This narrow fixture exercises the actual request readers with owned files in
// the fixed native paths. It supplies no database, archive, fence or recovery
// capability; distinct calls must still fail at the next missing-input gate.
func TestLifecycleNativeRunReuseRejectedBeforeLaterGates(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("actual Linux/root request fixture required")
	}
	oldSource := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = oldSource })
	t.Setenv("QS_RETIREMENT_SOURCE_UID", strconv.Itoa(os.Getuid()))
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	operation, original, current := stamp+"-1", stamp+"-2", stamp+"-3"
	originalRoot := filepath.Join("/opt/backups/qs-server/compatibility-retirement", operation)
	request := func(run string) lifecycleRequest {
		r := lifecycleRequest{FormatVersion: 1, Kind: "compatibility_retirement_lifecycle_request", ToolSourceSHA: sourceSHA,
			OriginalSourceSHA: strings.Repeat("b", 40), OperationID: operation, ActualRunID: run, ManifestSHA256: strings.Repeat("c", 64),
			ArchiveDirectory: filepath.Join(originalRoot, "archive"), WindowDirectory: filepath.Join(originalRoot, "window"), JournalDirectory: filepath.Join(originalRoot, "journal"),
			SourceDirectory: filepath.Join(originalRoot, "inventory-"+original), RestoreEngines: &lifecycleRestoreEngines{MySQLImageID: "sha256:" + strings.Repeat("d", 64), MongoImageID: "sha256:" + strings.Repeat("e", 64), Architecture: runtime.GOARCH},
			Approval:         backup.Approval{SourceSHA: strings.Repeat("b", 40), OperationID: operation, RunID: original},
			Recovery:         backup.TargetRecoveryRequest{SourceSHA: strings.Repeat("b", 40), OperationID: operation, OriginalRunID: original, ActualRunID: run, ManifestSHA256: strings.Repeat("c", 64), MongoNonTargetSHA256: strings.Repeat("f", 64), SQLHead: 99, MongoHead: 38},
			SourceFileSHA256: map[string]string{}}
		for _, name := range lifecycleSourceNames {
			r.SourceFileSHA256[name] = strings.Repeat("f", 64)
		}
		return r
	}
	for _, row := range []struct {
		name, run     string
		staging       bool
		recoveryReuse bool
		want          string
	}{
		{"staging_original_reuse", original, true, false, "lifecycle_staging_binding_rejected"},
		{"staging_recovery_original_reuse", current, true, true, "lifecycle_staging_binding_rejected"},
		{"staging_distinct_still_requires_ordered_schema", current, true, false, "lifecycle_ordered_mongo_schema_approval_missing"},
		{"loader_original_reuse", original, false, false, "lifecycle_binding_rejected"},
		{"loader_distinct_still_requires_manifest", current, false, false, "lifecycle_private_input_rejected"},
	} {
		t.Run(row.name, func(t *testing.T) {
			dir := lifecycleRootBatch(operation, row.run)
			if row.staging {
				dir = lifecycleInvocationBatch(operation, row.run)
			}
			if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal("exclusive fixture directory", err)
			}
			path := filepath.Join(dir, "lifecycle-request.json")
			t.Cleanup(func() {
				if err := os.Remove(path); err != nil {
					t.Error("owned request cleanup", err)
				}
				if err := os.Remove(dir); err != nil {
					t.Error("owned fixture directory cleanup", err)
				}
			})
			r := request(row.run)
			if row.recoveryReuse {
				r.Recovery.OriginalRunID = current
			}
			raw, err := json.Marshal(r)
			if err != nil || os.WriteFile(path, raw, 0600) != nil {
				t.Fatal("owned request fixture")
			}
			// A wrong digest must retain the earlier read/hash rejection, even
			// when this request also reuses an original run.
			if row.staging {
				_, err = stageLifecycleRootInputs(context.Background(), path, strings.Repeat("0", 64), operation, row.run)
				if lifecycleCategory(err) != "lifecycle_staging_bytes_changed_or_hash_rejected" {
					t.Fatalf("hash priority changed: %v", err)
				}
				_, err = stageLifecycleRootInputs(context.Background(), path, digestRaw(raw), operation, row.run)
			} else {
				_, _, err = loadLifecycleRequest(context.Background(), path, strings.Repeat("0", 64), operation, row.run, "prepare")
				if lifecycleCategory(err) != "lifecycle_private_input_rejected" {
					t.Fatalf("hash priority changed: %v", err)
				}
				_, archive, actualErr := loadLifecycleRequest(context.Background(), path, digestRaw(raw), operation, row.run, "prepare")
				if archive != nil {
					t.Fatal("fixture gained archive capability")
				}
				err = actualErr
			}
			if lifecycleCategory(err) != row.want {
				t.Fatalf("native run binding: got %v, want %s", err, row.want)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
				t.Fatal("rejection created staging or restore material")
			}
			if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, raw) {
				t.Fatal("rejection changed original input")
			}
		})
	}
}

// Offline original-producer files only. No Archive, acceptance, Window or
// production mutation is issued by this fixture or its retained filesystem FD.
func originalInventoryMaterialFixture(t *testing.T, records uint64) (string, backup.Approval, map[string]string) {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil || os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory")
	}
	a := backup.Approval{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", RunID: "456-1", RequestHash: strings.Repeat("b", 64)}
	hashes := map[string]string{}
	for i, name := range lifecycleSourceNames[1:] {
		raw := []byte("source-" + name)
		if records == 0 && i == 5 {
			raw = nil
		}
		if os.WriteFile(filepath.Join(dir, name), raw, 0600) != nil {
			t.Fatal("source")
		}
		hashes[name] = digestRaw(raw)
	}
	a.SQLMetadataSHA256 = hashes[lifecycleSourceNames[1]]
	a.MongoMetadataSHA256 = hashes[lifecycleSourceNames[2]]
	r := report{FormatVersion: 2, Kind: "readonly_compatibility_inventory", SourceSHA: a.SourceSHA, OperationID: a.OperationID, RunID: a.RunID, RequestHash: a.RequestHash, TargetHash: digest(targets), Complete: true, ErrorCategory: "none"}
	for i, v := range targets {
		b := targetBoundary{Database: v[0], Name: v[1], Kind: v[2], Present: true, Empty: records == 0}
		pageSize := uint64(productionLimits().PageSize)
		pages := records/pageSize + 1
		if records == 0 {
			pages = 0
		}
		s := snapshot{Database: v[0], Name: v[1], Kind: v[2], Present: true, Complete: true, Records: records, Bytes: records, DataHash: strings.Repeat("c", 64), SourceFile: lifecycleSourceNames[3+i], Boundary: &b, Pages: 2 * pages, Passes: 2}
		r.Targets = append(r.Targets, s)
		protocol := "mysql_cast_binary_columns_pk_order_v2"
		if i == 3 {
			protocol = "mongodb_server_bson_pk_order_v2"
		}
		asset := lifecycleInventorySourceAsset{1, "temporary_inventory_source_copy", s.SourceFile, a.SourceSHA, a.OperationID, a.RunID, a.RequestHash, protocol, b, true, false, true, false}
		if writeJSON(filepath.Join(dir, s.SourceFile+".asset.json"), asset) != nil {
			t.Fatal("asset")
		}
		for pass := 1; pass <= 2; pass++ {
			for page := 1; uint64(page) <= pages; page++ {
				count := min(uint64(page)*pageSize, records)
				p := lifecycleInventoryCheckpoint{1, "readonly_inventory_page_checkpoint", a.SourceSHA, pass, page, fmt.Sprint(count), count, count, s.DataHash, true, false}
				name := fmt.Sprintf("%s-%s-pass-%d-page-%06d.checkpoint.json", s.Database, s.Name, pass, page)
				if writeJSON(filepath.Join(dir, name), p) != nil {
					t.Fatal("checkpoint")
				}
			}
		}
	}
	if writeJSON(filepath.Join(dir, lifecycleSourceNames[0]), r) != nil {
		t.Fatal("report")
	}
	raw, e := os.ReadFile(filepath.Join(dir, lifecycleSourceNames[0]))
	if e != nil {
		t.Fatal(e)
	}
	a.InventorySHA256 = digestRaw(raw)
	hashes[lifecycleSourceNames[0]] = a.InventorySHA256
	return dir, a, hashes
}
func TestOriginalInventoryMaterialHandoffIncludesAssetsBothPassesAndEmptySources(t *testing.T) {
	pageSize := uint64(productionLimits().PageSize)
	for _, records := range []uint64{0, 1, pageSize, pageSize + 1} {
		t.Run(fmt.Sprint(records), func(t *testing.T) {
			dir, a, hashes := originalInventoryMaterialFixture(t, records)
			d, e := openLifecycleInventoryMaterialFiles(context.Background(), dir, uint32(os.Getuid()), a, hashes)
			if e != nil {
				t.Fatal(e)
			}
			defer d.close()
			pages := records/pageSize + 1
			if records == 0 {
				pages = 0
			}
			if len(d.files) != 11+int(8*pages) || len(d.children) != 0 {
				t.Fatal("original members were omitted")
			}
			for name, v := range d.files {
				if v.file == nil || d.checkFile(v) != nil || v.retained {
					t.Fatalf("missing original FD: %s", name)
				}
			}
			if d.purge(context.Background()) != nil || d.checkComplete(true) != nil || d.close() != nil {
				t.Fatal("exact file purge/close")
			}
			for _, v := range d.files {
				if v.file != nil {
					t.Fatal("unlinked original FD remains")
				}
			}
		})
	}
}

// These are offline original files, not a native Archive or acceptance proof.
func originalRootStagingMaterialFixture(t *testing.T, windowTool bool) (string, string, lifecycleRequest, map[string]string) {
	t.Helper()
	inventory, approval, hashes := originalInventoryMaterialFixture(t, 0)
	base, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(base, "123-1-789-1")
	invocation := filepath.Join(base, "invocation")
	if os.Mkdir(root, 0700) != nil || os.Mkdir(invocation, 0700) != nil {
		t.Fatal("directories")
	}
	for _, dir := range []string{root, invocation} {
		if os.Chmod(dir, 0700) != nil {
			t.Fatal("directory")
		}
	}
	originalRoot := filepath.Join("/opt/backups/qs-server/compatibility-retirement", approval.OperationID)
	manifest := []byte("offline frozen manifest")
	r := lifecycleRequest{FormatVersion: 1, Kind: "compatibility_retirement_lifecycle_request", OriginalSourceSHA: approval.SourceSHA, ToolSourceSHA: strings.Repeat("d", 40), OperationID: approval.OperationID, ActualRunID: "789-1", ManifestSHA256: digestRaw(manifest), Approval: approval, ArchiveDirectory: filepath.Join(originalRoot, "archive"), SourceDirectory: filepath.Join(originalRoot, "inventory-"+approval.RunID), SourceFileSHA256: hashes,
		RestoreEngines: &lifecycleRestoreEngines{MySQLImageID: "sha256:" + strings.Repeat("6", 64), MongoImageID: "sha256:" + strings.Repeat("7", 64), Architecture: runtime.GOARCH},
		Recovery:       backup.TargetRecoveryRequest{SourceSHA: approval.SourceSHA, OperationID: approval.OperationID, OriginalRunID: approval.RunID, ActualRunID: "789-1"}}
	if writeJSON(filepath.Join(root, "lifecycle-request.json"), r) != nil {
		t.Fatal("request")
	}
	request, _ := os.ReadFile(filepath.Join(root, "lifecycle-request.json"))
	if os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0600) != nil {
		t.Fatal("manifest")
	}
	if os.WriteFile(filepath.Join(root, "restore-native"), []byte("offline native bytes"), 0700) != nil {
		t.Fatal("native")
	}
	child := filepath.Join(root, "inventory-"+approval.RunID)
	if os.Mkdir(child, 0700) != nil {
		t.Fatal("child")
	}
	for _, name := range lifecycleSourceNames {
		raw, _ := os.ReadFile(filepath.Join(inventory, name))
		if os.WriteFile(filepath.Join(child, name), raw, 0600) != nil {
			t.Fatal("copy")
		}
	}
	i := lifecycleSourceCopyIntent{1, "root_once_exact_source_copy_intent", r.OriginalSourceSHA, r.ToolSourceSHA, r.OperationID, r.ActualRunID, approval.RunID, digestRaw(request), r.ManifestSHA256, hashes, targets, uint32(os.Getuid()), r.ArchiveDirectory, child, false, true, "", ""}
	if writeJSON(filepath.Join(root, "source-copy.intent.private.json"), i) != nil {
		t.Fatal("intent")
	}
	if windowTool {
		v := lifecycleAPIInvocationIntent{FormatVersion: 1, Kind: "independent_window_tool_native_invocation", DispatcherSourceSHA: strings.Repeat("e", 40), ToolSourceSHA: i.ToolSourceSHA, OriginalSourceSHA: i.OriginalSourceSHA, OperationID: i.OperationID, OriginalRunID: i.OriginalRunID, ActualRunID: i.ActualRunID, Stage: "prepare", TemplateSHA256: strings.Repeat("f", 64), DerivedSHA256: i.RequestSHA256, ManifestSHA256: i.ManifestSHA256, PackageSHA256: strings.Repeat("1", 64), ToolProgramSHA256: strings.Repeat("2", 64), NativeSHA256: digestRaw([]byte("offline native bytes")), NativePath: filepath.Join(root, "restore-native"), SourceUID: i.SourceUID}
		if writeJSON(filepath.Join(root, "tool.intent.private.json"), v) != nil {
			t.Fatal("tool")
		}
		for _, pair := range [][2]string{{"tool.intent.private.json", "native-call.intent.private.json"}, {"lifecycle-request.json", "lifecycle-request.json"}, {"manifest.json", "manifest.json"}} {
			raw, _ := os.ReadFile(filepath.Join(root, pair[0]))
			if os.WriteFile(filepath.Join(invocation, pair[1]), raw, 0600) != nil {
				t.Fatal("companion")
			}
		}
	} else {
		v := lifecycleOriginalRootToolIntent{1, "approved_root_once_tool_staging", "lifecycle", i.OperationID, i.ActualRunID, i.ToolSourceSHA, filepath.Join(originalRoot, "lifecycle-request.json"), i.RequestSHA256, i.ManifestSHA256, strings.Repeat("3", 64), digestRaw([]byte("offline native bytes")), i.SourceUID, false, true}
		if writeJSON(filepath.Join(root, "tool.intent.private.json"), v) != nil {
			t.Fatal("tool")
		}
	}
	raw, _ := os.ReadFile(filepath.Join(root, "source-copy.intent.private.json"))
	r.SourceCopyIntent = &lifecycleFinalFileBinding{Path: filepath.Join(root, "source-copy.intent.private.json"), SHA256: digestRaw(raw)}
	z := lifecyclePreparationRestoreZero{FormatVersion: 1, Kind: "original_preparation_isolated_restore_zero", OriginalSourceSHA: i.OriginalSourceSHA, ToolSourceSHA: i.ToolSourceSHA, OperationID: i.OperationID, OriginalRunID: i.OriginalRunID, ActualRunID: i.ActualRunID, ManifestSHA256: i.ManifestSHA256, ArchiveSHA256: strings.Repeat("8", 64), RequestSHA256: i.RequestSHA256, ElapsedMillis: 1000}
	for n, kind := range []string{"mysql", "mongodb"} {
		owner := strings.Repeat(fmt.Sprint(n+1), 32)
		engine := lifecyclePreparationRestoreEngine{Kind: kind, Owner: owner, ContainerID: strings.Repeat(fmt.Sprint(n+3), 64), ImageID: []string{r.RestoreEngines.MySQLImageID, r.RestoreEngines.MongoImageID}[n], Namespace: "qs_retirement_restore_" + digestRaw([]byte(i.OriginalSourceSHA + "\n" + i.OperationID + "\n" + i.ActualRunID + "\n" + i.ManifestSHA256))[:24], Volumes: []string{"qs-retirement-data-" + owner}}
		if n == 1 {
			engine.Volumes = append(engine.Volumes, "qs-retirement-config-"+owner)
		}
		z.Engines = append(z.Engines, engine)
		labels := map[string]string{"codex.task": "qs-compatibility-retirement", "codex.owner": owner, "qs.retirement.operation": i.OperationID, "qs.retirement.run": i.ActualRunID}
		intent := lifecyclePreparationRestoreIntent{FormatVersion: 1, Kind: "temporary_network_none_restore_intent", OriginalSourceSHA: i.OriginalSourceSHA, ToolSourceSHA: i.ToolSourceSHA, OperationID: i.OperationID, ActualRunID: i.ActualRunID, ManifestSHA256: i.ManifestSHA256, ArchiveSHA256: z.ArchiveSHA256, Namespace: engine.Namespace, Owner: owner, ContainerName: "qs-retirement-restore-" + owner, ImageID: engine.ImageID, Architecture: runtime.GOARCH, Labels: labels, ContainerLabels: labels, Volumes: engine.Volumes, ToolSHA256: digestRaw([]byte("offline native bytes")), Network: "none", PurgeRequired: true}
		created := lifecyclePreparationRestoreCreated{ContainerID: engine.ContainerID, Owner: owner, OperationID: i.OperationID, ActualRunID: i.ActualRunID}
		for _, v := range []struct {
			name  string
			value any
		}{{"restore-" + owner + ".intent.private.json", intent}, {"restore-" + owner + ".created.private.json", created}} {
			if writeJSON(filepath.Join(root, v.name), v.value) != nil {
				t.Fatal("restore metadata")
			}
			raw, _ := os.ReadFile(filepath.Join(root, v.name))
			info, _ := os.Stat(filepath.Join(root, v.name))
			st, _ := infoStat(info)
			z.Files = append(z.Files, lifecyclePreparationRestoreMaterial{v.name, digestRaw(raw), info.Size(), st.Uid, st.Gid, uint64(st.Dev), uint64(st.Ino), uint32(info.Mode().Perm())})
		}
	}
	registration := lifecyclePreparationRestoreRegistration{FormatVersion: 1, Kind: "temporary_isolated_restore_registration", OriginalSourceSHA: i.OriginalSourceSHA, ToolSourceSHA: i.ToolSourceSHA, OperationID: i.OperationID, OriginalRunID: i.OriginalRunID, ActualRunID: i.ActualRunID, ManifestSHA256: i.ManifestSHA256, ArchiveSHA256: z.ArchiveSHA256, Namespace: z.Engines[0].Namespace, MySQLOriginalUUID: strings.Repeat("a", 64), MySQLRestoreUUID: strings.Repeat("b", 64), MongoOriginalProcess: strings.Repeat("c", 64), MongoRestoreProcess: strings.Repeat("d", 64), PurgeRequired: true}
	name := "lifecycle-restore-" + i.ActualRunID + ".registration.private.json"
	if writeJSON(filepath.Join(root, name), registration) != nil {
		t.Fatal("restore registration")
	}
	raw, _ = os.ReadFile(filepath.Join(root, name))
	info, _ := os.Stat(filepath.Join(root, name))
	st, _ := infoStat(info)
	z.Files = append(z.Files, lifecyclePreparationRestoreMaterial{name, digestRaw(raw), info.Size(), st.Uid, st.Gid, uint64(st.Dev), uint64(st.Ino), uint32(info.Mode().Perm())})
	name = "lifecycle-restore-" + i.ActualRunID + ".zero.private.json"
	if writeJSON(filepath.Join(root, name), z) != nil {
		t.Fatal("zero metadata")
	}
	raw, _ = os.ReadFile(filepath.Join(root, name))
	r.PreparationRestoreZero = &lifecycleFinalFileBinding{Path: filepath.Join(root, name), SHA256: digestRaw(raw)}
	r.ActualRunID, r.ToolSourceSHA = "999-1", strings.Repeat("9", 40)
	r.Recovery.ArchiveSHA256 = strings.Repeat("8", 64)
	return root, invocation, r, hashes
}

func TestOriginalRootStagingMaterialHandoffReopensBothActualProducerSchemas(t *testing.T) {
	for _, window := range []bool{false, true} {
		t.Run(fmt.Sprint(window), func(t *testing.T) {
			root, invocation, r, hashes := originalRootStagingMaterialFixture(t, window)
			d, peer, err := openLifecycleRootStagingMaterialFiles(context.Background(), root, invocation, r, hashes, uint32(os.Getuid()), uint32(os.Getuid()))
			if err != nil {
				t.Fatal(err)
			}
			defer d.close()
			if len(d.files) != 11 || len(d.children) != 1 || len(d.children["inventory-"+r.Approval.RunID].files) != 7 || window && (peer == nil || len(peer.files) != 3) || !window && peer != nil {
				t.Fatal("producer members incomplete")
			}
			if peer != nil {
				defer peer.close()
				if err = peer.purge(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err = peer.checkComplete(true); err != nil {
					t.Fatal(err)
				}
			}
			if err = d.purge(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err = d.checkComplete(true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOriginalRootStagingMaterialHandoffRejectsRebindingAndExtraFiles(t *testing.T) {
	for _, name := range []string{"wrong_intent_hash", "wrong_source_uid", "wrong_archive", "missing_copy", "changed_native", "extra_root", "extra_child", "extra_companion", "changed_companion", "unknown_tool_schema", "null_intent"} {
		t.Run(name, func(t *testing.T) {
			root, invocation, r, hashes := originalRootStagingMaterialFixture(t, true)
			var i lifecycleSourceCopyIntent
			intent, _ := os.ReadFile(r.SourceCopyIntent.Path)
			_ = json.Unmarshal(intent, &i)
			switch name {
			case "wrong_intent_hash":
				r.SourceCopyIntent.SHA256 = strings.Repeat("0", 64)
			case "wrong_source_uid":
				i.SourceUID++
				raw, _ := json.Marshal(i)
				_ = os.WriteFile(r.SourceCopyIntent.Path, raw, 0600)
				r.SourceCopyIntent.SHA256 = digestRaw(raw)
			case "wrong_archive":
				i.ArchiveDirectory += "-other"
				raw, _ := json.Marshal(i)
				_ = os.WriteFile(r.SourceCopyIntent.Path, raw, 0600)
				r.SourceCopyIntent.SHA256 = digestRaw(raw)
			case "missing_copy":
				_ = os.Remove(filepath.Join(i.SourceStagingDirectory, lifecycleSourceNames[3]))
			case "changed_native":
				_ = os.WriteFile(filepath.Join(root, "restore-native"), []byte("other"), 0700)
			case "extra_root":
				_ = os.WriteFile(filepath.Join(root, "extra"), []byte("body"), 0600)
			case "extra_child":
				_ = os.WriteFile(filepath.Join(i.SourceStagingDirectory, "extra"), []byte("body"), 0600)
			case "extra_companion":
				_ = os.WriteFile(filepath.Join(invocation, "extra"), []byte("body"), 0600)
			case "changed_companion":
				_ = os.WriteFile(filepath.Join(invocation, "manifest.json"), []byte("other"), 0600)
			case "unknown_tool_schema":
				_ = os.WriteFile(filepath.Join(root, "tool.intent.private.json"), []byte(`{"kind":"other","complete":true}`), 0600)
			case "null_intent":
				raw := bytes.Replace(intent, []byte(`"drop_authority": false`), []byte(`"drop_authority": null`), 1)
				if bytes.Equal(raw, intent) {
					raw = bytes.Replace(intent, []byte(`"drop_authority":false`), []byte(`"drop_authority":null`), 1)
				}
				_ = os.WriteFile(r.SourceCopyIntent.Path, raw, 0600)
				r.SourceCopyIntent.SHA256 = digestRaw(raw)
			}
			d, peer, err := openLifecycleRootStagingMaterialFiles(context.Background(), root, invocation, r, hashes, uint32(os.Getuid()), uint32(os.Getuid()))
			if err == nil || d != nil || peer != nil {
				t.Fatal("unbound original material accepted")
			}
			if _, err = os.Stat(filepath.Join(root, "lifecycle-request.json")); err != nil {
				t.Fatal("rejection purged original")
			}
		})
	}
}

func TestOriginalPreparationRestoreZeroRejectsUnboundAndChangedProducerMetadata(t *testing.T) {
	// Actual filesystem/source binding only. No fixture can prove Docker zero or
	// mint the original producer's same-process native release owner.
	for _, mutation := range []string{"missing_reference", "wrong_receipt_hash", "missing_zero", "wrong_source", "wrong_run", "wrong_request", "late_budget", "reused_owner", "same_cid", "missing_metadata", "replacement_inode", "wrong_member_hash", "wrong_original_uid", "metadata_wrong_operation", "unknown_receipt_field", "null_member_inode"} {
		t.Run(mutation, func(t *testing.T) {
			root, invocation, r, hashes := originalRootStagingMaterialFixture(t, true)
			raw, _ := os.ReadFile(r.PreparationRestoreZero.Path)
			var z lifecyclePreparationRestoreZero
			_ = json.Unmarshal(raw, &z)
			switch mutation {
			case "missing_reference":
				r.PreparationRestoreZero = nil
			case "wrong_receipt_hash":
				r.PreparationRestoreZero.SHA256 = strings.Repeat("0", 64)
			case "missing_zero":
				_ = os.Remove(r.PreparationRestoreZero.Path)
			case "wrong_source":
				z.ToolSourceSHA = strings.Repeat("f", 40)
			case "wrong_run":
				z.ActualRunID = r.ActualRunID
			case "wrong_request":
				z.RequestSHA256 = strings.Repeat("f", 64)
			case "late_budget":
				z.ElapsedMillis = 600001
			case "reused_owner":
				z.Engines[1].Owner = z.Engines[0].Owner
			case "same_cid":
				z.Engines[1].ContainerID = z.Engines[0].ContainerID
			case "missing_metadata":
				_ = os.Remove(filepath.Join(root, z.Files[0].Name))
			case "replacement_inode":
				path := filepath.Join(root, z.Files[0].Name)
				body, _ := os.ReadFile(path)
				_ = os.Rename(path, path+"-old")
				_ = os.WriteFile(path, body, 0600)
				_ = os.Remove(path + "-old")
			case "wrong_member_hash":
				z.Files[0].SHA256 = strings.Repeat("f", 64)
			case "wrong_original_uid":
				z.Files[0].UID++
			case "metadata_wrong_operation":
				path := filepath.Join(root, z.Files[0].Name)
				body, _ := os.ReadFile(path)
				var intent lifecyclePreparationRestoreIntent
				_ = json.Unmarshal(body, &intent)
				intent.OperationID = "other-1"
				body, _ = json.Marshal(intent)
				_ = os.WriteFile(path, body, 0600)
				info, _ := os.Stat(path)
				st, _ := infoStat(info)
				z.Files[0] = lifecyclePreparationRestoreMaterial{z.Files[0].Name, digestRaw(body), info.Size(), st.Uid, st.Gid, uint64(st.Dev), uint64(st.Ino), uint32(info.Mode().Perm())}
			}
			if mutation != "missing_reference" && mutation != "wrong_receipt_hash" && mutation != "missing_zero" {
				raw, _ = json.Marshal(z)
				if mutation == "unknown_receipt_field" {
					raw = append(raw[:len(raw)-1], []byte(`,"complete":true}`)...)
				}
				if mutation == "null_member_inode" {
					raw = bytes.Replace(raw, []byte(fmt.Sprintf(`"inode":%d`, z.Files[0].Inode)), []byte(`"inode":null`), 1)
				}
				_ = os.WriteFile(r.PreparationRestoreZero.Path, raw, 0600)
				r.PreparationRestoreZero.SHA256 = digestRaw(raw)
			}
			d, peer, err := openLifecycleRootStagingMaterialFiles(context.Background(), root, invocation, r, hashes, uint32(os.Getuid()), uint32(os.Getuid()))
			if err == nil || d != nil || peer != nil {
				t.Fatal("unbound preparation metadata admitted")
			}
			if _, err = os.Stat(filepath.Join(root, "lifecycle-request.json")); err != nil {
				t.Fatal("rejection changed source files")
			}
		})
	}
}

func TestPreparationRestoreMetadataRequiresActualFiveProducerHashes(t *testing.T) {
	for _, mutation := range []string{"none", "missing_original_producer", "missing_member", "extra_member", "changed_original_hash"} {
		t.Run(mutation, func(t *testing.T) {
			root, _, r, _ := originalRootStagingMaterialFixture(t, false)
			raw, _ := os.ReadFile(r.PreparationRestoreZero.Path)
			var z lifecyclePreparationRestoreZero
			_ = json.Unmarshal(raw, &z)
			originalHashes := map[string]string{}
			for _, m := range z.Files {
				originalHashes[m.Name] = m.SHA256
			}
			first := z.Files[0].Name
			z.Files = nil // Native path requires the original write-time producer map.
			switch mutation {
			case "missing_original_producer":
				originalHashes = nil
			case "missing_member":
				delete(originalHashes, first)
			case "extra_member":
				originalHashes["foreign"] = strings.Repeat("f", 64)
			case "changed_original_hash":
				originalHashes[first] = strings.Repeat("f", 64)
			}
			d, err := openLifecycleMaterialDirectory(root, uint32(os.Getuid()))
			if err != nil {
				t.Fatal(err)
			}
			defer d.close()
			files, err := registerLifecyclePreparationRestoreMetadata(context.Background(), d, &z, digestRaw([]byte("offline native bytes")), runtime.GOARCH, uint32(os.Getuid()), originalHashes)
			if mutation == "none" {
				if err != nil || len(files) != 5 {
					t.Fatal("original producer hashes rejected", err)
				}
			} else if err == nil || files != nil {
				t.Fatal("missing original producer accepted")
			}
		})
	}
}

func originalHistoricalWriteMaterialFixture(t *testing.T) (string, lifecycleRequest, lifecycleHistoricalMaterialManifest) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(base, 0700)
	path := filepath.Join(base, "history-write-789-1")
	if os.Mkdir(path, 0700) != nil {
		t.Fatal("directory")
	}
	m := lifecycleHistoricalMaterialManifest{Version: 1, SourceSHA: strings.Repeat("a", 40), ToolSourceSHA: strings.Repeat("b", 40), OperationID: "123-1", RunID: "789-1", MaxSpoolBytes: 16 << 30, JournalSequence: 2}
	for _, name := range []string{"prepared-mongo-private.bin", "prepared-sql-private.bin", "journal-1.json", "journal-2.json"} {
		raw := []byte("private-source-" + name)
		if name == "prepared-sql-private.bin" {
			raw = nil
		}
		if os.WriteFile(filepath.Join(path, name), raw, 0600) != nil {
			t.Fatal("source")
		}
		info, _ := os.Stat(filepath.Join(path, name))
		st, _ := infoStat(info)
		m.Files = append(m.Files, lifecycleHistoricalMaterial{name, digestRaw(raw), info.Size(), st.Uid, st.Gid, uint64(st.Dev), uint64(st.Ino), uint32(info.Mode().Perm())})
	}
	if writeJSON(filepath.Join(path, "history.materials.private.json"), m) != nil {
		t.Fatal("manifest")
	}
	raw, _ := os.ReadFile(filepath.Join(path, "history.materials.private.json"))
	report := lifecycleHistoricalWriteMaterialReport{Protocol: "qs-compatibility-evidence-write/v1", SourceSHA: m.SourceSHA, ToolSourceSHA: m.ToolSourceSHA, OperationID: m.OperationID, ActualRunID: m.RunID, RequestSHA256: strings.Repeat("c", 64), DescriptorSHA256: strings.Repeat("d", 64), CommitState: "sql_committed_mongo_not_required", MongoCommitRequirement: "not_required", AIOriginalCommands: 2, AISourceReferences: 3, ActualSQLCommitResponse: true, AICommandPersistenceComplete: true, EvidenceWriteFinished: true, ErrorCategory: "none", MaterialManifestSHA256: digestRaw(raw)}
	if writeJSON(filepath.Join(path, "history.write.json"), report) != nil {
		t.Fatal("report")
	}
	raw, _ = os.ReadFile(filepath.Join(path, "history.write.json"))
	r := lifecycleRequest{OriginalSourceSHA: m.SourceSHA, OperationID: m.OperationID, ActualRunID: "999-1", HistoricalWriteReport: &lifecycleFinalFileBinding{Path: filepath.Join(path, "history.write.json"), SHA256: digestRaw(raw)}}
	return path, r, m
}

func TestOriginalHistoricalWriteMaterialHandoffUsesActualSourceIdentityAndEOF(t *testing.T) {
	path, r, _ := originalHistoricalWriteMaterialFixture(t)
	d, err := openLifecycleHistoricalWriteMaterialFiles(context.Background(), path, r, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	if len(d.files) != 6 {
		t.Fatal("original report/manifest/spools/sequence incomplete")
	}
	for _, v := range d.files {
		if v.file == nil || v.info == nil {
			t.Fatal("no actual RO source FD")
		}
	}
	if err = d.purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = d.checkComplete(true); err != nil {
		t.Fatal(err)
	}
}

func TestOriginalHistoricalWriteMaterialHandoffRejectsUnknownSourceOrChangedMembers(t *testing.T) {
	for _, mutation := range []string{"wrong_report_hash", "wrong_source", "unknown_commit", "missing", "replace_inode", "extra", "wrong_owner", "wrong_budget", "noncontinuous", "wrong_member_hash", "null_member_bytes", "descriptor_permission"} {
		t.Run(mutation, func(t *testing.T) {
			path, r, m := originalHistoricalWriteMaterialFixture(t)
			reportRaw, _ := os.ReadFile(r.HistoricalWriteReport.Path)
			var report lifecycleHistoricalWriteMaterialReport
			_ = json.Unmarshal(reportRaw, &report)
			switch mutation {
			case "wrong_report_hash":
				r.HistoricalWriteReport.SHA256 = strings.Repeat("0", 64)
			case "wrong_source":
				report.SourceSHA = strings.Repeat("e", 40)
			case "unknown_commit":
				report.CommitState = "sql_committed_mongo_unknown"
			case "missing":
				_ = os.Remove(filepath.Join(path, "journal-2.json"))
			case "replace_inode":
				p := filepath.Join(path, m.Files[0].Name)
				body, _ := os.ReadFile(p)
				_ = os.Rename(p, p+"-old")
				_ = os.WriteFile(p, body, 0600)
				_ = os.Remove(p + "-old")
			case "extra":
				_ = os.WriteFile(filepath.Join(path, "unknown-body"), []byte("body"), 0600)
			case "wrong_owner":
				m.Files[0].UID++
			case "wrong_budget":
				m.MaxSpoolBytes = 32 << 30
			case "noncontinuous":
				m.Files[3].Name = "journal-3.json"
			case "wrong_member_hash":
				m.Files[0].SHA256 = strings.Repeat("0", 64)
			}
			if mutation == "wrong_owner" || mutation == "wrong_budget" || mutation == "noncontinuous" || mutation == "wrong_member_hash" || mutation == "null_member_bytes" || mutation == "descriptor_permission" {
				raw, _ := json.Marshal(m)
				if mutation == "null_member_bytes" {
					raw = bytes.Replace(raw, []byte(`"bytes":0`), []byte(`"bytes":null`), 1)
				}
				if mutation == "descriptor_permission" {
					raw = append(raw[:len(raw)-1], []byte(`,"permission":true}`)...)
				}
				_ = os.WriteFile(filepath.Join(path, "history.materials.private.json"), raw, 0600)
				report.MaterialManifestSHA256 = digestRaw(raw)
			}
			if mutation != "wrong_report_hash" {
				raw, _ := json.Marshal(report)
				_ = os.WriteFile(r.HistoricalWriteReport.Path, raw, 0600)
				r.HistoricalWriteReport.SHA256 = digestRaw(raw)
			}
			d, err := openLifecycleHistoricalWriteMaterialFiles(context.Background(), path, r, uint32(os.Getuid()))
			if err == nil || d != nil {
				t.Fatal("unbound source admitted")
			}
			if _, err = os.Stat(r.HistoricalWriteReport.Path); err != nil {
				t.Fatal("rejection changed original report")
			}
		})
	}
}

func TestHistoricalSpoolRegistrationKeepsOrdinaryBoundAndRejectsOtherNames(t *testing.T) {
	path, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(path, 0700)
	d, e := openLifecycleMaterialDirectory(path, uint32(os.Getuid()))
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	p := filepath.Join(path, "prepared-mongo-private.bin")
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if f.Truncate((2<<30)+1) != nil || f.Close() != nil {
		t.Fatal("sparse source")
	}
	if d.register("prepared-mongo-private.bin", strings.Repeat("0", 64), uint32(os.Getuid()), 0600) == nil {
		t.Fatal("ordinary limit widened")
	}
	if d.registerHistoricalCASSpool("ordinary", strings.Repeat("0", 64), uint32(os.Getuid())) == nil {
		t.Fatal("unrelated large source admitted")
	}
	if os.Truncate(p, (16<<30)+1) != nil {
		t.Fatal("sparse bound")
	}
	if d.registerHistoricalCASSpool("prepared-mongo-private.bin", strings.Repeat("0", 64), uint32(os.Getuid())) == nil {
		t.Fatal("original spool budget widened")
	}
}
func TestOriginalInventoryMaterialHandoffRejectsUnboundIncompleteOrChangedProducerFiles(t *testing.T) {
	for _, kind := range []string{"missing-asset", "missing-second-pass", "wrong-source", "wrong-request", "wrong-final-prefix", "different-pass-cursor", "extra-member", "wrong-owner", "changed-after-open"} {
		t.Run(kind, func(t *testing.T) {
			dir, a, hashes := originalInventoryMaterialFixture(t, 1)
			assetPath := filepath.Join(dir, lifecycleSourceNames[3]+".asset.json")
			checkpointPath := filepath.Join(dir, "mysql-domain_event_outbox-pass-2-page-000001.checkpoint.json")
			switch kind {
			case "missing-asset":
				if os.Remove(assetPath) != nil {
					t.Fatal("remove")
				}
			case "missing-second-pass":
				if os.Remove(checkpointPath) != nil {
					t.Fatal("remove")
				}
			case "wrong-source", "wrong-request":
				var v lifecycleInventorySourceAsset
				raw, _ := os.ReadFile(assetPath)
				if json.Unmarshal(raw, &v) != nil {
					t.Fatal("read")
				}
				if kind == "wrong-source" {
					v.SourceSHA = strings.Repeat("d", 40)
				} else {
					v.RequestHash = strings.Repeat("d", 64)
				}
				raw, _ = json.Marshal(v)
				if os.WriteFile(assetPath, raw, 0600) != nil {
					t.Fatal("write")
				}
			case "wrong-final-prefix", "different-pass-cursor":
				var v lifecycleInventoryCheckpoint
				raw, _ := os.ReadFile(checkpointPath)
				if json.Unmarshal(raw, &v) != nil {
					t.Fatal("read")
				}
				if kind == "wrong-final-prefix" {
					v.PrefixHash = strings.Repeat("d", 64)
				} else {
					v.Cursor = "other"
				}
				raw, _ = json.Marshal(v)
				if os.WriteFile(checkpointPath, raw, 0600) != nil {
					t.Fatal("write")
				}
			case "extra-member":
				if os.WriteFile(filepath.Join(dir, "unknown"), []byte("body"), 0600) != nil {
					t.Fatal("write")
				}
			}
			uid := uint32(os.Getuid())
			if kind == "wrong-owner" {
				uid++
			}
			d, e := openLifecycleInventoryMaterialFiles(context.Background(), dir, uid, a, hashes)
			if kind == "changed-after-open" {
				if e != nil {
					t.Fatal(e)
				}
				defer d.close()
				if os.WriteFile(assetPath, []byte("changed"), 0600) != nil || d.checkComplete(false) == nil {
					t.Fatal("original identity/bytes change accepted")
				}
			} else if e == nil {
				defer d.close()
				t.Fatal("foreign/incomplete producer material accepted")
			}
			if _, e := os.Stat(filepath.Join(dir, lifecycleSourceNames[3])); e != nil {
				t.Fatal("failed registration deleted a source")
			}
		})
	}
}

// Only real temporary fixture FDs and original producer wire formats are used.
// The intentionally partial operation parent cannot authorize final acceptance.
func historicalWriteInputFixture(t *testing.T, newerAITool bool) (string, lifecycleRequest, *lifecycleMaterialDirectory, *lifecycleMaterialDirectory) {
	t.Helper()
	writerPath, r, _ := originalHistoricalWriteMaterialFixture(t)
	root := filepath.Dir(writerPath)
	inventoryPath, a, hashes := originalInventoryMaterialFixture(t, 0)
	requestRaw := []byte("original readonly inventory request")
	a.RequestHash = digestRaw(requestRaw)
	put := func(path string, value any) string {
		raw, e := json.Marshal(value)
		if e != nil || os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("fixture producer JSON")
		}
		return digestRaw(raw)
	}
	var inv report
	raw, _ := os.ReadFile(filepath.Join(inventoryPath, "inventory.private.json"))
	_ = json.Unmarshal(raw, &inv)
	inv.RequestHash = a.RequestHash
	for _, name := range lifecycleSourceNames[3:] {
		var asset lifecycleInventorySourceAsset
		raw, _ = os.ReadFile(filepath.Join(inventoryPath, name+".asset.json"))
		_ = json.Unmarshal(raw, &asset)
		asset.RequestHash = a.RequestHash
		put(filepath.Join(inventoryPath, name+".asset.json"), asset)
	}
	a.InventorySHA256 = put(filepath.Join(inventoryPath, "inventory.private.json"), inv)
	hashes[lifecycleSourceNames[0]] = a.InventorySHA256
	actualInventory := filepath.Join(root, "inventory-"+a.RunID)
	if os.Rename(inventoryPath, actualInventory) != nil || os.WriteFile(filepath.Join(root, "inventory-request.json"), requestRaw, 0600) != nil {
		t.Fatal("fixture original inventory")
	}
	r.Approval = a
	inventory, e := openLifecycleInventoryMaterialFiles(context.Background(), actualInventory, uint32(os.Getuid()), a, hashes)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = inventory.close() })
	parent := lifecycleHistoricalRequestMaterial{FormatVersion: 1, Kind: "readonly_compatibility_history_request", SourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID, RunID: a.RunID,
		InventoryRequest: lifecycleFinalFileBinding{filepath.Join(root, "inventory-request.json"), a.RequestHash}, InventoryReport: lifecycleFinalFileBinding{filepath.Join(actualInventory, "inventory.private.json"), a.InventorySHA256}}
	for n, target := range targets {
		f := inventory.files[lifecycleSourceNames[3+n]]
		parent.Assets = append(parent.Assets, lifecycleHistoricalRequestAsset{target[0], target[1], filepath.Join(actualInventory, lifecycleSourceNames[3+n]), f.hash, uint64(f.info.Size())})
	}
	parentHash := put(filepath.Join(root, "history-request.json"), parent)
	writeRun, boundsRun := "789-1", "678-1"
	registrationPath := filepath.Join(root, "history-write-registration-"+writeRun)
	boundsPath := filepath.Join(root, "ai-host-bounds-"+boundsRun)
	bootstrapPath := filepath.Join(root, "ai-bootstrap-bounds-"+boundsRun)
	for _, path := range []string{registrationPath, boundsPath, bootstrapPath} {
		if os.Mkdir(path, 0700) != nil {
			t.Fatal("fixture original producer directory")
		}
	}
	request := parent
	request.RunID = writeRun
	writeRequestHash := put(filepath.Join(registrationPath, "history.request.json"), request)
	request.RunID = boundsRun
	aiRequestHash := put(filepath.Join(bootstrapPath, "history.request.json"), request)
	protectionHash := put(filepath.Join(root, "ai-message-protection.json"), map[string]string{"original": "protection"})
	aiBoundsHash := put(filepath.Join(boundsPath, "ai.bounds.json"), map[string]string{"original": "ai bounds"})
	peerBoundsHash := put(filepath.Join(boundsPath, "peer.bounds.json"), map[string]string{"original": "peer bounds"})
	input := lifecycleHistoricalInputMaterial{FormatVersion: 1, Kind: "historical_evidence_write_host_input", SourceSHA: r.OriginalSourceSHA, ToolSourceSHA: strings.Repeat("b", 40), OperationID: r.OperationID, ActualRunID: writeRun, Mode: "write", RequestSHA256: writeRequestHash,
		RuntimeSourceSHA: strings.Repeat("c", 40), ImageID: "sha256:" + strings.Repeat("d", 64), ContainerID: strings.Repeat("e", 64), RuntimeBindingSHA256: strings.Repeat("f", 64), AssetsDirectory: "/actual/host/assets",
		AIBounds: &lifecycleFinalFileBinding{filepath.Join(boundsPath, "ai.bounds.json"), aiBoundsHash}, PeerBounds: &lifecycleFinalFileBinding{filepath.Join(boundsPath, "peer.bounds.json"), peerBoundsHash}, Protection: &lifecycleFinalFileBinding{filepath.Join(root, "ai-message-protection.json"), protectionHash}}
	inputHash := put(filepath.Join(registrationPath, "write.input.json"), input)
	record := lifecycleHistoricalInputRegistration{1, "historical_evidence_write_registration", input.SourceSHA, input.ToolSourceSHA, r.OperationID, writeRun, strings.Repeat("1", 64), a.RunID, parentHash, writeRequestHash, inputHash, strings.Repeat("2", 64), "run_id"}
	put(filepath.Join(registrationPath, "write.registration.json"), record)
	aiInput := input
	aiInput.Kind, aiInput.Mode, aiInput.ActualRunID, aiInput.RequestSHA256 = "readonly_ai_external_host_input", "bounds", boundsRun, aiRequestHash
	aiInput.AIBounds, aiInput.PeerBounds, aiInput.Protection = nil, nil, nil
	aiInput.ToolSourceSHA = ""
	if newerAITool {
		aiInput.ToolSourceSHA = input.ToolSourceSHA
	}
	encoded, _ := json.Marshal(aiInput)
	var aiInputMap map[string]any
	_ = json.Unmarshal(encoded, &aiInputMap)
	if !newerAITool {
		delete(aiInputMap, "tool_source_sha")
	}
	aiInputHash := put(filepath.Join(bootstrapPath, "ai-host.input.json"), aiInputMap)
	ai := lifecycleHistoricalAIBoundsMaterial{Protocol: "qs-compatibility-ai-host-readonly/v1", Mode: "bounds", SourceSHA: r.OriginalSourceSHA, ToolSourceSHA: aiInput.ToolSourceSHA, OperationID: r.OperationID, ActualRunID: boundsRun, ExternalRunID: "678", RequestSHA256: aiRequestHash, DescriptorSHA256: aiInputHash,
		ExpectedRuntimeSHA256: input.RuntimeBindingSHA256, RuntimeSHA256: input.RuntimeBindingSHA256, ErrorCategory: "none", DiagnosticOnly: true, DiagnosticReadComplete: true, RequiredAdapters: []string{},
		Bounds: &retirement.AIExternalBoundsSummary{Scope: "diagnostic-unapproved-bounds-only", SourceSHA256: strings.Repeat("3", 64), RuntimeBindingSHA256: input.RuntimeBindingSHA256, AIBoundsSHA256: aiBoundsHash, PeerBoundsSHA256: peerBoundsHash, AIPhysicalObjects: 43, AILogicalObjects: 53, PeerObjects: 14, IndependentEpochs: 2, NextCycleRequired: true}}
	put(filepath.Join(boundsPath, "ai-host.readiness.json"), ai)
	aiRecord := lifecycleHistoricalAIBootstrapRegistration{FormatVersion: 1, Kind: "readonly_ai_host_derivation_registration", SourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID, ActualRunID: boundsRun, Mode: "bounds", ApprovalSHA256: strings.Repeat("4", 64), ParentRunID: a.RunID, ParentRequestSHA256: parentHash, DerivedRequestSHA256: aiRequestHash, DescriptorSHA256: aiInputHash, BinarySHA256: strings.Repeat("5", 64), OnlyParentFieldReplaced: "run_id"}
	if newerAITool {
		aiRecord.OriginalSourceSHA, aiRecord.ToolSourceSHA = r.OriginalSourceSHA, input.ToolSourceSHA
	}
	put(filepath.Join(bootstrapPath, "ai-host.registration.json"), aiRecord)
	var writerReport lifecycleHistoricalWriteMaterialReport
	raw, _ = os.ReadFile(r.HistoricalWriteReport.Path)
	_ = json.Unmarshal(raw, &writerReport)
	writerReport.RequestSHA256, writerReport.DescriptorSHA256 = writeRequestHash, inputHash
	r.HistoricalWriteReport.SHA256 = put(r.HistoricalWriteReport.Path, writerReport)
	writer, e := openLifecycleHistoricalWriteMaterialFiles(context.Background(), writerPath, r, uint32(os.Getuid()))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = writer.close() })
	return root, r, writer, inventory
}
func TestOriginalHistoricalInputHandoffUsesActualLinkedProducerFilesAndKeepsParentIncomplete(t *testing.T) {
	for _, newer := range []bool{false, true} {
		t.Run(fmt.Sprint(newer), func(t *testing.T) {
			root, r, writer, inventory := historicalWriteInputFixture(t, newer)
			registration, previous, e := openLifecycleHistoricalWriteInputFiles(context.Background(), r, writer, inventory, root, uint32(os.Getuid()))
			if e != nil {
				t.Fatal(e)
			}
			defer registration.close()
			defer func() {
				for _, d := range previous {
					_ = d.close()
				}
			}()
			if len(registration.files) != 3 || len(previous) != 3 || len(previous[0].files) != 3 || len(previous[1].files) != 3 || len(previous[2].files) != 3 {
				t.Fatal("original linked members omitted")
			}
			for _, d := range append([]*lifecycleMaterialDirectory{registration}, previous...) {
				for _, f := range d.files {
					if f.file == nil || d.checkFile(f) != nil || f.retained {
						t.Fatal("original FD not held")
					}
				}
			}
			if previous[2].checkComplete(false) == nil {
				t.Fatal("partial operation parent became complete")
			}
			if registration.checkComplete(false) != nil || previous[0].checkComplete(false) != nil || previous[1].checkComplete(false) != nil {
				t.Fatal("exact original leaves incomplete")
			}
			p := filepath.Join(registration.path, "write.input.json")
			raw, _ := os.ReadFile(p)
			if os.Rename(p, p+".prior") != nil || os.WriteFile(p, raw, 0600) != nil || os.Remove(p+".prior") != nil || registration.checkFile(registration.files["write.input.json"]) == nil {
				t.Fatal("replacement original inode admitted")
			}
		})
	}
}
func TestOriginalHistoricalInputHandoffRejectsUnknownOrUnboundPreviousMaterials(t *testing.T) {
	for _, mutation := range []string{"registration_extra", "bounds_extra", "bootstrap_extra", "missing_bounds", "wrong_parent", "wrong_asset", "wrong_runtime", "null_bounds_field", "unknown_readiness_authority", "wrong_bootstrap_source"} {
		t.Run(mutation, func(t *testing.T) {
			root, r, writer, inventory := historicalWriteInputFixture(t, false)
			bounds := filepath.Join(root, "ai-host-bounds-678-1")
			bootstrap := filepath.Join(root, "ai-bootstrap-bounds-678-1")
			switch mutation {
			case "registration_extra":
				_ = os.WriteFile(filepath.Join(root, "history-write-registration-789-1", "unknown"), nil, 0600)
			case "bounds_extra":
				_ = os.WriteFile(filepath.Join(bounds, "unknown"), nil, 0600)
			case "bootstrap_extra":
				_ = os.WriteFile(filepath.Join(bootstrap, "unknown"), nil, 0600)
			case "missing_bounds":
				_ = os.Remove(filepath.Join(bounds, "peer.bounds.json"))
			case "wrong_parent":
				_ = os.WriteFile(filepath.Join(root, "history-request.json"), []byte("changed"), 0600)
			case "wrong_asset":
				_ = os.WriteFile(filepath.Join(inventory.path, lifecycleSourceNames[6]), []byte("changed"), 0600)
			default:
				p := filepath.Join(bounds, "ai-host.readiness.json")
				if mutation == "wrong_bootstrap_source" {
					p = filepath.Join(bootstrap, "ai-host.registration.json")
				}
				raw, _ := os.ReadFile(p)
				var value map[string]any
				_ = json.Unmarshal(raw, &value)
				switch mutation {
				case "wrong_runtime":
					value["runtime_binding_sha256"] = strings.Repeat("0", 64)
				case "null_bounds_field":
					value["bounds"].(map[string]any)["recovery_authority"] = nil
				case "unknown_readiness_authority":
					value["production_success"] = true
				case "wrong_bootstrap_source":
					value["source_sha"] = strings.Repeat("0", 40)
				}
				raw, _ = json.Marshal(value)
				_ = os.WriteFile(p, raw, 0600)
			}
			registration, previous, e := openLifecycleHistoricalWriteInputFiles(context.Background(), r, writer, inventory, root, uint32(os.Getuid()))
			if e == nil || registration != nil || previous != nil {
				t.Fatal("unbound original producer admitted")
			}
		})
	}
}
func TestHistoricalProducerClosedNestedSchemasRejectNullEmptyBytesAndMissingInputField(t *testing.T) {
	q := lifecycleHistoricalRequestMaterial{FormatVersion: 1, Kind: "readonly_compatibility_history_request", InventoryRequest: lifecycleFinalFileBinding{"/a", "hash"}, InventoryReport: lifecycleFinalFileBinding{"/b", "hash"}, Assets: make([]lifecycleHistoricalRequestAsset, 4)}
	raw, _ := json.Marshal(q)
	raw = bytes.Replace(raw, []byte(`"full_file_bytes":0`), []byte(`"full_file_bytes":null`), 1)
	if decodeLifecycleHistoricalRequest(raw, &q) == nil {
		t.Fatal("null empty source bytes admitted")
	}
	v := lifecycleHistoricalInputMaterial{}
	raw, _ = json.Marshal(v)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "expected_ai_head") // 14 fields still remain when tool_source_sha is present.
	raw, _ = json.Marshal(fields)
	if decodeLifecycleHistoricalBoundsInput(raw, &v) == nil {
		t.Fatal("missing required input field replaced by optional tool field")
	}
}

func originalPreparationBudgetFixture(t *testing.T) (string, string, lifecycleRequest, map[string]string) {
	t.Helper()
	root, invocation, r, hashes := originalRootStagingMaterialFixture(t, true)
	var intent lifecycleSourceCopyIntent
	raw, _ := os.ReadFile(r.SourceCopyIntent.Path)
	if decodeLifecycleSourceCopyIntent(raw, &intent) != nil {
		t.Fatal("original intent")
	}
	basis := stop.Descriptor{Version: 1, SourceSHA: intent.OriginalSourceSHA, RuntimeSourceSHA: strings.Repeat("e", 40), ToolSourceSHA: intent.ToolSourceSHA, OriginalRunID: intent.OriginalRunID, OperationID: intent.OperationID, ManifestSHA256: intent.ManifestSHA256, HostRole: "server-a", MachineIDSHA256: strings.Repeat("c", 64), DockerPath: "/usr/bin/docker", DockerSHA256: strings.Repeat("d", 64), Containers: []stop.Container{{Component: "qs-apiserver"}, {Component: "qs-collection-server"}}}
	result := lifecyclePreparationBudgetResult{"qs_native_temporary_budget_key_result", intent.ToolSourceSHA, intent.OperationID, intent.ActualRunID, strings.Repeat("e", 64), true, false, false, "none"}
	for name, value := range map[string]any{"budget-key.basis.private.json": basis, "budget-key.result.private.json": result} {
		raw, _ := json.Marshal(value)
		if os.WriteFile(filepath.Join(root, name), raw, 0600) != nil {
			t.Fatal("original budget producer")
		}
		if strings.Contains(name, "basis") {
			intent.BudgetDescriptorSHA256 = digestRaw(raw)
		} else {
			intent.BudgetResultSHA256 = digestRaw(raw)
		}
	}
	raw, _ = json.Marshal(intent)
	if os.WriteFile(r.SourceCopyIntent.Path, raw, 0600) != nil {
		t.Fatal("actual producer intent")
	}
	r.SourceCopyIntent.SHA256 = digestRaw(raw)
	return root, invocation, r, hashes
}
func TestOriginalPreparationBudgetMaterialIsBoundByWriteTimeIntentAndKeptInRootScope(t *testing.T) {
	root, invocation, r, hashes := originalPreparationBudgetFixture(t)
	d, peer, e := openLifecycleRootStagingMaterialFiles(context.Background(), root, invocation, r, hashes, uint32(os.Getuid()), uint32(os.Getuid()))
	if e != nil {
		t.Fatal(e)
	}
	defer d.close()
	defer peer.close()
	if len(d.files) != 13 || d.files["budget-key.basis.private.json"] == nil || d.files["budget-key.result.private.json"] == nil {
		t.Fatal("actual original budget material omitted")
	}
	if d.checkComplete(false) != nil {
		t.Fatal("full root members unknown")
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("receipt enabled effects")
	}
}
func TestOriginalPreparationBudgetMaterialRejectsUnknownOrUnboundNativeConclusion(t *testing.T) {
	for _, mutation := range []string{"wrong_hash", "wrong_tuple", "borrow_final_descriptor", "receipt_authority", "missing_basis", "extra_key", "null_public_key", "one_intent_field"} {
		t.Run(mutation, func(t *testing.T) {
			root, invocation, r, hashes := originalPreparationBudgetFixture(t)
			var intent lifecycleSourceCopyIntent
			raw, _ := os.ReadFile(r.SourceCopyIntent.Path)
			if decodeLifecycleSourceCopyIntent(raw, &intent) != nil {
				t.Fatal("intent")
			}
			switch mutation {
			case "wrong_hash":
				intent.BudgetResultSHA256 = strings.Repeat("0", 64)
			case "missing_basis":
				_ = os.Remove(filepath.Join(root, "budget-key.basis.private.json"))
			default:
				name := "budget-key.result.private.json"
				if mutation == "borrow_final_descriptor" {
					name = "budget-key.basis.private.json"
				}
				raw, _ = os.ReadFile(filepath.Join(root, name))
				var v map[string]any
				_ = json.Unmarshal(raw, &v)
				switch mutation {
				case "wrong_tuple":
					v["actual_run_id"] = "888-1"
				case "borrow_final_descriptor":
					v["remote_descriptor_sha256"] = strings.Repeat("a", 64)
				case "receipt_authority":
					v["whole_writer_fence_proven"] = true
				case "extra_key":
					v["mutation_permit"] = true
				case "null_public_key":
					v["public_key"] = nil
				}
				raw, _ = json.Marshal(v)
				_ = os.WriteFile(filepath.Join(root, name), raw, 0600)
				if strings.Contains(name, "basis") {
					intent.BudgetDescriptorSHA256 = digestRaw(raw)
				} else {
					intent.BudgetResultSHA256 = digestRaw(raw)
				}
			}
			raw, _ = json.Marshal(intent)
			if mutation == "one_intent_field" {
				var v map[string]any
				_ = json.Unmarshal(raw, &v)
				delete(v, "budget_key_result_sha256")
				raw, _ = json.Marshal(v)
			}
			_ = os.WriteFile(r.SourceCopyIntent.Path, raw, 0600)
			r.SourceCopyIntent.SHA256 = digestRaw(raw)
			d, peer, e := openLifecycleRootStagingMaterialFiles(context.Background(), root, invocation, r, hashes, uint32(os.Getuid()), uint32(os.Getuid()))
			if e == nil || d != nil || peer != nil {
				t.Fatal("unbound budget producer material admitted")
			}
		})
	}
}

func TestOriginalHistoricalServiceInputsUseExistingApprovalHashesAndExactSourceFDs(t *testing.T) {
	for _, mutation := range []string{"none", "wrong_hash", "missing", "replace_inode", "nil_control"} {
		t.Run(mutation, func(t *testing.T) {
			path, _ := filepath.EvalSymlinks(t.TempDir())
			_ = os.Chmod(path, 0700)
			d, e := openLifecycleMaterialDirectory(path, uint32(os.Getuid()))
			if e != nil {
				t.Fatal(e)
			}
			defer d.close()
			r := lifecycleRequest{ServiceControl: &lifecycleServiceControl{}}
			for name, value := range map[string]string{"approved-services.json": "original final A descriptor", "ssh-channel.json": "original approved channel"} {
				raw, _ := json.Marshal(value)
				_ = os.WriteFile(filepath.Join(path, name), raw, 0600)
				if name == "approved-services.json" {
					r.ServiceControl.LocalDescriptorSHA256 = digestRaw(raw)
				} else {
					r.ServiceControl.SSHChannelSHA256 = digestRaw(raw)
				}
			}
			if mutation == "wrong_hash" {
				r.ServiceControl.SSHChannelSHA256 = strings.Repeat("0", 64)
			}
			if mutation == "missing" {
				_ = os.Remove(filepath.Join(path, "ssh-channel.json"))
			}
			if mutation == "nil_control" {
				r.ServiceControl = nil
			}
			e = registerLifecycleHistoricalServiceInputs(context.Background(), d, r, uint32(os.Getuid()))
			if mutation == "wrong_hash" || mutation == "missing" {
				if e == nil {
					t.Fatal("unbound source input admitted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if mutation == "nil_control" {
				if len(d.files) != 0 {
					t.Fatal("optional old request guessed files")
				}
				return
			}
			if len(d.files) != 2 {
				t.Fatal("exact source input pair omitted")
			}
			for _, v := range d.files {
				if v.file == nil || v.retained || d.checkFile(v) != nil {
					t.Fatal("actual source FD missing")
				}
			}
			if mutation == "replace_inode" {
				p := filepath.Join(path, "ssh-channel.json")
				raw, _ := os.ReadFile(p)
				_ = os.Rename(p, p+".old")
				_ = os.WriteFile(p, raw, 0600)
				_ = os.Remove(p + ".old")
				if d.checkFile(d.files["ssh-channel.json"]) == nil {
					t.Fatal("replacement source inode admitted")
				}
			}
		})
	}
}

func originalOperationInventorySourceFixture(t *testing.T) (*lifecycleMaterialDirectory, lifecycleRequest) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(root, 0700)
	put := func(directory, name string, value any) string {
		b, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		b = append(b, '\n')
		if e = os.WriteFile(filepath.Join(directory, name), b, 0600); e != nil {
			t.Fatal(e)
		}
		return digestRaw(b)
	}
	source := strings.Repeat("a", 40)
	op := "123-1"
	identityRun := "111-1"
	boundsRun := "222-1"
	q := request{FormatVersion: 2, Kind: "readonly_inventory_boundary_request", OperationID: op, SourceSHA: source, TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Identities: map[string]string{"mysql": strings.Repeat("a", 64), "mongodb": strings.Repeat("b", 64)}, Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}}
	boundaryRequestSHA := put(root, "boundary-request.json", q)
	identityRequestValue := identityRequest{FormatVersion: 1, Kind: "readonly_identity_discovery_request", SourceSHA: source, OperationID: op, TargetHash: digest(targets)}
	identityRequestSHA := put(root, "identity-request.json", identityRequestValue)
	identityRoot := filepath.Join(root, "identity-"+identityRun)
	boundsRoot := filepath.Join(root, "bounds-"+boundsRun)
	for _, d := range []string{identityRoot, boundsRoot} {
		if os.Mkdir(d, 0700) != nil {
			t.Fatal("fixture")
		}
	}
	identitySHA := put(identityRoot, "identity.private.json", identityReport{FormatVersion: 1, Kind: "readonly_identity_discovery", SourceSHA: source, OperationID: op, RunID: identityRun, RequestHash: identityRequestSHA, TargetHash: digest(targets), Complete: true, DiagnosticOnly: true, ErrorCategory: "none"})
	boundsSHA := put(boundsRoot, "boundary.private.json", report{FormatVersion: 2, Kind: "readonly_inventory_boundaries", SourceSHA: source, OperationID: op, RunID: boundsRun, RequestHash: boundaryRequestSHA, TargetHash: digest(targets), Complete: true, DiagnosticOnly: true, ErrorCategory: "none", SourceBytesProtocol: "no_source_body_copy"})
	q.Kind = "readonly_inventory_request"
	q.BoundaryRunID = boundsRun
	q.BoundaryReportHash = boundsSHA
	inventoryRequestSHA := put(root, "inventory-request.json", q)
	identityRef := lifecycleOriginalReportReference{identityRun, source, identitySHA}
	boundaryRef := lifecycleOriginalReportReference{boundsRun, source, boundsSHA}
	for _, b := range []struct {
		name, run, mode, sha string
		boundary             any
	}{{"boundary-request-bootstrap.json", "333-1", "bootstrap-bounds", boundaryRequestSHA, nil}, {"inventory-request-bootstrap.json", "444-1", "bootstrap-inventory", inventoryRequestSHA, boundaryRef}} {
		put(root, b.name, map[string]any{"format_version": 1, "kind": "private_request_bootstrap_binding", "source_sha": source, "operation_id": op, "created_run_id": b.run, "prepare_mode": b.mode, "approval_sha256": strings.Repeat("c", 64), "request_sha256": b.sha, "identity_report": identityRef, "boundary_report": b.boundary})
	}
	if os.WriteFile(filepath.Join(root, "operation.lock"), nil, 0600) != nil {
		t.Fatal("fixture")
	}
	d, e := openLifecycleMaterialDirectory(root, uint32(os.Getuid()))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = d.close() })
	if e = d.register("inventory-request.json", inventoryRequestSHA, uint32(os.Getuid()), 0600); e != nil {
		t.Fatal(e)
	}
	return d, lifecycleRequest{OriginalSourceSHA: source, OperationID: op, Approval: backup.Approval{RunID: "456-1", RequestHash: inventoryRequestSHA}}
}
func TestOriginalOperationInventoryCatalogUsesLinkedOriginalHashChain(t *testing.T) {
	d, r := originalOperationInventorySourceFixture(t)
	if e := registerLifecycleOriginalOperationLock(d, uint32(os.Getuid())); e != nil {
		t.Fatal(e)
	}
	if e := completeLifecycleOriginalInventoryInputs(context.Background(), r, d, uint32(os.Getuid())); e != nil {
		t.Fatal(e)
	}
	if e := d.checkComplete(false); e != nil {
		t.Fatal(e)
	}
	if len(d.files) != 6 || len(d.children) != 2 {
		t.Fatal("original fixed producer namespace not closed")
	}
	for _, child := range d.children {
		for _, f := range child.files {
			if !f.retained {
				t.Fatal("body-free diagnostic report not retained")
			}
		}
	}
	if e := os.WriteFile(filepath.Join(d.path, "foreign.json"), []byte("{}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if d.checkComplete(false) == nil || d.files["foreign.json"] != nil {
		t.Fatal("foreign material adopted")
	}
}
func TestOriginalOperationInventoryCatalogRejectsChangedReferenceAndBusyLock(t *testing.T) {
	for _, mutation := range []string{"request_hash", "report_hash", "operation", "run_reuse", "foreign", "busy"} {
		t.Run(mutation, func(t *testing.T) {
			d, r := originalOperationInventorySourceFixture(t)
			if mutation == "busy" {
				fd, e := syscall.Open(filepath.Join(d.path, "operation.lock"), syscall.O_RDONLY, 0)
				if e != nil {
					t.Fatal(e)
				}
				defer syscall.Close(fd)
				if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
					t.Fatal(e)
				}
				if registerLifecycleOriginalOperationLock(d, uint32(os.Getuid())) == nil {
					t.Fatal("active producer lock accepted")
				}
				return
			}
			path := filepath.Join(d.path, "inventory-request-bootstrap.json")
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			var value map[string]any
			if json.Unmarshal(raw, &value) != nil {
				t.Fatal("fixture")
			}
			switch mutation {
			case "request_hash":
				value["request_sha256"] = strings.Repeat("0", 64)
			case "report_hash":
				value["identity_report"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "operation":
				value["operation_id"] = "999-1"
			case "run_reuse":
				value["created_run_id"] = "111-1"
			case "foreign":
				value["arbitrary_source"] = "foreign.json"
			}
			raw, _ = json.Marshal(value)
			if os.WriteFile(path, raw, 0600) != nil {
				t.Fatal("fixture")
			}
			if completeLifecycleOriginalInventoryInputs(context.Background(), r, d, uint32(os.Getuid())) == nil {
				t.Fatal("unbound producer chain accepted")
			}
		})
	}
}

func TestOriginalOperationMetadataCatalogBindsParentApprovalAndProposal(t *testing.T) {
	for _, mutation := range []string{"none", "metadata_hash", "proposal_hash", "approval_hash", "foreign"} {
		t.Run(mutation, func(t *testing.T) {
			d, r := originalOperationInventorySourceFixture(t)
			put := func(directory, name string, value any) string {
				b, _ := json.Marshal(value)
				b = append(b, '\n')
				if os.WriteFile(filepath.Join(directory, name), b, 0600) != nil {
					t.Fatal("fixture")
				}
				return digestRaw(b)
			}
			r.Approval.InventorySHA256 = strings.Repeat("e", 64)
			requestValue := map[string]any{"format_version": 1, "kind": "readonly_compatibility_history_request", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID, "run_id": r.Approval.RunID}
			requestHash := put(d.path, "history-request.json", requestValue)
			if e := d.register("history-request.json", requestHash, uint32(os.Getuid()), 0600); e != nil {
				t.Fatal(e)
			}
			inventoryRef := map[string]any{"run_id": r.Approval.RunID, "sha256": r.Approval.InventorySHA256}
			metaRun := "777-1"
			metadataApproval := map[string]any{"format_version": 1, "kind": "readonly_history_metadata_approval", "prepare_mode": "bootstrap-history-metadata", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID}
			metadataApprovalBytes, _ := json.Marshal(metadataApproval)
			metadataApprovalHash := digestRaw(append(metadataApprovalBytes, '\n'))
			path := filepath.Join(d.path, "history-metadata-"+metaRun)
			if os.Mkdir(path, 0700) != nil {
				t.Fatal("fixture")
			}
			metadata := map[string]any{"format_version": 1, "kind": "readonly_history_file_metadata", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID, "run_id": metaRun, "metadata_limits": map[string]any{}, "approved_inventory_report": inventoryRef, "inventory_request_sha256": r.Approval.RequestHash, "approval_sha256": metadataApprovalHash, "parent_proposal_run_id": r.Approval.RunID, "parent_proposal_sha256": requestHash, "assets": []any{}, "equal_full_physical_passes": 2, "metadata_complete": true, "input_baseline_sha256": strings.Repeat("f", 64), "semantic_source_coverage_verified": false, "production_process_budget_proven": false, "complete": false, "execution_allowed": false, "drop_ready": false, "cas_complete": false}
			metadataHash := put(path, "history-metadata.json", metadata)
			put(path, "history-parent-proposal.json", requestValue)
			put(path, "history-metadata-bootstrap.json", map[string]any{"format_version": 1, "kind": "readonly_history_metadata_binding", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID, "created_run_id": metaRun, "approval_sha256": metadataApprovalHash, "approval": metadataApproval, "parent_proposal_run_id": r.Approval.RunID, "parent_proposal_sha256": requestHash})
			parentApproval := map[string]any{"format_version": 1, "kind": "readonly_history_parent_registration_approval", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID}
			b, _ := json.Marshal(parentApproval)
			parentApprovalHash := digestRaw(append(b, '\n'))
			record := map[string]any{"format_version": 1, "kind": "readonly_history_parent_registration", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID, "created_run_id": "888-1", "approval_sha256": parentApprovalHash, "approval": parentApproval, "request_sha256": requestHash, "parent_run_id": r.Approval.RunID, "metadata_report": map[string]any{"run_id": metaRun, "sha256": metadataHash}, "inventory_report": inventoryRef}
			switch mutation {
			case "metadata_hash":
				record["metadata_report"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "proposal_hash":
				if os.WriteFile(filepath.Join(path, "history-parent-proposal.json"), []byte("{}\n"), 0600) != nil {
					t.Fatal("fixture")
				}
			case "approval_hash":
				record["approval_sha256"] = strings.Repeat("0", 64)
			case "foreign":
				if os.WriteFile(filepath.Join(path, "foreign.json"), []byte("{}\n"), 0600) != nil {
					t.Fatal("fixture")
				}
			}
			put(d.path, "history-request-bootstrap.json", record)
			e := completeLifecycleOriginalMetadataInputs(context.Background(), r, d, uint32(os.Getuid()))
			if mutation == "none" {
				if e != nil {
					t.Fatal(e)
				}
				if !d.children["history-metadata-"+metaRun].files["history-metadata.json"].retained {
					t.Fatal("body-free metadata diagnostic not retained")
				}
			} else if e == nil {
				t.Fatal("unbound metadata producer admitted")
			}
		})
	}
}

func TestOriginalAIExecMaterialCatalogChecksOriginalTupleAndPrefixOnly(t *testing.T) {
	source := strings.Repeat("a", 40)
	r := lifecycleRequest{OriginalSourceSHA: source, OperationID: "123-1", FinalHistory: &lifecycleFinalHistoryInput{RuntimeSourceSHA: strings.Repeat("b", 40), ImageID: "sha256:" + strings.Repeat("c", 64), ContainerID: strings.Repeat("d", 64)}}
	binding := lifecycleOriginalAIExecBinding{SourceSHA: source, OperationID: r.OperationID, RunID: "456", RuntimeSourceSHA: r.FinalHistory.RuntimeSourceSHA, ImageID: r.FinalHistory.ImageID, ContainerID: r.FinalHistory.ContainerID, PythonSHA256: strings.Repeat("e", 64), InputSHA256: strings.Repeat("f", 64), DeadlineUnixNano: 1}
	first := lifecycleOriginalAIExecRecord{Protocol: "qs-ai-exec-lifecycle/v1", Sequence: 1, Binding: binding, Stage: "create_intent", EngineVersionSHA256: strings.Repeat("a", 64)}
	initial, _ := json.Marshal(first)
	initial = append(initial, '\n')
	next := first
	next.Sequence = 2
	next.PreviousSHA256 = digestRaw(initial)
	next.Stage = "unknown"
	second, _ := json.Marshal(next)
	second = append(second, '\n')
	original := append(append([]byte(nil), initial...), second...)
	// Unknown remains unknown. Registration may account for original bytes but
	// supplies no quiescence, native completion or release permission.
	if e := validateLifecycleOriginalAIExecMaterial(original, r); e != nil {
		t.Fatal(e)
	}
	for _, mutation := range []string{"source", "operation", "runtime", "container", "prefix", "sequence", "unknown_field", "truncated", "oversized"} {
		t.Run(mutation, func(t *testing.T) {
			altered := next
			switch mutation {
			case "source":
				altered.Binding.SourceSHA = strings.Repeat("0", 40)
			case "operation":
				altered.Binding.OperationID = "999-1"
			case "runtime":
				altered.Binding.RuntimeSourceSHA = strings.Repeat("0", 40)
			case "container":
				altered.Binding.ContainerID = strings.Repeat("0", 64)
			case "prefix":
				altered.PreviousSHA256 = strings.Repeat("0", 64)
			case "sequence":
				altered.Sequence = 4
			}
			b, _ := json.Marshal(altered)
			if mutation == "unknown_field" {
				var fields map[string]any
				_ = json.Unmarshal(b, &fields)
				fields["private_body"] = "forbidden"
				b, _ = json.Marshal(fields)
			}
			raw := append(append([]byte(nil), initial...), append(b, '\n')...)
			if mutation == "truncated" {
				raw = raw[:len(raw)-1]
			}
			if mutation == "oversized" {
				raw = make([]byte, (64<<10)+1)
			}
			if validateLifecycleOriginalAIExecMaterial(raw, r) == nil {
				t.Fatal("unbound original exec material accepted")
			}
		})
	}
}

func TestOriginalReadonlyHistoryCatalogFollowsFrozenProofAndTerminalHashes(t *testing.T) {
	for _, mutation := range []string{"none", "readiness_hash", "source_hash", "unknown_terminal", "shadow_foreign", "output_foreign", "run_reuse"} {
		t.Run(mutation, func(t *testing.T) {
			root, r, writer, inventory := historicalWriteInputFixture(t, true)
			registration, previous, e := openLifecycleHistoricalWriteInputFiles(context.Background(), r, writer, inventory, root, uint32(os.Getuid()))
			if e != nil {
				t.Fatal(e)
			}
			defer registration.close()
			parent := previous[len(previous)-1]
			defer parent.close()
			for _, d := range previous[:len(previous)-1] {
				defer d.close()
			}
			put := func(path string, value any) string {
				b, _ := json.Marshal(value)
				b = append(b, '\n')
				if os.WriteFile(path, b, 0600) != nil {
					t.Fatal("fixture")
				}
				return digestRaw(b)
			}
			run := "888-1"
			history := filepath.Join(root, "history-"+run)
			output := filepath.Join(history, "output")
			shadow := filepath.Join(history, "mysql-volume-shadow")
			for _, d := range []string{history, output, shadow} {
				if os.Mkdir(d, 0700) != nil {
					t.Fatal("fixture")
				}
			}
			var requestValue lifecycleHistoricalRequestMaterial
			raw, _ := os.ReadFile(filepath.Join(root, "history-request.json"))
			if json.Unmarshal(raw, &requestValue) != nil {
				t.Fatal("fixture")
			}
			requestValue.RunID = run
			requestHash := put(filepath.Join(history, "history.request.json"), requestValue)
			approval := map[string]any{"format_version": 1, "kind": "readonly_history_bootstrap_approval", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID}
			approvalRaw, _ := json.Marshal(approval)
			put(filepath.Join(history, "history.bootstrap.json"), map[string]any{"format_version": 1, "kind": "immutable_history_run_derivation", "source_sha": r.OriginalSourceSHA, "operation_id": r.OperationID, "actual_run_id": run, "approval_sha256": digestRaw(append(approvalRaw, '\n')), "approval": approval, "parent_request_sha256": parent.files["history-request.json"].hash, "parent_run_id": r.Approval.RunID, "derived_request_sha256": requestHash, "only_changed_field": "run_id", "binary_sha256": strings.Repeat("a", 64)})
			readiness := map[string]any{}
			for _, key := range strings.Fields("completed_readonly_pipeline source_files_and_actual_origins_matched whole_four_source_coverage_complete business_and_responsibility_facts_unchanged independent_production_approval_verified ordered_mongo_source_metadata_approved host_process_budget_proven full_external_ai_closure_verified distributed_atomic_snapshot writer_fence_proven cas_complete post_cas_readback_complete backup_restore_qualified mutation_backend_enabled drop_ready") {
				readiness[key] = false
			}
			for _, key := range strings.Fields("completed_readonly_pipeline source_files_and_actual_origins_matched whole_four_source_coverage_complete business_and_responsibility_facts_unchanged") {
				readiness[key] = true
			}
			for _, key := range strings.Fields("local_candidates locally_qualified blocked_local joint_event_pages ai_blocked_pages sql_ledger_count mongo_collection_count elapsed_milliseconds") {
				readiness[key] = 0
			}
			for _, key := range strings.Fields("whole_source_index_sha256 candidate_sha256 sql_current_facts_sha256 mongo_current_facts_sha256") {
				readiness[key] = strings.Repeat("e", 64)
			}
			readiness["protocol"] = "qs-compatibility-history-readonly/v1"
			readiness["source_sha"] = r.OriginalSourceSHA
			readiness["operation_id"] = r.OperationID
			readiness["run_id"] = run
			readiness["request_sha256"] = requestHash
			readiness["inventory_request_sha256"] = r.Approval.RequestHash
			readiness["inventory_report_sha256"] = r.Approval.InventorySHA256
			readiness["independent_epochs"] = 2
			readiness["sql_global"] = map[string]any{}
			readiness["mongo_global"] = map[string]any{}
			readiness["ai_reverse_global"] = map[string]any{}
			readiness["blocking_reasons"] = map[string]uint64{}
			readiness["required_adapters"] = []string{}
			readiness["error_category"] = "none"
			var inv report
			raw, _ = os.ReadFile(filepath.Join(inventory.path, "inventory.private.json"))
			_ = json.Unmarshal(raw, &inv)
			hashes := []string{}
			sources := []retirement.SourceCopyReceipt{}
			for n, asset := range requestValue.Assets {
				hashes = append(hashes, asset.SHA256)
				protocol := retirement.SQLSourceProtocol
				if targets[n][0] == "mongodb" {
					protocol = retirement.MongoSourceProtocol
				}
				sources = append(sources, retirement.SourceCopyReceipt{Protocol: protocol, Records: inv.Targets[n].Records, Bytes: inv.Targets[n].Bytes, DataHash: inv.Targets[n].DataHash, Complete: true})
			}
			if mutation == "source_hash" {
				hashes[0] = strings.Repeat("0", 64)
			}
			readiness["sources"] = sources
			readiness["full_source_file_sha256"] = hashes
			readinessHash := put(filepath.Join(output, "history.readiness.json"), readiness)
			cid := strings.Repeat("b", 64)
			image := "sha256:" + strings.Repeat("c", 64)
			nonce := strings.Repeat("d", 64)
			name := "qs-compatibility-history-" + run
			labels := map[string]string{"qs.compatibility-retirement.operation": r.OperationID, "qs.compatibility-retirement.run": run, "qs.compatibility-retirement.source": r.OriginalSourceSHA, "qs.compatibility-retirement.request": requestHash, "qs.compatibility-retirement.creation": nonce, "qs.compatibility-retirement.kind": "history-readonly"}
			created := map[string]any{"id": cid, "name": name, "image": image, "labels": labels, "mounts": map[string]any{}, "limits": map[string]any{}, "owner_uid": os.Getuid(), "owner_gid": os.Getgid()}
			put(filepath.Join(history, "history.container.json"), created)
			delete(created, "id")
			created["declared_image_volumes"] = map[string]any{"/var/lib/mysql": map[string]any{}}
			put(filepath.Join(history, "history.creation.intent.json"), created)
			terminal := map[string]any{"id": cid, "name": name, "image": image, "actual_run_id": run, "source_sha": r.OriginalSourceSHA, "request_sha256": requestHash, "creation_nonce": nonce, "owner_uid": os.Getuid(), "owner_gid": os.Getgid(), "status": "exited", "exit_code": 0, "container_removed": true, "private_readiness_sha256": readinessHash}
			switch mutation {
			case "readiness_hash":
				terminal["private_readiness_sha256"] = strings.Repeat("0", 64)
			case "unknown_terminal":
				terminal["status"] = "running"
			case "shadow_foreign":
				if os.WriteFile(filepath.Join(shadow, "foreign"), nil, 0600) != nil {
					t.Fatal("fixture")
				}
			case "output_foreign":
				if os.WriteFile(filepath.Join(output, "foreign"), nil, 0600) != nil {
					t.Fatal("fixture")
				}
			}
			put(filepath.Join(history, "history.terminal.json"), terminal)
			summary := map[string]any{}
			for _, key := range strings.Fields("classified verified_live verified_retired unverifiable_closed unresolved ambiguous hash_conflicts unknown_execution unexplained_high retirement_references") {
				summary[key] = 0
			}
			summary["references_hash"] = strings.Repeat("f", 64)
			producer := map[string]string{"protocol": "qs_compatibility_retirement_history_v1", "source_sha": r.OriginalSourceSHA, "run_id": run}
			if mutation == "run_reuse" {
				producer["run_id"] = r.Approval.RunID
			}
			proof := map[string]json.RawMessage{}
			proof["producer"], _ = json.Marshal(producer)
			proof["summary"], _ = json.Marshal(summary)
			e = completeLifecycleOriginalReadOnlyHistory(context.Background(), r, parent, inventory, proof, uint32(os.Getuid()))
			if mutation == "none" {
				if e != nil {
					t.Fatal(e)
				}
				d := parent.children["history-"+run]
				if len(d.files) != 5 || len(d.children) != 2 || !d.children["output"].files["history.readiness.json"].retained || !d.files["history.terminal.json"].retained {
					t.Fatal("fixed history producer scope not closed")
				}
			} else if e == nil {
				t.Fatal("unbound readonly producer accepted")
			}
		})
	}
}
