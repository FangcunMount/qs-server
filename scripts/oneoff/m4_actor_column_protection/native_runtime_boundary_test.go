//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/go-sql-driver/mysql"
)

// Only the integration test binary has this subprocess entry. It executes the
// same serve path, then exits and releases its session; no daemon is shared.
func TestMain(m *testing.M) {
	if os.Getenv("QS_M4_C_OWNED_RUNTIME_PROCESS") == "fixed_native_cli_process" {
		if os.Getenv("QS_M4_C_OBJECT_NATIVE_REQUIRED") != "1" || os.Getenv("QS_M4_C_OBJECT_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" {
			os.Exit(2)
		}
		for i, arg := range os.Args {
			if arg == "--" {
				os.Exit(serve(context.Background(), os.Args[i+1:], os.Getenv, version.GitCommit, os.Stdout))
			}
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func (h *actorNative) captureOtherRuntimeDirectory() *actorNative {
	h.t.Helper()
	other := *h
	other.dir = privateRecoveryDir(h.t)
	other.original = material{}
	other.materialPath, other.materialHash, other.captureEvidence, other.captureEvidenceHash = "", "", "", ""
	other.capture()
	if other.materialPath == h.materialPath || other.binding != h.binding {
		h.t.Fatal("different legal capture did not share the same database")
	}
	return &other
}

func (h *actorNative) runtimeArgs(mode string, restoreArgs []string) []string {
	h.t.Helper()
	now := time.Now().UTC()
	v := runtimeIntent{Format: 1, Kind: "m4_c_first_runtime_intent", Operation: strconv.FormatInt(now.UnixNano(), 10) + "-1", ToolSource: h.tool, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, MaterialHash: h.materialHash, CaptureEvidenceHash: h.captureEvidenceHash, CapturePath: h.materialPath, Marker: derivedRuntimeMarker(h.original), Binding: h.binding, Mode: mode, FreezeHash: fingerprint([]byte("owned synthetic writer freeze")), ResponsibilityHash: fingerprint([]byte("owned synthetic responsibility")), WindowStarted: mustTime(h.original.CapturedAt).Add(-time.Second).Format(time.RFC3339Nano), Decision: now.Format(time.RFC3339Nano), Deadline: now.Add(5 * time.Minute).Format(time.RFC3339Nano), Expires: now.Add(10 * time.Minute).Format(time.RFC3339Nano), RuntimeFrozen: true, RuntimeNotStarted: true, AllowMarker: true, ApplyNotDispatched: mode == "cancel101"}
	args := []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash}
	if mode == "restored101" {
		path, hash := "", ""
		for _, a := range restoreArgs {
			if strings.HasPrefix(a, "--approval-file=") {
				path = strings.TrimPrefix(a, "--approval-file=")
			}
			if strings.HasPrefix(a, "--approval-sha256=") {
				hash = strings.TrimPrefix(a, "--approval-sha256=")
			}
		}
		raw, e := readPrivateLimit(path, hash, 32768)
		var ap approval
		if e != nil || strictDecode(raw, &ap) != nil {
			h.t.Fatal("original consumed restore approval unavailable")
		}
		v.WindowStarted, v.Deadline, v.Expires = ap.WindowStarted, ap.Deadline, ap.Expires
		v.FreezeHash, v.ResponsibilityHash, v.RestoreApprovalHash = ap.FreezeHash, ap.ResponsibilityHash, hash
		args = append(args, "--approval-file="+path, "--approval-sha256="+hash)
	}
	raw, _ := json.Marshal(v)
	path := filepath.Join(h.dir, "runtime-intent-"+v.Operation+".json")
	if e := writeExclusive(path, raw); e != nil {
		h.t.Fatal(e)
	}
	return append(args, "--runtime-intent-file="+path, "--runtime-intent-sha256="+fingerprint(raw))
}

func (h *actorNative) runtimeProcess(action string, extras ...string) receipt {
	h.t.Helper()
	args := []string{"-test.run=^$", "--", "--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m", "--capture-evidence-file=" + h.captureEvidence, "--capture-evidence-sha256=" + h.captureEvidenceHash}
	args = append(args, extras...)
	ctx, cancel := context.WithTimeout(h.t.Context(), 2*time.Minute+queryLimit)
	defer cancel()
	c := exec.CommandContext(ctx, os.Args[0], args...)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "QS_M4_C_OWNED_RUNTIME_PROCESS=") && !strings.HasPrefix(v, "QS_M4_C_OBJECT_MYSQL_DSN=") && !strings.HasPrefix(v, "QS_M4_C_OBJECT_MONGO_URI=") {
			c.Env = append(c.Env, v)
		}
	}
	c.Env = append(c.Env, "QS_M4_C_OWNED_RUNTIME_PROCESS=fixed_native_cli_process", "QS_M4_C_OBJECT_MYSQL_DSN="+h.dsn, "QS_M4_C_OBJECT_MONGO_URI="+h.uri)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	e := c.Run()
	code := 0
	if e != nil {
		var exit *exec.ExitError
		if !errors.As(e, &exit) {
			h.t.Fatal("independent process termination unproven")
		}
		code = exit.ExitCode()
	}
	var r receipt
	if stdout.Len() > 65536 || stderr.Len() != 0 || decodeCaptureDocument(stdout.Bytes(), &r) != nil || (code == 0) != r.Complete || ctx.Err() != nil {
		h.t.Fatal("actual independent stdout/exit inconsistent")
	}
	h.t.Logf("owned process action=%s exit=%d complete=%t category=%s", action, code, r.Complete, r.Error)
	return r
}

func requireActorRuntimeReleased(t *testing.T, r receipt) {
	t.Helper()
	if !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || !r.RecoveryGateAcquired || !r.RecoveryGateReleased || r.RecoveryGateReleaseUnproven || r.ReleaseError != "" || r.TransactionReleaseUnproven {
		t.Fatal("runtime boundary resources unproven", r)
	}
}

func (h *actorNative) assertRuntimeMarked(r receipt) {
	h.t.Helper()
	requireActorRuntimeReleased(h.t, r)
	if !r.Complete || r.ReadOnly || r.Action != "mark-first-runtime" || r.Error != "none" || r.ApprovalConsumed || !r.SQLSnapshot || !r.MandatoryEOF || !r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundary == nil || r.DatabaseBoundaryRecord == nil || r.DatabaseBoundary.Generation != h.original.RuntimeBoundary.Generation+1 || !r.MarkerWritten || r.MarkerWriteUnknown || r.MarkerHash != r.DatabaseBoundaryRecord.MarkerHash {
		h.t.Fatal("first-runtime facts not completely established", r)
	}
	raw, e := readPrivateLimit(derivedRuntimeMarker(h.original), r.MarkerHash, maxRuntimeIntentBytes)
	if e != nil || !bytes.Equal(raw, r.DatabaseBoundaryRecord.MarkerRaw) {
		h.t.Fatal("original committed marker bytes differ", e)
	}
}

func (h *actorNative) runtimeRecord(generation uint64) (runtimeBoundaryRecord, string) {
	h.t.Helper()
	ddl := h.query(runtimeBoundaryShow)
	if len(ddl) != 1 || ddl[0][0].String != runtimeBoundaryTable || ddl[0][1].String != runtimeBoundaryDDL {
		h.t.Fatal("actual CTAS SHOW CREATE shape differs")
	}
	rows := h.query("SELECT singleton,generation,HEX(state_sha256),HEX(state_json) FROM `m4_c_runtime_boundary`")
	if len(rows) != 1 || len(rows[0]) != 4 || rows[0][0].String != "1" || rows[0][1].String != strconv.FormatUint(generation, 10) {
		h.t.Fatal("actual singleton generation differs")
	}
	raw, e := hex.DecodeString(rows[0][3].String)
	var record runtimeBoundaryRecord
	if e != nil || strictDecode(raw, &record) != nil || validateRuntimeBoundaryRecord(record, raw, h.binding) != nil || record.MaterialHash != h.materialHash || record.ToolSource != h.tool || strings.ToLower(rows[0][2].String) != fingerprint(raw) {
		h.t.Fatal("actual original boundary bytes/hash rejected")
	}
	return record, fingerprint(raw)
}

func (h *actorNative) assertOldRuntimeMaterialRejected() {
	h.t.Helper()
	ap := h.restoreApproval()
	args := h.approvedRestoreArgs(ap)
	r := h.runtimeProcess("restore", args...)
	requireActorRuntimeReleased(h.t, r)
	if r.Complete || r.Error != "runtime_database_boundary_changed" || r.ApprovalConsumed || r.DatabaseBoundaryAdvanced || r.MarkerWritten {
		h.t.Fatal("old capture crossed durable boundary", r)
	}
	_, claim, e := recoveryPaths(h.original, h.materialHash, ap.Operation)
	if e != nil {
		h.t.Fatal(e)
	}
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		h.t.Fatal("old material consumed its approval")
	}
	if _, e := os.Stat(derivedRuntimeMarker(h.original)); !os.IsNotExist(e) {
		h.t.Fatal("unrelated capture marker was created")
	}
}

