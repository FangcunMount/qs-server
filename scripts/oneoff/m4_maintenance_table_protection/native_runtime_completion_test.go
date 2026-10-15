//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/sys/unix"
)

// This independent selector is deliberately outside the historical native
// prefix. Its required top/children must be added by a separate CI task; the
// original 74 events and SQL resources are not silently enlarged here.
func TestM4MaintenanceTablesCommittedRuntimeCompletionNative(t *testing.T) {
	for _, mode := range []string{"cancel102", "target103", "restored102"} {
		t.Run("CreateACKUnknown_"+mode, func(t *testing.T) {
			h := newMaintenanceNative(t)
			h.capture()
			var restorePath, restoreHash string
			if mode == "target103" {
				h.upgradePrefix(len(scope), true)
			}
			if mode == "restored102" {
				h.upgradePrefix(len(scope), true)
				args, p, hash := h.restoreInput("8100-1")
				restorePath, restoreHash = p, hash
				r := h.action("restore", args...)
				h.assertRestoreReleased(r)
				if !r.Complete {
					t.Fatal("actual restoration fixture failed", r)
				}
			}
			intent := h.runtimeArgs(mode, restorePath, restoreHash)
			seen, _, undo := h.useACKTransport("runtime_boundary_create_ack_loss", nil)
			failed, oldRaw, code := h.completionInvocation("mark-first-runtime", intent, nil)
			undo()
			if !seen.Load() || failed.Complete || code != 1 || !failed.DatabaseBoundaryAdvanceUnknown || failed.MarkerWritten {
				t.Fatal("actual committed CTAS ACK-loss prerequisite failed", failed)
			}
			oldPath := filepath.Join(h.dir, "original-unknown-stdout.json")
			if e := writeExclusive(oldPath, oldRaw); e != nil {
				t.Fatal(e)
			}
			args := h.currentCompletionArgs(intent, "absent", "8101-1")
			before, head, identity := h.inspectionSQLWitness(), h.mongoHead(), h.independentBinding()
			reader := h.inspectionReader(false)
			reader.confirmationInsertForbidden()
			result, _, exit := reader.completionInvocation("complete-first-runtime", args, nil)
			reader.assertCurrentCompletion(result, exit, true)
			if before != h.inspectionSQLWitness() || !bytes.Equal(head, h.mongoHead()) || identity != h.independentBinding() {
				t.Fatal("confirmation mutated actual database facts")
			}
			after, e := os.ReadFile(oldPath)
			if e != nil || !bytes.Equal(oldRaw, after) {
				t.Fatal("original unknown stdout was rewritten")
			}
			retry, _, _ := h.completionInvocation("complete-first-runtime", args, nil)
			h.assertRestoreReleased(retry)
			if retry.Complete || retry.RuntimeCompletion != nil || retry.Error != "first_runtime_marker_unknown_or_started" {
				t.Fatal("absent authorization was silently reused for an existing marker", retry)
			}
		})
	}
	t.Run("UpdateCommitACKUnknown", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		h.assertRuntimeMarker(h.action("mark-first-runtime", h.runtimeArgs("cancel102", "", "")...))
		h = h.captureAnotherDirectory()
		intent := h.runtimeArgs("cancel102", "", "")
		seen, _, undo := h.useACKTransport("runtime_boundary_commit_ack_loss", nil)
		failed, oldRaw, code := h.completionInvocation("mark-first-runtime", intent, nil)
		undo()
		if !seen.Load() || code != 1 || failed.Complete || !failed.DatabaseBoundaryAdvanceUnknown || failed.MarkerWritten {
			t.Fatal("actual CAS COMMIT ACK-loss prerequisite failed", failed)
		}
		args := h.currentCompletionArgs(intent, "absent", "8102-1")
		before := h.inspectionSQLWitness()
		result, _, exit := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertCurrentCompletion(result, exit, true)
		if result.DatabaseBoundary.Generation != 2 || before != h.inspectionSQLWitness() || !bytes.Contains(oldRaw, []byte(`"runtime_database_boundary_advance_unknown":true`)) {
			t.Fatal("same G2 confirmation advanced or rewrote the unknown result")
		}
	})
	t.Run("ExistingOriginalMarkerAfterOutputLoss", nativeCompletionAcrossActualOldCLI)

	t.Run("CompletionOutputLossThenIdempotentExisting", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		seen, _, undo := h.useACKTransport("runtime_boundary_create_ack_loss", nil)
		r, _, _ := h.completionInvocation("mark-first-runtime", intent, nil)
		undo()
		if !seen.Load() || !r.DatabaseBoundaryAdvanceUnknown {
			t.Fatal("actual committed intent fixture missing")
		}
		absent := h.currentCompletionArgs(intent, "absent", "8104-1")
		_, _, exit := h.completionInvocation("complete-first-runtime", absent, badOutput{})
		if exit != 1 {
			t.Fatal("lost completion output accepted")
		}
		args := h.currentCompletionArgs(intent, "same_existing", "8105-1")
		before := h.inspectionSQLWitness()
		marker, _ := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		for range 2 {
			r, _, exit := h.completionInvocation("complete-first-runtime", args, nil)
			h.assertCurrentCompletion(r, exit, false)
		}
		got, _ := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		if !bytes.Equal(marker, got) || before != h.inspectionSQLWitness() {
			t.Fatal("repeat confirmation changed existing bytes or generation")
		}
	})
	t.Run("PartialMarkerRemainsBlocked", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		_, _, undo := h.useACKTransport("runtime_boundary_create_marker_failure", func() error {
			return writeExclusive(derivedRuntimeMarker(h.original), []byte("original partial marker"))
		})
		failed, _, exit := h.completionInvocation("mark-first-runtime", intent, nil)
		undo()
		if exit != 1 || failed.Complete || !failed.DatabaseBoundaryAdvanced {
			t.Fatal("committed partial-marker prerequisite missing", failed)
		}
		args := h.currentCompletionArgs(intent, "same_existing", "8106-1")
		before := h.inspectionSQLWitness()
		marker, _ := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		r, _, _ := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertRestoreReleased(r)
		if r.Complete || r.RuntimeCompletion != nil || r.Error != "runtime_completion_marker_rejected" {
			t.Fatal("partial marker gained completion authority", r)
		}
		after, _ := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		if !bytes.Equal(marker, after) || before != h.inspectionSQLWitness() {
			t.Fatal("partial diagnostic material changed")
		}
	})
	t.Run("RetainedDriftBlocked", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		h.assertRuntimeMarker(h.action("mark-first-runtime", intent...))
		args := h.currentCompletionArgs(intent, "same_existing", "8107-1")
		result, e := h.db.ExecContext(t.Context(), "UPDATE testee SET name=CONCAT(name,' drift')")
		if e != nil {
			t.Fatal(e)
		}
		if n, e := result.RowsAffected(); e != nil || n == 0 {
			t.Fatal("actual nonempty retained drift fixture unavailable")
		}
		before := h.inspectionSQLWitness()
		r, _, _ := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertRestoreReleased(r)
		if r.Complete || r.RuntimeCompletion != nil || r.Error != "runtime_completion_current_facts_changed" || before != h.inspectionSQLWitness() {
			t.Fatal("retained drift was accepted or repaired", r)
		}
	})
	t.Run("NewGenerationBlocked", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		h.assertRuntimeMarker(h.action("mark-first-runtime", intent...))
		args := h.currentCompletionArgs(intent, "same_existing", "8108-1")
		fresh := h.captureAnotherDirectory()
		fresh.assertRuntimeMarker(fresh.action("mark-first-runtime", fresh.runtimeArgs("cancel102", "", "")...))
		before := h.inspectionSQLWitness()
		r, _, _ := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertRestoreReleased(r)
		if r.Complete || r.RuntimeCompletion != nil || r.Error != "runtime_completion_committed_record_rejected" || before != h.inspectionSQLWitness() {
			t.Fatal("old intent crossed a newer committed generation", r)
		}
	})
	t.Run("ReleaseUnknownWithholdsTypedConfirmation", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		h.assertRuntimeMarker(h.action("mark-first-runtime", intent...))
		args := h.currentCompletionArgs(intent, "same_existing", "8109-1")
		before := h.inspectionSQLWitness()
		seen, _, undo := h.useACKTransport("runtime_release_prepare_loss", nil)
		r, _, exit := h.completionInvocation("complete-first-runtime", args, nil)
		undo()
		if !seen.Load() || exit != 1 || r.Complete || r.RuntimeCompletion != nil || !r.RuntimeGateReleaseUnproven || r.RuntimeGateReleased || r.Error != "runtime_gate_release_unproven" || before != h.inspectionSQLWitness() {
			t.Fatal("unknown release became a terminal confirmation", r)
		}
	})
	t.Run("SharedServerGateBlocksConfirmation", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		h.assertRuntimeMarker(h.action("mark-first-runtime", intent...))
		args := h.currentCompletionArgs(intent, "same_existing", "8110-1")
		before := h.inspectionSQLWitness()
		conn, e := h.db.Conn(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		key, _ := runtimeGateKey(h.binding)
		var acquired int
		if conn.QueryRowContext(t.Context(), "SELECT GET_LOCK(?,0)", key).Scan(&acquired) != nil || acquired != 1 {
			_ = conn.Close()
			t.Fatal("owned exact database gate unavailable")
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
			defer cancel()
			var released int
			releaseErr := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", key).Scan(&released)
			closeErr := conn.Close()
			if releaseErr != nil || released != 1 || closeErr != nil {
				t.Error("owned blocking server gate release unproven")
			}
			if before != h.inspectionSQLWitness() {
				t.Error("busy confirmation changed database facts")
			}
		}()
		r, _, exit := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertRestoreReleased(r)
		if exit != 1 || r.Complete || r.RuntimeCompletion != nil || r.RuntimeGateAcquired || r.Error != "runtime_gate_busy_or_unavailable" {
			t.Fatal("confirmation bypassed the actual same-database server gate", r)
		}
		// Use the independently owned administrative pool while the selected
		// fixture pool's only connection remains deliberately occupied.
		var generation uint64
		if h.root.QueryRowContext(t.Context(), "SELECT generation FROM `"+h.name+"`.m4_d_runtime_boundary").Scan(&generation) != nil || generation != 1 || before == "" {
			t.Fatal("busy confirmation changed or hid the committed generation")
		}
	})
	t.Run("DirtyTargetIsNotRepaired", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		h.upgradePrefix(len(scope), true)
		intent := h.runtimeArgs("target103", "", "")
		h.assertRuntimeMarker(h.action("mark-first-runtime", intent...))
		args := h.currentCompletionArgs(intent, "same_existing", "8111-1")
		if _, e := h.db.ExecContext(t.Context(), "UPDATE schema_migrations SET dirty=TRUE"); e != nil {
			t.Fatal(e)
		}
		before := h.inspectionSQLWitness()
		r, _, exit := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertRestoreReleased(r)
		if exit != 1 || r.Complete || r.RuntimeCompletion != nil || r.Error != "clean102_or103_expected_for_action" || before != h.inspectionSQLWitness() {
			t.Fatal("dirty target was accepted or repaired", r)
		}
	})
}

