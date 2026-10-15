//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/go-sql-driver/mysql"
)

// The owned fixture reexecutes this exact test binary to prove that a completed
// process and its released advisory lock are not the durable state authority.
// Production builds contain neither this entry point nor its environment key.
func TestMain(m *testing.M) {
	if os.Getenv("QS_M4_D_OWNED_RUNTIME_PROCESS") == "fixed_native_cli_process" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Exit(serve(context.Background(), os.Args[i+1:], os.Getenv, version.GitCommit, os.Stdout))
			}
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func (h *maintenanceNative) separateRuntimeProcess(action string, extras ...string) receipt {
	h.t.Helper()
	args := []string{"-test.run=^$", "--", "--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m"}
	args = append(args, extras...)
	args = append(args, "--capture-evidence-file="+h.captureEvidencePath, "--capture-evidence-sha256="+h.captureEvidenceHash)
	ctx, cancel := context.WithTimeout(h.t.Context(), 2*time.Minute+queryLimit)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], args...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "QS_M4_D_OWNED_RUNTIME_PROCESS=") && !strings.HasPrefix(value, "QS_M4_D_OBJECT_MYSQL_DSN=") && !strings.HasPrefix(value, "QS_M4_D_OBJECT_MONGO_URI=") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "QS_M4_D_OWNED_RUNTIME_PROCESS=fixed_native_cli_process", "QS_M4_D_OBJECT_MYSQL_DSN="+h.dsn, "QS_M4_D_OBJECT_MONGO_URI="+h.uri)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	e := command.Run()
	code := 0
	if e != nil {
		var exited *exec.ExitError
		if !errors.As(e, &exited) {
			h.t.Fatal("owned CLI process termination unknown")
		}
		code = exited.ExitCode()
	}
	var r receipt
	if stdout.Len() > 65536 || stderr.Len() != 0 || decodeCaptureDocument(stdout.Bytes(), &r) != nil || (code == 0) != r.Complete || ctx.Err() != nil {
		h.t.Fatal("independent actual CLI stdout/exit/release disagrees")
	}
	h.t.Logf("owned independent process exit=%d action=%s complete=%t category=%s", code, action, r.Complete, r.Error)
	return r
}

func (h *maintenanceNative) captureAnotherDirectory() *maintenanceNative {
	h.t.Helper()
	other := *h
	other.dir = privateFixture(h.t)
	other.materialPath, other.materialHash, other.captureEvidencePath, other.captureEvidenceHash = "", "", "", ""
	other.captureStdout, other.captureExit = nil, 1
	other.original = material{}
	other.capture()
	if other.materialPath == h.materialPath || other.binding != h.binding {
		h.t.Fatal("different legal capture did not retain the same database identity")
	}
	return &other
}

func (h *maintenanceNative) independentRuntimeBoundary(generation uint64, materialHash, intentHash string) string {
	h.t.Helper()
	ddl := h.query("SHOW CREATE TABLE `m4_d_runtime_boundary`")
	if len(ddl) != 1 || len(ddl[0]) != 2 || ddl[0][0].String != runtimeBoundaryTable || ddl[0][1].String != runtimeBoundaryDDL {
		h.t.Fatal("actual CTAS SHOW CREATE definition differs from the qualified fixed table")
	}
	rows := h.query("SELECT singleton,generation,HEX(state_sha256),HEX(state_json) FROM `m4_d_runtime_boundary`")
	if len(rows) != 1 || len(rows[0]) != 4 || rows[0][0].String != "1" || rows[0][1].String != strconv.FormatUint(generation, 10) {
		h.t.Fatal("actual singleton or monotonic generation differs")
	}
	raw, e := hex.DecodeString(rows[0][3].String)
	var record runtimeBoundaryRecord
	if e != nil || json.Unmarshal(raw, &record) != nil || record.Generation != generation || record.MaterialHash != materialHash || record.IntentHash != intentHash || record.SQLIdentity != h.binding.SQLIdentity || record.SQLName != h.name || strings.ToLower(rows[0][2].String) != fingerprint(raw) {
		h.t.Fatal("independent persisted identity/material/intent/hash witness differs")
	}
	return fingerprint(raw)
}

