package main

import (
	"bytes"
	"context"
	"encoding/json"
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