func (h *maintenanceNative) completionInvocation(action string, extras []string, out io.Writer) (receipt, []byte, int) {
	h.t.Helper()
	args := []string{"--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m"}
	args = append(args, extras...)
	args = append(args, "--capture-evidence-file="+h.captureEvidencePath, "--capture-evidence-sha256="+h.captureEvidenceHash)
	var stdout bytes.Buffer
	if out == nil {
		out = &stdout
	}
	exit := serve(h.t.Context(), args, func(key string) string {
		switch key {
		case "QS_M4_D_OBJECT_MYSQL_DSN":
			return h.dsn
		case "QS_M4_D_OBJECT_MONGO_URI":
			return h.uri
		}
		return ""
	}, h.tool, out)
	var r receipt
	if stdout.Len() > 0 {
		if strictDecode(stdout.Bytes(), &r) != nil || (exit == 0) != r.Complete {
			h.t.Fatal("actual original completion stdout/exit disagree")
		}
		h.t.Logf("owned completion action=%s exit=%d category=%s complete=%t", action, exit, r.Error, r.Complete)
	}
	return r, append([]byte(nil), stdout.Bytes()...), exit
}

func (h *maintenanceNative) currentCompletionArgs(original []string, markerState, operation string) []string {
	h.t.Helper()
	values := map[string]string{}
	for _, arg := range original {
		pair := strings.SplitN(strings.TrimPrefix(arg, "--"), "=", 2)
		if len(pair) == 2 {
			values[pair[0]] = pair[1]
		}
	}
	o, e := readRuntimeBoundary(h.t.Context(), h.db, h.binding)
	if e != nil || o.Record == nil {
		h.t.Fatal("actual committed row required", e)
	}
	markerHash := fingerprint(deterministicCompletionMarker(o, h.binding))
	if markerState == "same_existing" {
		raw, e := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		if e != nil {
			h.t.Fatal(e)
		}
		markerHash = fingerprint(raw)
	}
	now := time.Now().UTC()
	v := runtimeCompletionApproval{Format: 1, Kind: "m4_d_complete_committed_runtime_intent", Operation: operation, ToolSource: h.tool, OriginalToolSource: h.original.ToolSource, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, Binding: h.binding, MaterialHash: h.materialHash, CaptureEvidenceHash: h.captureEvidenceHash, IntentHash: values["runtime-intent-sha256"], RestoreApprovalHash: values["approval-sha256"], Boundary: o.State, RecordKind: o.Record.Kind, Mode: o.Record.Mode, SQLVersion: o.Record.SQLVersion, FactsHash: completionFactsHash(expectedCompletionFacts(h.original, o.Record.Mode)), Marker: derivedRuntimeMarker(h.original), MarkerState: markerState, MarkerHash: markerHash, FreezeHash: fingerprint([]byte("current owned no business writers")), ResponsibilityHash: fingerprint([]byte("current owned synthetic responsibility")), Decision: now.Format(time.RFC3339Nano), Expires: now.Add(2 * time.Minute).Format(time.RFC3339Nano), DataOwner: "system (10001)", RecoveryOwner: "system (10001)", RuntimeFrozen: true, RuntimeNotStarted: true, AllowCompletion: true}
	raw, _ := json.Marshal(v)
	path := filepath.Join(h.dir, "completion-"+operation+".json")
	if e := writeExclusive(path, raw); e != nil {
		h.t.Fatal(e)
	}
	return append(append([]string(nil), original...), "--completion-approval-file="+path, "--completion-approval-sha256="+fingerprint(raw))
}