func runtimeIntentHashArgument(t *testing.T, args []string) string {
	t.Helper()
	for _, arg := range args {
		if strings.HasPrefix(arg, "--runtime-intent-sha256=") {
			return strings.TrimPrefix(arg, "--runtime-intent-sha256=")
		}
	}
	t.Fatal("independently approved intent hash missing")
	return ""
}

func (h *maintenanceNative) assertStaleRuntimeMaterial(operation string) {
	h.t.Helper()
	args, _, _ := h.restoreInput(operation)
	r := h.separateRuntimeProcess("restore", args...)
	if r.Complete || r.Error != "runtime_database_boundary_changed" || r.ApprovalConsumed || !r.RuntimeGateAcquired || !r.RuntimeGateReleased || r.MarkerWritten {
		h.t.Fatal("a different old capture crossed the database generation before claim", r)
	}
	var a approval
	for _, arg := range args {
		if strings.HasPrefix(arg, "--approval-file=") {
			raw, e := os.ReadFile(strings.TrimPrefix(arg, "--approval-file="))
			if e != nil || strictDecode(raw, &a) != nil {
				h.t.Fatal("owned independent approval unavailable")
			}
		}
	}
	_, claim, e := recoveryPaths(h.original, h.materialHash, a.Operation)
	if e != nil {
		h.t.Fatal(e)
	}
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		h.t.Fatal("stale material consumed its operation before rejection")
	}
}

func TestM4MaintenanceTablesNativeRuntimeGeneration(t *testing.T) {
	t.Run("TwoCapturesAfterIndependentProcessRelease", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		other := h.captureAnotherDirectory()
		h.upgradePrefix(len(scope), true)
		args := h.runtimeArgs("target103", "", "")
		h.assertRuntimeMarker(h.separateRuntimeProcess("mark-first-runtime", args...))
		h.independentRuntimeBoundary(1, h.materialHash, runtimeIntentHashArgument(t, args))
		other.assertStaleRuntimeMaterial("4101-1")
		before := other.original
		if r := other.verify(); r.Complete || r.Error != "target_object_or_identity_rejected" || other.original.RuntimeBoundary != before.RuntimeBoundary {
			t.Fatal("stale material obtained a fresh target certificate")
		}
	})
	for _, mode := range []string{"cancel102", "restored102"} {
		t.Run("NewT0After_"+mode, func(t *testing.T) {
			h := newMaintenanceNative(t)
			before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
			h.capture()
			old := h.captureAnotherDirectory()
			var ap, ah string
			if mode == "restored102" {
				h.upgradePrefix(len(scope), true)
				args, path, hash := h.restoreInput("4102-1")
				ap, ah = path, hash
				h.restoreUsingArgsAndVerify(args, before, modern, head, identity)
			}
			intent := h.runtimeArgs(mode, ap, ah)
			h.assertRuntimeMarker(h.separateRuntimeProcess("mark-first-runtime", intent...))
			hash := h.independentRuntimeBoundary(1, h.materialHash, runtimeIntentHashArgument(t, intent))
			fresh := h.captureAnotherDirectory()
			if fresh.original.RuntimeBoundary != (runtimeBoundaryState{Generation: 1, Fingerprint: hash}) || !reflect.DeepEqual(before, fresh.originalRaw()) || !reflect.DeepEqual(modern, fresh.retainedRaw()) {
				t.Fatal("new clean102 T0 reset generation or changed original/modern facts")
			}
			fresh.upgradePrefix(len(scope), true)
			old.assertStaleRuntimeMaterial("4103-1")
			args, path, approvalHash := fresh.restoreInput("4104-1")
			fresh.restoreUsingArgsAndVerify(args, before, modern, head, identity)
			intent = fresh.runtimeArgs("restored102", path, approvalHash)
			fresh.assertRuntimeMarker(fresh.separateRuntimeProcess("mark-first-runtime", intent...))
			fresh.independentRuntimeBoundary(2, fresh.materialHash, runtimeIntentHashArgument(t, intent))
		})
	}
	for _, state := range []string{"ForeignTable", "ForeignView", "EmptyTable"} {
		t.Run(state, func(t *testing.T) {
			h := newMaintenanceNative(t)
			h.capture()
			query := "CREATE TABLE `m4_d_runtime_boundary` (id bigint)"
			want := "runtime_database_boundary_schema_rejected"
			if state == "ForeignView" {
				query = "CREATE SQL SECURITY INVOKER VIEW `m4_d_runtime_boundary` AS SELECT 1 AS id"
			}
			if state == "EmptyTable" {
				query, want = runtimeBoundaryDDL, "runtime_database_boundary_row_or_release_rejected"
			}
			if _, e := h.db.ExecContext(t.Context(), query); e != nil {
				t.Fatal("owned foreign/empty runtime boundary injection failed")
			}
			h.upgradePrefix(len(scope), true)
			args, _, _ := h.restoreInput("4105-1")
			r := h.action("restore", args...)
			if r.Complete || r.Error != want || r.ApprovalConsumed || r.DatabaseBoundaryAdvanced {
				t.Fatal("unknown/empty object was repaired or used as initial", r)
			}
		})
	}
	t.Run("HiddenBoundaryPermissionRejected", nativeRuntimeBoundaryPermission)
	t.Run("CreateExecuteACKUnknown", nativeRuntimeBoundaryCreateACK)
	t.Run("UpdateCommitACKUnknown", nativeRuntimeBoundaryUpdateACK)
	t.Run("AdvanceThenLocalFailure", nativeRuntimeBoundaryLocalFailure)
	t.Run("CreateMDLDeadline", nativeRuntimeBoundaryMDL)
}