// This selector is intentionally distinct from all original30 selectors. It
// must be listed explicitly by the next owned runner/CI qualification task.
func TestM4ActorColumnsNativeRuntimeGeneration(t *testing.T) {
	t.Logf("actual compiled C fixture tool=%s phase=before_dependency_binding", version.GitCommit)
	t.Run("TwoCapturesAfterIndependentProcessExit", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		other := h.captureOtherRuntimeDirectory()
		h.upgradePrefix(2, true)
		h.assertRuntimeMarked(h.runtimeProcess("mark-first-runtime", h.runtimeArgs("target102", nil)...))
		h.runtimeRecord(1)
		other.assertOldRuntimeMaterialRejected()
		if r := other.verify(); r.Complete || r.Error != "target_retained_facts_rejected" {
			t.Fatal("stale capture obtained target certificate", r)
		}
	})
	for _, mode := range []string{"cancel101", "restored101"} {
		t.Run("NewT0After_"+mode, func(t *testing.T) {
			h := newActorNative(t)
			before := h.originalWitness()
			h.capture()
			old := h.captureOtherRuntimeDirectory()
			var restoreArgs []string
			if mode == "restored101" {
				h.upgradePrefix(2, true)
				restoreArgs = h.approvedRestoreArgs(h.restoreApproval())
				requireActorRestoreSuccess(t, h.action("restore", restoreArgs...))
			}
			h.assertRuntimeMarked(h.runtimeProcess("mark-first-runtime", h.runtimeArgs(mode, restoreArgs)...))
			_, hash := h.runtimeRecord(1)
			fresh := h.captureOtherRuntimeDirectory()
			if fresh.original.RuntimeBoundary != (runtimeBoundaryState{Generation: 1, Fingerprint: hash}) {
				t.Fatal("fresh T0 guessed/reset generation")
			}
			fresh.assertOriginalWitness(before)
			fresh.upgradePrefix(2, true)
			old.assertOldRuntimeMaterialRejected()
			args := fresh.approvedRestoreArgs(fresh.restoreApproval())
			requireActorRestoreSuccess(t, fresh.action("restore", args...))
			fresh.assertOriginalWitness(before)
			fresh.assertRuntimeMarked(fresh.runtimeProcess("mark-first-runtime", fresh.runtimeArgs("restored101", args)...))
			fresh.runtimeRecord(2)
		})
	}
	for _, kind := range []string{"ForeignTable", "ForeignView", "EmptyTable", "MalformedRecord"} {
		t.Run(kind, func(t *testing.T) {
			h := newActorNative(t)
			h.capture()
			query := "CREATE TABLE `m4_c_runtime_boundary` (id bigint)"
			want := "runtime_database_boundary_schema_rejected"
			switch kind {
			case "ForeignView":
				query = "CREATE SQL SECURITY INVOKER VIEW `m4_c_runtime_boundary` AS SELECT 1 AS id"
			case "EmptyTable", "MalformedRecord":
				query = runtimeBoundaryDDL
				want = "runtime_database_boundary_row_or_release_rejected"
			}
			if _, e := h.db.ExecContext(t.Context(), query); e != nil {
				t.Fatal("owned foreign state injection unavailable")
			}
			if kind == "MalformedRecord" {
				if _, e := h.db.ExecContext(t.Context(), "INSERT INTO `m4_c_runtime_boundary` VALUES(1,1,UNHEX(SHA2('{}',256)),X'7B7D')"); e != nil {
					t.Fatal(e)
				}
				want = "runtime_database_boundary_row_rejected"
			}
			h.upgradePrefix(2, true)
			r := h.action("restore", h.approvedRestoreArgs(h.restoreApproval())...)
			requireActorRuntimeReleased(t, r)
			if r.Complete || r.Error != want || r.ApprovalConsumed || r.DatabaseBoundaryAdvanced {
				t.Fatal("foreign/empty record repaired or treated as N0", r)
			}
		})
	}
	t.Run("ReadOnlyCaptureAbsentAndCurrent", nativeActorReadOnlyBoundary)
	t.Run("MissingControlPermission", nativeActorHiddenBoundary)
	t.Run("CreateExecuteACKUnknown", nativeActorBoundaryCreateACK)
	t.Run("CASCommitACKUnknown", nativeActorBoundaryCommitACK)
	t.Run("ReleaseACKUnknown", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		args := h.runtimeArgs("cancel101", nil)
		direct := h.dsn
		seen, undo := actorTerminalACKProxy(h, "release_prepare")
		r, raw, exit := actorTerminalAction(h, t.Context(), "mark-first-runtime", args...)
		undo()
		h.dsn = direct
		if exit == 0 || r.Complete || !seen.Load() || !r.DatabaseBoundaryAdvanced || !r.MarkerWritten || r.RecoveryGateReleased || !r.RecoveryGateReleaseUnproven || r.ReleaseError != "recovery_gate_release_unproven" || !r.SQLPoolClosed || !r.MongoClosed {
			t.Fatal("unknown release signed startup fact", r)
		}
		h.runtimeRecord(1)
		if bytes.Contains(raw, []byte(`"complete":true`)) {
			t.Fatal("unknown original receipt changed")
		}
	})
	t.Run("CreateMDLPendingHoldsGateAndDeadline", nativeActorBoundaryMDL)
}