func (h *maintenanceNative) assertCurrentCompletion(r receipt, exit int, created bool) {
	h.t.Helper()
	h.assertRestoreReleased(r)
	if exit != 0 || !r.Complete || r.Error != "none" || !r.ReadOnly || r.RuntimeCompletion == nil || r.RuntimeCompletion.AuthorizationMode != completionAuthorizationMode || r.RuntimeCompletion.BusinessStartAuthorized || r.RuntimeCompletion.MarkerCreated != created || r.RuntimeCompletion.MarkerHash == "" || r.ApprovalConsumed || r.MarkerWritten || r.MarkerWriteUnknown || r.MarkerHash != "" || r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown || !r.RuntimeGateAcquired || !r.RuntimeGateReleased || r.DatabaseBoundary == nil || r.DatabaseBoundaryRecord == nil {
		h.t.Fatal("actual current confirmation claimed old success, recovery or business authority", r)
	}
}

// The inherited account helper proves UPDATE/CREATE denial. This new action
// also requires the actual SELECT-only connection to reject INSERT, even an
// empty INSERT that could not alter the owned ledger if the grant were wrong.
func (h *maintenanceNative) confirmationInsertForbidden() {
	h.t.Helper()
	pool, e := sql.Open("mysql", h.dsn)
	if e != nil {
		h.t.Fatal("owned read-only INSERT probe unavailable")
	}
	defer func() {
		if pool.Close() != nil || pool.Stats().OpenConnections != 0 {
			h.t.Error("owned read-only INSERT probe release unknown")
		}
	}()
	ctx, cancel := context.WithTimeout(h.t.Context(), queryLimit)
	defer cancel()
	_, e = pool.ExecContext(ctx, "INSERT INTO schema_migrations SELECT version,dirty FROM schema_migrations WHERE 1=0")
	var server *mysql.MySQLError
	if !errors.As(e, &server) || server.Number != 1142 {
		h.t.Fatal("actual SELECT-only completion account did not reject INSERT")
	}
}

