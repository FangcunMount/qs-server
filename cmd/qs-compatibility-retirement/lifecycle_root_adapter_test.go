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
	"testing"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
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
		pages := records/1000 + 1
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
				count := min(uint64(page)*1000, records)
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
	for _, records := range []uint64{0, 1, 1000, 1001} {
		t.Run(fmt.Sprint(records), func(t *testing.T) {
			dir, a, hashes := originalInventoryMaterialFixture(t, records)
			d, e := openLifecycleInventoryMaterialFiles(context.Background(), dir, uint32(os.Getuid()), a, hashes)
			if e != nil {
				t.Fatal(e)
			}
			defer d.close()
			pages := records/1000 + 1
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
	root, _ := filepath.EvalSymlinks(t.TempDir())
	invocation, _ := filepath.EvalSymlinks(t.TempDir())
	for _, dir := range []string{root, invocation} {
		if os.Chmod(dir, 0700) != nil {
			t.Fatal("directory")
		}
	}
	originalRoot := filepath.Join("/opt/backups/qs-server/compatibility-retirement", approval.OperationID)
	manifest := []byte("offline frozen manifest")
	r := lifecycleRequest{FormatVersion: 1, Kind: "compatibility_retirement_lifecycle_request", OriginalSourceSHA: approval.SourceSHA, ToolSourceSHA: strings.Repeat("d", 40), OperationID: approval.OperationID, ActualRunID: "789-1", ManifestSHA256: digestRaw(manifest), Approval: approval, ArchiveDirectory: filepath.Join(originalRoot, "archive"), SourceDirectory: filepath.Join(originalRoot, "inventory-"+approval.RunID), SourceFileSHA256: hashes,
		Recovery: backup.TargetRecoveryRequest{SourceSHA: approval.SourceSHA, OperationID: approval.OperationID, OriginalRunID: approval.RunID, ActualRunID: "789-1"}}
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
	i := lifecycleSourceCopyIntent{1, "root_once_exact_source_copy_intent", r.OriginalSourceSHA, r.ToolSourceSHA, r.OperationID, r.ActualRunID, approval.RunID, digestRaw(request), r.ManifestSHA256, hashes, targets, uint32(os.Getuid()), r.ArchiveDirectory, child, false, true}
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
			if len(d.files) != 5 || len(d.children) != 1 || len(d.children["inventory-"+r.Approval.RunID].files) != 7 || window && (peer == nil || len(peer.files) != 3) || !window && peer != nil {
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