func actorBoundaryReadOnlyUser(h *actorNative, includeControl bool) string {
	h.t.Helper()
	user := "m4c_b_" + fingerprint([]byte(h.name))[:16]
	password := fingerprint([]byte(h.name + " owned boundary"))
	if _, e := h.root.ExecContext(h.t.Context(), "CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"'"); e != nil {
		h.t.Fatal("owned read-only principal unavailable")
	}
	h.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
		defer cancel()
		if _, e := h.root.ExecContext(ctx, "DROP USER '"+user+"'@'%'"); e != nil {
			h.t.Error("owned principal cleanup unproven")
		}
	})
	tables := []string{"schema_migrations", "testee", "operators", "operator_retirement_tasks", "operator_recovery_archives"}
	if includeControl {
		tables = []string{"*"}
	}
	for _, name := range tables {
		target := "`" + name + "`"
		if name == "*" {
			target = "*"
		}
		if _, e := h.root.ExecContext(h.t.Context(), "GRANT SELECT ON `"+h.name+"`."+target+" TO '"+user+"'@'%'"); e != nil {
			h.t.Fatal("owned SELECT grant unavailable")
		}
	}
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil {
		h.t.Fatal(e)
	}
	cfg.User, cfg.Passwd = user, password
	return cfg.FormatDSN()
}