func nativeRuntimeBoundaryPermission(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	other := h.captureAnotherDirectory()
	user := "m4d_b_" + fingerprint([]byte(h.name))[:16]
	password := fingerprint([]byte(h.name + " owned boundary permission"))
	if _, e := h.root.ExecContext(t.Context(), "CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"'"); e != nil {
		t.Fatal("owned restricted boundary principal unavailable")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
		defer cancel()
		if _, e := h.root.ExecContext(ctx, "DROP USER '"+user+"'@'%'"); e != nil {
			t.Error("owned restricted principal release unproved")
		}
	})
	for _, s := range scope {
		if _, e := h.root.ExecContext(t.Context(), "GRANT SELECT,INSERT,CREATE ON `"+h.name+"`.`"+s.name+"` TO '"+user+"'@'%'"); e != nil {
			t.Fatal("owned original table grants unavailable")
		}
	}
	for _, s := range retainedScope {
		if _, e := h.root.ExecContext(t.Context(), "GRANT SELECT ON `"+h.name+"`.`"+s.name+"` TO '"+user+"'@'%'"); e != nil {
			t.Fatal("owned retained table grants unavailable")
		}
	}
	if _, e := h.root.ExecContext(t.Context(), "GRANT SELECT,INSERT,DELETE ON `"+h.name+"`.schema_migrations TO '"+user+"'@'%'"); e != nil {
		t.Fatal("owned ledger grants unavailable")
	}
	intent := h.runtimeArgs("cancel102", "", "")
	h.assertRuntimeMarker(h.action("mark-first-runtime", intent...))
	h.upgradePrefix(len(scope), true)
	cfg, e := mysql.ParseDSN(other.dsn)
	if e != nil {
		t.Fatal("owned selected DSN invalid")
	}
	cfg.User, cfg.Passwd = user, password
	other.dsn = cfg.FormatDSN()
	args, _, _ := other.restoreInput("4106-1")
	r := other.action("restore", args...)
	if r.Complete || r.Error != "runtime_database_boundary_unknown" || r.ApprovalConsumed || r.DatabaseBoundaryAdvanced {
		t.Fatal("a hidden runtime table was mistaken for absent initial state", r)
	}
}

func nativeRuntimeBoundaryCreateACK(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	intent := h.runtimeArgs("cancel102", "", "")
	observed, _, undo := h.useACKTransport("runtime_boundary_create_ack_loss", nil)
	r := h.action("mark-first-runtime", intent...)
	undo()
	if !observed.Load() || r.Complete || r.ReadOnly || r.Error != "runtime_database_boundary_create_outcome_unknown" || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten {
		t.Fatal("real CTAS execution ACK loss was not conservatively closed", r)
	}
	h.independentRuntimeBoundary(1, h.materialHash, runtimeIntentHashArgument(t, intent))
	if retry := h.action("mark-first-runtime", intent...); retry.Complete || retry.Error != "runtime_database_boundary_changed" || retry.DatabaseBoundaryAdvanceUnknown || retry.MarkerWritten {
		t.Fatal("unknown CTAS auto-completed or advanced the same intent twice", retry)
	}
}