// This is one fixed historical CLI, not a caller-selected compatibility ref.
// CI builds the exact Git archive and independently freezes this executable's
// actual hash. The only consumer is this owned cross-source native fixture.
const completionOldCLISource = "5707bfdcdbf38bb2d5cf74d45d9db04e85f49b9c"
const completionOldCLIProbeArgument = "--fixed-old-cli-pre-environment-probe-must-be-invalid"

type completionOldCLI struct{ file, hash string }

func requiredCompletionOldCLI(getenv func(string) string) (completionOldCLI, error) {
	v := completionOldCLI{getenv("QS_M4_D_COMPLETION_OLD_CLI_FILE"), getenv("QS_M4_D_COMPLETION_OLD_CLI_SHA256")}
	if getenv("QS_M4_D_COMPLETION_OLD_CLI_REQUIRED") != "1" || !absolutePath(v.file) || !nonzeroHash(v.hash) {
		return v, errors.New("fixed_old_cli_explicit_binding_required")
	}
	if _, e := v.currentHash(); e != nil {
		return v, e
	}
	return v, nil
}

func (v completionOldCLI) currentHash() (hash string, resultErr error) {
	parent, e := privateParent(v.file)
	if e != nil {
		return "", errors.New("fixed_old_cli_private_parent_rejected")
	}
	defer func() {
		if parent.Close() != nil {
			hash = ""
			resultErr = errors.New("fixed_old_cli_parent_release_unknown")
		}
	}()
	fd, e := unix.Openat(int(parent.Fd()), filepath.Base(v.file), syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return "", errors.New("fixed_old_cli_unavailable")
	}
	file := os.NewFile(uintptr(fd), v.file)
	defer func() {
		if file.Close() != nil {
			hash = ""
			resultErr = errors.New("fixed_old_cli_file_release_unknown")
		}
	}()
	before, e := file.Stat()
	if e != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0700 || before.Size() <= 0 || before.Size() > 64<<20 {
		return "", errors.New("fixed_old_cli_private_executable_rejected")
	}
	stat, ok := before.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 {
		return "", errors.New("fixed_old_cli_owner_or_link_rejected")
	}
	digest := sha256.New()
	n, e := io.Copy(digest, io.LimitReader(file, 64<<20+1))
	after, se := file.Stat()
	if e != nil || se != nil || n != before.Size() || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return "", errors.New("fixed_old_cli_read_or_change_unproven")
	}
	hash = hex.EncodeToString(digest.Sum(nil))
	if hash != v.hash {
		return "", errors.New("fixed_old_cli_hash_rejected")
	}
	return hash, nil
}