func nativeActorReadOnlyBoundary(t *testing.T) {
	h := newActorNative(t)
	direct := h.dsn
	restricted := actorBoundaryReadOnlyUser(h, true)
	h.dsn = restricted
	h.capture()
	h.dsn = direct
	if rows := h.query("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='m4_c_runtime_boundary'"); len(rows) != 1 || rows[0][0].String != "0" {
		t.Fatal("capture created boundary")
	}
	h.assertRuntimeMarked(h.action("mark-first-runtime", h.runtimeArgs("cancel101", nil)...))
	fresh := h.captureOtherRuntimeDirectory()
	fresh.dsn = restricted
	other := fresh.captureOtherRuntimeDirectory()
	if other.original.RuntimeBoundary.Generation != 1 {
		t.Fatal("SELECT-only capture lost current generation")
	}
}

func nativeActorHiddenBoundary(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	old := h.captureOtherRuntimeDirectory()
	restricted := actorBoundaryReadOnlyUser(h, false)
	h.assertRuntimeMarked(h.action("mark-first-runtime", h.runtimeArgs("cancel101", nil)...))
	h.upgradePrefix(2, true)
	old.dsn = restricted
	r := old.action("restore", old.approvedRestoreArgs(old.restoreApproval())...)
	requireActorRuntimeReleased(t, r)
	if r.Complete || r.Error != "runtime_database_boundary_unknown" || r.ApprovalConsumed || r.DatabaseBoundaryAdvanced {
		t.Fatal("hidden object became initial state", r)
	}
}

