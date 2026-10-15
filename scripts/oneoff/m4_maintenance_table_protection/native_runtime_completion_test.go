//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
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
	t.Run("ExistingOriginalMarkerAfterOutputLoss", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		intent := h.runtimeArgs("cancel102", "", "")
		_, _, exit := h.completionInvocation("mark-first-runtime", intent, badOutput{})
		if exit != 1 {
			t.Fatal("original stdout delivery failure became success")
		}
		args := h.currentCompletionArgs(intent, "same_existing", "8103-1")
		before := h.inspectionSQLWitness()
		marker, _ := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		r, _, exit := h.completionInvocation("complete-first-runtime", args, nil)
		h.assertCurrentCompletion(r, exit, false)
		got, _ := readOwnedPrivate(derivedRuntimeMarker(h.original), maxRuntimeIntentBytes)
		if !bytes.Equal(marker, got) || before != h.inspectionSQLWitness() {
			t.Fatal("existing original marker was reencoded or database changed")
		}
	})
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