type completionOldCLIOutput struct{ bytes.Buffer }

func (w *completionOldCLIOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > 65536 {
		return 0, errors.New("fixed_old_cli_output_limit")
	}
	return w.Buffer.Write(p)
}

// A test-local observation of the actual OS process, not a new tool receipt or
// approval protocol. []byte fields preserve exact stdout/stderr as base64 in
// the already retained Go JSONL, including an empty broken-pipe stdout.
type completionOldCLIObservation struct {
	Source       string   `json:"fixed_source"`
	BinaryBefore string   `json:"binary_sha256_before"`
	BinaryAfter  string   `json:"binary_sha256_after"`
	Argv         []string `json:"actual_argv"`
	Termination  string   `json:"termination"`
	ExitCode     *int     `json:"actual_exit_code"`
	Signal       int      `json:"actual_signal"`
	Stdout       []byte   `json:"actual_stdout_base64"`
	Stderr       []byte   `json:"actual_stderr_base64"`
	StdoutHash   string   `json:"actual_stdout_sha256"`
	StderrHash   string   `json:"actual_stderr_sha256"`
}

func (v completionOldCLI) invoke(ctx context.Context, args, dbEnv []string, closedPipe bool) (out completionOldCLIObservation, resultErr error) {
	out = completionOldCLIObservation{Source: completionOldCLISource, Argv: append([]string{v.file}, args...), Termination: "unknown", Stdout: []byte{}, Stderr: []byte{}}
	before, e := v.currentHash()
	if e != nil {
		return out, e
	}
	out.BinaryBefore = before
	command := exec.CommandContext(ctx, v.file, args...)
	command.WaitDelay = queryLimit
	// The invalid-args probe receives no database ENV at all. The real native
	// calls receive only this fixture's explicit two selected database inputs.
	command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GOMAXPROCS=2"}, dbEnv...)
	var stdout, stderr completionOldCLIOutput
	command.Stdout, command.Stderr = &stdout, &stderr
	var broken *os.File
	if closedPipe {
		read, write, e := os.Pipe()
		if e != nil {
			return out, e
		}
		if e := read.Close(); e != nil {
			_ = write.Close()
			return out, e
		}
		broken = write
		command.Stdout = write
	}
	runErr := command.Run()
	var pipeErr error
	if broken != nil {
		pipeErr = broken.Close()
	}
	out.Stdout = append(out.Stdout, stdout.Bytes()...)
	out.Stderr = append(out.Stderr, stderr.Bytes()...)
	out.StdoutHash, out.StderrHash = fingerprint(out.Stdout), fingerprint(out.Stderr)
	if command.ProcessState != nil {
		if wait, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok {
			if wait.Signaled() {
				out.Termination = "signaled"
				out.Signal = int(wait.Signal())
			} else if wait.Exited() {
				out.Termination = "exited"
				code := wait.ExitStatus()
				out.ExitCode = &code
			}
		}
	}
	out.BinaryAfter, e = v.currentHash()
	if e != nil {
		return out, e
	}
	var exited *exec.ExitError
	if ctx.Err() != nil || pipeErr != nil || out.Termination == "unknown" || runErr != nil && !errors.As(runErr, &exited) {
		return out, errors.New("fixed_old_cli_process_or_delivery_unknown")
	}
	return out, nil
}