func nativeActorBoundaryCreateACK(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	args := h.runtimeArgs("cancel101", nil)
	direct := h.dsn
	seen, undo := actorTerminalACKProxy(h, "runtime_create")
	r, raw, exit := actorTerminalAction(h, t.Context(), "mark-first-runtime", args...)
	undo()
	h.dsn = direct
	if exit == 0 || r.Complete || !seen.Load() || r.Error != "runtime_database_boundary_create_outcome_unknown" || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten || !r.SQLPoolClosed || !r.MongoClosed {
		t.Fatal("actual CTAS execute ACK loss not preserved", r)
	}
	record, _ := h.runtimeRecord(1)
	if len(record.MarkerRaw) == 0 || record.MarkerHash != fingerprint(record.MarkerRaw) {
		t.Fatal("precommit marker bytes missing after unknown response")
	}
	path := filepath.Join(h.dir, "unknown-create-stdout.json")
	if e := writeExclusive(path, raw); e != nil {
		t.Fatal(e)
	}
	if retry := h.action("mark-first-runtime", args...); retry.Complete || retry.Error != "runtime_database_boundary_changed" || retry.DatabaseBoundaryAdvanced || retry.MarkerWritten {
		t.Fatal("unknown CREATE auto-completed/replayed", retry)
	}
	if got, e := readPrivate(path, fingerprint(raw)); e != nil || !bytes.Equal(raw, got) {
		t.Fatal("unknown original receipt overwritten")
	}
	if _, e := os.Stat(derivedRuntimeMarker(h.original)); !os.IsNotExist(e) {
		t.Fatal("unknown CREATE wrote marker")
	}
}

func nativeActorBoundaryCommitACK(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	h.assertRuntimeMarked(h.action("mark-first-runtime", h.runtimeArgs("cancel101", nil)...))
	fresh := h.captureOtherRuntimeDirectory()
	args := fresh.runtimeArgs("cancel101", nil)
	direct := fresh.dsn
	seen, undo := actorTerminalACKProxy(fresh, "runtime_commit")
	r, _, exit := actorTerminalAction(fresh, t.Context(), "mark-first-runtime", args...)
	undo()
	fresh.dsn = direct
	if exit == 0 || r.Complete || !seen.Load() || r.Error != "runtime_database_boundary_commit_outcome_unknown" || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten || !r.SQLPoolClosed || !r.MongoClosed {
		t.Fatal("actual CAS COMMIT ACK loss not preserved", r)
	}
	fresh.runtimeRecord(2)
	if retry := fresh.action("mark-first-runtime", args...); retry.Complete || retry.Error != "runtime_database_boundary_changed" || retry.DatabaseBoundaryAdvanced || retry.MarkerWritten {
		t.Fatal("unknown CAS repeated", retry)
	}
}

func nativeActorBoundaryMDL(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	other := h.captureOtherRuntimeDirectory()
	args := h.runtimeArgs("cancel101", nil)
	blocker, e := h.db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = blocker.ExecContext(t.Context(), "FLUSH TABLES WITH READ LOCK"); e != nil {
		_ = blocker.Close()
		t.Fatal("owned global read lock unavailable")
	}
	var once sync.Once
	unlock := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
			defer cancel()
			_, e := blocker.ExecContext(ctx, "UNLOCK TABLES")
			ce := blocker.Close()
			if e != nil || ce != nil {
				t.Error("owned MDL blocker release unproven")
			}
		})
	}
	t.Cleanup(unlock)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	done := make(chan receipt, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		r, _, _ := actorTerminalAction(h, ctx, "mark-first-runtime", args...)
		done <- r
	}()
	defer func() {
		cancel()
		unlock()
		select {
		case <-finished:
		case <-time.After(queryLimit + time.Second):
			t.Error("owned pending mark did not end")
		}
	}()
	seen := false
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !seen {
		q, stop := context.WithTimeout(ctx, time.Second)
		var count int
		e := h.root.QueryRowContext(q, "SELECT COUNT(DISTINCT th.PROCESSLIST_ID) FROM performance_schema.metadata_locks ml JOIN performance_schema.threads th ON th.THREAD_ID=ml.OWNER_THREAD_ID WHERE ml.LOCK_STATUS='PENDING' AND th.PROCESSLIST_DB=? AND th.PROCESSLIST_INFO LIKE ?", h.name, "CREATE TABLE `m4_c_runtime_boundary` (%").Scan(&count)
		stop()
		if e != nil {
			t.Fatal("owned CTAS PENDING observation unavailable")
		}
		seen = count == 1
		if !seen {
			select {
			case r := <-done:
				t.Fatalf("mark returned before actual MDL witness: %s", r.Error)
			case <-ctx.Done():
				t.Fatal("CTAS never observed PENDING")
			case <-ticker.C:
			}
		}
	}
	blocked := other.action("mark-first-runtime", other.runtimeArgs("cancel101", nil)...)
	if blocked.Complete || blocked.Error != "recovery_gate_busy_or_unavailable" || blocked.DatabaseBoundaryAdvanced || blocked.MarkerWritten {
		t.Fatal("first start crossed pending CTAS gate", blocked)
	}
	cancel()
	unlock()
	select {
	case r := <-done:
		if r.Complete || r.MarkerWritten || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced {
			t.Fatal("deadline/unknown CREATE became startup fact", r)
		}
	case <-time.After(queryLimit + time.Second):
		t.Fatal("pending CREATE did not finish")
	}
	// Independently observe statement completion after unlocking; a client
	// cancellation/closed connection is never described as a server rollback.
	observe, stop := context.WithTimeout(t.Context(), queryLimit)
	defer stop()
	for {
		var active int
		e := h.root.QueryRowContext(observe, "SELECT COUNT(*) FROM performance_schema.processlist WHERE DB=? AND INFO LIKE ?", h.name, "CREATE TABLE `m4_c_runtime_boundary` (%").Scan(&active)
		if e != nil {
			t.Fatal("independent statement end unproven")
		}
		if active == 0 {
			break
		}
		select {
		case <-observe.Done():
			t.Fatal("server CREATE still active")
		case <-ticker.C:
		}
	}
	if _, e := os.Stat(derivedRuntimeMarker(h.original)); !os.IsNotExist(e) {
		t.Fatal("cancelled unknown CREATE wrote marker")
	}
	actual, e := readRuntimeBoundary(observe, h.db, h.binding)
	if e != nil || actual.State.Generation > 1 {
		t.Fatal("independent post-deadline control state unproven", e)
	}
	t.Logf("actual post-deadline boundary generation=%d record_present=%t; client cancellation is not a rollback witness", actual.State.Generation, actual.Record != nil)
}