func nativeRuntimeBoundaryUpdateACK(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	first := h.runtimeArgs("cancel102", "", "")
	h.assertRuntimeMarker(h.action("mark-first-runtime", first...))
	fresh := h.captureAnotherDirectory()
	intent := fresh.runtimeArgs("cancel102", "", "")
	observed, _, undo := fresh.useACKTransport("runtime_boundary_commit_ack_loss", nil)
	r := fresh.action("mark-first-runtime", intent...)
	undo()
	if !observed.Load() || r.Complete || r.ReadOnly || r.Error != "runtime_database_boundary_commit_outcome_unknown" || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten {
		t.Fatal("actual runtime CAS COMMIT ACK loss granted a startup permit", r)
	}
	fresh.independentRuntimeBoundary(2, fresh.materialHash, runtimeIntentHashArgument(t, intent))
	if retry := fresh.action("mark-first-runtime", intent...); retry.Complete || retry.Error != "runtime_database_boundary_changed" || retry.DatabaseBoundaryAdvanceUnknown || retry.MarkerWritten {
		t.Fatal("unknown CAS advanced the same intent twice", retry)
	}
	inspection := fresh.action("inspect")
	if !inspection.Complete || inspection.DatabaseBoundaryRecord == nil || inspection.DatabaseBoundaryRecord.MaterialHash != fresh.materialHash || inspection.DatabaseBoundaryRecord.IntentHash != runtimeIntentHashArgument(t, intent) {
		t.Fatal("clean102 read-only diagnostic lost the exact committed intent")
	}
}

func nativeRuntimeBoundaryLocalFailure(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	intent := h.runtimeArgs("cancel102", "", "")
	_, _, undo := h.useACKTransport("runtime_boundary_create_marker_failure", func() error {
		return writeExclusive(derivedRuntimeMarker(h.original), []byte("owned local marker interference during SQL advance"))
	})
	r := h.action("mark-first-runtime", intent...)
	undo()
	if r.Complete || !r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown || r.MarkerWritten || r.Error != "first_runtime_marker_unknown_or_started" {
		t.Fatal("committed SQL intent plus local marker failure became a startup permit", r)
	}
	h.independentRuntimeBoundary(1, h.materialHash, runtimeIntentHashArgument(t, intent))
}

func nativeRuntimeBoundaryMDL(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	intent := h.runtimeArgs("cancel102", "", "")
	unlock := h.blockRuntimeDDL()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	done := make(chan receipt, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- h.actionContext(ctx, "mark-first-runtime", intent...)
	}()
	defer func() {
		cancel()
		unlock()
		select {
		case <-finished:
		case <-time.After(queryLimit + time.Second):
			t.Error("owned runtime metadata-wait goroutine release unproved")
		}
	}()
	seen := false
	end := time.Now().Add(queryLimit)
	for time.Now().Before(end) && !seen {
		q, stop := context.WithTimeout(t.Context(), time.Second)
		var count int
		e := h.root.QueryRowContext(q, "SELECT COUNT(DISTINCT th.PROCESSLIST_ID) FROM performance_schema.metadata_locks ml JOIN performance_schema.threads th ON th.THREAD_ID=ml.OWNER_THREAD_ID WHERE ml.LOCK_STATUS='PENDING' AND th.PROCESSLIST_DB=? AND th.PROCESSLIST_INFO LIKE ?", h.name, "CREATE TABLE `m4_d_runtime_boundary` (%").Scan(&count)
		stop()
		if e != nil {
			unlock()
			t.Fatal("owned runtime CTAS metadata wait observation unavailable")
		}
		seen = count == 1
		if !seen {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !seen {
		unlock()
		t.Fatal("actual runtime CTAS did not enter the observed metadata wait")
	}
	<-ctx.Done()
	unlock()
	select {
	case r := <-done:
		if r.Complete || r.ReadOnly || r.MarkerWritten || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced {
			t.Fatal("metadata wait crossed the original deadline into startup", r)
		}
	case <-time.After(queryLimit + time.Second):
		t.Fatal("owned runtime metadata-wait process did not finish")
	}
	if _, e := os.Stat(filepath.Join(h.dir, "first-runtime")); !os.IsNotExist(e) {
		t.Fatal("timed-out runtime CTAS produced a local startup marker")
	}
}