func logCompletionOldCLI(t *testing.T, o completionOldCLIObservation) {
	t.Helper()
	raw, e := json.Marshal(o)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("owned_fixed_old_cli_process=%s", raw)
}

func validateCompletionOldCLIProbe(o completionOldCLIObservation) error {
	var r receipt
	if o.Termination != "exited" || o.ExitCode == nil || *o.ExitCode != 1 || o.Signal != 0 || len(o.Stderr) != 0 || decodeCaptureDocument(o.Stdout, &r) != nil || r.ToolSource != completionOldCLISource || r.Complete || r.Error != "arguments_invalid" || r.MaterialHash != "" || r.SQLConnectionClosed || r.SQLPoolClosed || r.MongoClosed || r.RuntimeGateAcquired || r.RuntimeGateReleased || r.ApprovalConsumed || r.MarkerWritten || r.DatabaseBoundaryAdvanced || r.RuntimeCompletion != nil {
		return errors.New("fixed_old_cli_compiled_source_or_pre_environment_probe_rejected")
	}
	return nil
}

func (v completionOldCLI) probe(ctx context.Context) (completionOldCLIObservation, error) {
	ctx, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	out, e := v.invoke(ctx, []string{completionOldCLIProbeArgument}, nil, false)
	if e != nil {
		return out, e
	}
	return out, validateCompletionOldCLIProbe(out)
}

func (h *maintenanceNative) invokeCompletionOldCLI(v completionOldCLI, action string, extra []string, closedPipe bool) completionOldCLIObservation {
	h.t.Helper()
	args := []string{"--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m"}
	args = append(args, extra...)
	if action != "capture" {
		args = append(args, "--capture-evidence-file="+h.captureEvidencePath, "--capture-evidence-sha256="+h.captureEvidenceHash)
	}
	ctx, cancel := context.WithTimeout(h.t.Context(), 2*time.Minute+queryLimit)
	defer cancel()
	out, e := v.invoke(ctx, args, []string{"QS_M4_D_OBJECT_MYSQL_DSN=" + h.dsn, "QS_M4_D_OBJECT_MONGO_URI=" + h.uri}, closedPipe)
	logCompletionOldCLI(h.t, out)
	if e != nil {
		h.t.Fatal(e)
	}
	return out
}