// This is a local pipe test, not a database execution event. A prepared CTAS
// must lose only its actual Execute ACK, never the earlier Prepare response.
func TestActorRuntimeBoundaryACKObservationIsSelective(t *testing.T) {
	for _, tc := range []struct {
		name, mode, query        string
		command                  byte
		armed, drop, serverError bool
	}{
		{"create_prepare", "runtime_create", "CREATE TABLE `m4_c_runtime_boundary` (singleton tinyint) AS SELECT ?", 22, false, false, false},
		{"create_execute", "runtime_create", "bound-execute-bytes", 23, true, true, false},
		{"unarmed_execute", "runtime_create", "other-execute", 23, false, false, false},
		{"update_prepare", "runtime_commit", "UPDATE `m4_c_runtime_boundary` SET generation=?", 22, false, false, false},
		{"cas_commit", "runtime_commit", "COMMIT", 3, true, true, false},
		{"readonly_commit", "runtime_commit", "COMMIT", 3, false, false, false},
		{"server_error_not_ack", "runtime_create", "bound-execute-bytes", 23, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer func() { _ = client.Close(); _ = server.Close() }()
			if client.SetDeadline(time.Now().Add(queryLimit)) != nil || server.SetDeadline(time.Now().Add(queryLimit)) != nil {
				t.Fatal("bounded local pipe unavailable")
			}
			seen := &atomic.Bool{}
			c := &actorTerminalLossConn{Conn: client, mode: tc.mode, armed: tc.armed, seen: seen}
			body := append([]byte{tc.command}, []byte(tc.query)...)
			request := append([]byte{byte(len(body)), byte(len(body) >> 8), byte(len(body) >> 16), 0}, body...)
			response := []byte{1, 0, 0, 1, 0}
			if tc.serverError {
				response[4] = 0xff
			}
			done := make(chan error, 1)
			go func() {
				got := make([]byte, len(request))
				if _, e := io.ReadFull(server, got); e != nil {
					done <- e
					return
				}
				if !bytes.Equal(got, request) {
					done <- errors.New("request changed")
					return
				}
				_, e := server.Write(response)
				done <- e
			}()
			if _, e := c.Write(request); e != nil {
				t.Fatal(e)
			}
			reply := make([]byte, len(response))
			_, e := io.ReadFull(c, reply)
			if tc.drop {
				if e != io.EOF || seen.Load() == tc.serverError {
					t.Fatal("selected actual ACK classification wrong", e)
				}
			} else if e != nil || seen.Load() || !bytes.Equal(reply, response) {
				t.Fatal("unrelated/Prepare response intercepted", e)
			}
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