func nativeCompletionAcrossActualOldCLI(t *testing.T) {
	old, e := requiredCompletionOldCLI(os.Getenv)
	if e != nil {
		t.Fatal(e)
	}
	probe, e := old.probe(t.Context())
	logCompletionOldCLI(t, probe)
	if e != nil {
		t.Fatal(e)
	}
	h := newMaintenanceNative(t)
	if h.tool == completionOldCLISource {
		t.Fatal("cross-source fixture compiled as the old tool")
	}
	h.materialPath = filepath.Join(h.dir, "actual-old-capture.json")
	capture := h.invokeCompletionOldCLI(old, "capture", []string{"--capture-file=" + h.materialPath}, false)
	var captured receipt
	if capture.Termination != "exited" || capture.ExitCode == nil || *capture.ExitCode != 0 || capture.Signal != 0 || len(capture.Stderr) != 0 || decodeCaptureDocument(capture.Stdout, &captured) != nil || !captured.Complete || captured.ToolSource != completionOldCLISource {
		t.Fatal("actual old capture did not exit0 with its own successful stdout")
	}
	raw, e := readPrivate(h.materialPath, captured.MaterialHash)
	if e != nil || strictDecode(raw, &h.original) != nil || validateMaterial(h.original, completionOldCLISource, h.binding) != nil || h.original.CapturePath != h.materialPath || validateCaptureReceipt(captured, h.original, captured.MaterialHash, completionOldCLISource) != nil {
		t.Fatal("actual old capture/material/release scope was not qualified")
	}
	h.materialHash = captured.MaterialHash
	receiptFile := filepath.Join(h.dir, "actual-old-capture-stdout.json")
	if e := writeExclusive(receiptFile, capture.Stdout); e != nil {
		t.Fatal(e)
	}
	proof := captureExecutionEvidence{Format: 1, Kind: "m4_d_capture_execution", Termination: capture.Termination, ExitCode: capture.ExitCode, ReceiptFile: receiptFile, ReceiptHash: capture.StdoutHash}
	proofRaw, _ := json.Marshal(proof)
	h.captureEvidencePath = filepath.Join(h.dir, "actual-old-capture-execution.json")
	h.captureEvidenceHash = fingerprint(proofRaw)
	if e := writeExclusive(h.captureEvidencePath, proofRaw); e != nil {
		t.Fatal(e)
	}
	if readCaptureEvidence(h.original, h.materialHash, completionOldCLISource, h.captureEvidencePath, h.captureEvidenceHash) != nil {
		t.Fatal("actual old stdout/exit evidence rejected")
	}
	author := *h
	author.tool = completionOldCLISource
	intent := author.runtimeArgs("cancel102", "", "")
	failed := h.invokeCompletionOldCLI(old, "mark-first-runtime", intent, true)
	if failed.Termination == "exited" && (failed.ExitCode == nil || *failed.ExitCode == 0) || failed.Termination != "exited" && failed.Termination != "signaled" || len(failed.Stdout) != 0 {
		t.Fatal("closed actual stdout pipe did not fail the real old process")
	}
	record, e := readRuntimeBoundary(t.Context(), h.db, h.binding)
	if e != nil || record.Record == nil || record.Record.ToolSource != completionOldCLISource || record.Record.MaterialHash != h.materialHash || record.Record.IntentHash != runtimeIntentHashArgument(t, intent) {
		t.Fatal("real old output loss did not leave the exact old committed record")
	}
	marker, e := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
	var oldMarker originalRuntimeMarker
	if e != nil || decodeCaptureDocument(marker, &oldMarker) != nil || oldMarker.ToolSource != completionOldCLISource || oldMarker.Kind != "m4_d_first_runtime_boundary" || oldMarker.Material != h.materialHash || oldMarker.Intent != record.Record.IntentHash {
		t.Fatal("real old process did not leave its complete original marker")
	}
	// These bounded byte logs are the actual old output and marker, not typed
	// re-encodings. The existing native JSONL can reconstruct every raw byte.
	frozen, e := inspectionDirectoryWitness(h.dir)
	if e != nil {
		t.Fatal(e)
	}
	args := h.currentCompletionArgs(intent, "same_existing", "8103-1")
	before, head, identity := h.inspectionSQLWitness(), h.mongoHead(), h.independentBinding()
	result, currentRaw, exit := h.completionInvocation("complete-first-runtime", args, nil)
	h.assertCurrentCompletion(result, exit, false)
	if result.RuntimeCompletion.OriginalToolSource != completionOldCLISource || result.RuntimeCompletion.ToolSource != h.tool || result.DatabaseBoundaryRecord.ToolSource != completionOldCLISource {
		t.Fatal("current confirmation did not distinguish the two actual compiled sources")
	}
	after, e := inspectionDirectoryWitness(h.dir)
	if e != nil {
		t.Fatal(e)
	}
	delete(after, "completion-8103-1.json")
	got, e := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
	if e != nil || !bytes.Equal(marker, got) || before != h.inspectionSQLWitness() || !bytes.Equal(head, h.mongoHead()) || identity != h.independentBinding() || !reflect.DeepEqual(frozen, after) {
		t.Fatal("cross-source confirmation rewrote old raw input/marker or database facts")
	}
	final, e := old.currentHash()
	if e != nil {
		t.Fatal(e)
	}
	rawRecord, _ := json.Marshal(map[string]any{"old_compiled_source": completionOldCLISource, "current_compiled_source": h.tool, "old_binary_final_sha256": final, "old_marker_raw_base64": marker, "old_marker_sha256": fingerprint(marker), "old_record_raw_base64": record.Raw, "old_record_sha256": record.State.Fingerprint, "original_capture_execution_raw_base64": proofRaw, "original_capture_execution_sha256": h.captureEvidenceHash, "current_entry_stdout_base64": currentRaw, "current_entry_stdout_sha256": fingerprint(currentRaw), "current_entry_return": exit, "sql_full_facts_sha256_before_after": before, "mongo40_bson_sha256_before_after": fingerprint(head), "binding_before_after": identity})
	t.Logf("owned_actual_cross_source_confirmation=%s", rawRecord)
}

func TestCompletionOldCLIFixtureRejectsUnboundOrUnsafeExecutable(t *testing.T) {
	for _, name := range []string{"missing_required", "missing_file", "relative", "missing_hash", "wrong_hash", "public_mode", "symlink", "hardlink"} {
		t.Run(name, func(t *testing.T) {
			dir := privateFixture(t)
			p := filepath.Join(dir, "old-cli")
			body := []byte("owned negative executable bytes, never executed")
			if e := os.WriteFile(p, body, 0700); e != nil {
				t.Fatal(e)
			}
			values := map[string]string{"QS_M4_D_COMPLETION_OLD_CLI_REQUIRED": "1", "QS_M4_D_COMPLETION_OLD_CLI_FILE": p, "QS_M4_D_COMPLETION_OLD_CLI_SHA256": fingerprint(body)}
			switch name {
			case "missing_required":
				delete(values, "QS_M4_D_COMPLETION_OLD_CLI_REQUIRED")
			case "missing_file":
				values["QS_M4_D_COMPLETION_OLD_CLI_FILE"] = filepath.Join(dir, "absent")
			case "relative":
				values["QS_M4_D_COMPLETION_OLD_CLI_FILE"] = "old-cli"
			case "missing_hash":
				delete(values, "QS_M4_D_COMPLETION_OLD_CLI_SHA256")
			case "wrong_hash":
				values["QS_M4_D_COMPLETION_OLD_CLI_SHA256"] = fingerprint([]byte("other"))
			case "public_mode":
				if e := os.Chmod(p, 0755); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				link := p + ".link"
				if e := os.Symlink(p, link); e != nil {
					t.Fatal(e)
				}
				values["QS_M4_D_COMPLETION_OLD_CLI_FILE"] = link
			case "hardlink":
				if e := os.Link(p, p+".link"); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := requiredCompletionOldCLI(func(key string) string { return values[key] }); e == nil {
				t.Fatal("unsafe/unbound executable was accepted")
			}
		})
	}
}

func TestCompletionActualOldCLIProbeAndClosedPipeStayBeforeDatabase(t *testing.T) {
	old, e := requiredCompletionOldCLI(os.Getenv)
	if e != nil {
		t.Fatal(e)
	}
	probe, e := old.probe(t.Context())
	logCompletionOldCLI(t, probe)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"source", "exit", "db_opened", "success", "signal"} {
		t.Run(name, func(t *testing.T) {
			copy := probe
			copy.Stdout = append([]byte{}, probe.Stdout...)
			var r receipt
			if decodeCaptureDocument(copy.Stdout, &r) != nil {
				t.Fatal("actual old probe unavailable")
			}
			switch name {
			case "source":
				r.ToolSource = strings.Repeat("b", 40)
			case "exit":
				code := 0
				copy.ExitCode = &code
			case "db_opened":
				r.SQLConnectionClosed = true
			case "success":
				r.Complete = true
			case "signal":
				copy.Termination = "signaled"
				copy.Signal = 13
				copy.ExitCode = nil
			}
			copy.Stdout, _ = json.Marshal(r)
			if validateCompletionOldCLIProbe(copy) == nil {
				t.Fatal("wrong compiled identity/termination/preENV boundary accepted")
			}
		})
	}
	ctx, cancel := context.WithTimeout(t.Context(), queryLimit)
	defer cancel()
	failed, e := old.invoke(ctx, []string{completionOldCLIProbeArgument}, nil, true)
	logCompletionOldCLI(t, failed)
	if e != nil {
		t.Fatal(e)
	}
	if failed.Termination == "exited" && (failed.ExitCode == nil || *failed.ExitCode == 0) || failed.Termination != "exited" && failed.Termination != "signaled" || len(failed.Stdout) != 0 {
		t.Fatal("real closed stdout did not produce an actual old process failure")
	}
}
