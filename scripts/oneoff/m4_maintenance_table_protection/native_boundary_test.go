//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Continuations copy the already-frozen first approval. Only the explicit new
// operation changes; no helper creates a new first deadline, marker or scope.
func (h *maintenanceNative) continuation(path, hash, operation string, change func(*approval)) []string {
	h.t.Helper()
	raw, e := readPrivateLimit(path, hash, 32768)
	var a approval
	if e != nil || strictDecode(raw, &a) != nil {
		h.t.Fatal("first independently frozen approval unavailable")
	}
	a.Operation = operation
	if change != nil {
		change(&a)
	}
	raw, e = json.Marshal(a)
	if e != nil {
		h.t.Fatal("owned continuation encoding failed")
	}
	next := filepath.Join(h.dir, "continuation-"+operation+"-"+fingerprint(raw)[:12]+".json")
	if writeExclusive(next, raw) != nil {
		h.t.Fatal("new independently supplied continuation unavailable")
	}
	return []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash, "--approval-file=" + next, "--approval-sha256=" + fingerprint(raw)}
}

func (h *maintenanceNative) boundaryBytes() []byte {
	h.t.Helper()
	path, _, e := recoveryPaths(h.original, h.materialHash, "1-1")
	if e != nil {
		h.t.Fatal("owned boundary path unavailable")
	}
	raw, e := readOwnedPrivate(path, 32768)
	if e != nil {
		h.t.Fatal("actual durable first boundary unavailable")
	}
	return raw
}

func (h *maintenanceNative) assertFailedRestore(r receipt, category string, consumed bool) {
	h.t.Helper()
	h.assertRestoreReleased(r)
	if r.Complete || r.Error != category || r.ApprovalConsumed != consumed || r.ReadOnly {
		h.t.Fatal("bounded restore failure differs", r.Error, r.ApprovalConsumed)
	}
}

func (h *maintenanceNative) assertFailedRestoreGateUnknown(r receipt, category string) {
	h.t.Helper()
	// On a failed wire, gate.release can mark the pinned sql.Conn done before
	// the later Close returns sql.ErrConnDone. Preserve that actual receipt bool;
	// do not require a positive local-close ACK or infer server lock release.
	// SQLPoolClosed still includes the original zero-open/zero-in-use checks.
	if r.Complete || r.Error != category || !r.ApprovalConsumed || r.ReadOnly || !r.SQLPoolClosed || !r.MongoClosed || !r.RuntimeGateAcquired || r.RuntimeGateReleased || !r.RuntimeGateReleaseUnproven || r.ReleaseError != "runtime_gate_release_unproven" || r.TransactionReleaseUnproven {
		h.t.Fatal("wire loss must retain the original error and unproved server gate", r)
	}
}

// The proxy consumes a real server OK but leaves the client unaware. A fresh
// independent connection witnesses the resulting table before any continuation.
func (h *maintenanceNative) interruptCreate(index int, args []string) {
	h.t.Helper()
	observed, _, undo := h.useACKTransport(fmt.Sprintf("create_%d_ack_loss", index), nil)
	r := h.action("restore", args...)
	undo()
	h.assertFailedRestoreGateUnknown(r, "original_table_create_failed_or_unknown")
	if !observed.Load() {
		h.t.Fatal("real CREATE success ACK was not observed")
	}
	h.assertSQLHead(103, false)
	if len(h.query("SHOW CREATE TABLE `"+scope[index].name+"`")) != 1 {
		h.t.Fatal("successful CREATE was not independently witnessed")
	}
}

func TestM4MaintenanceTablesNativeCreateACKLossContinuation(t *testing.T) {
	for index := range scope {
		t.Run(fmt.Sprintf("create%d", index), func(t *testing.T) {
			h := newMaintenanceNative(t)
			before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
			h.capture()
			h.upgradePrefix(len(scope), true)
			args, ap, hash := h.restoreInput("2001-1")
			h.interruptCreate(index, args)
			first := h.boundaryBytes()
			reused := h.action("restore", args...)
			h.assertFailedRestore(reused, "recovery_operation_consumed_or_unknown", false)
			h.restoreUsingArgsAndVerify(h.continuation(ap, hash, "2001-2", nil), before, modern, head, identity)
			if !bytes.Equal(first, h.boundaryBytes()) {
				t.Fatal("new operation reset the original recovery boundary")
			}
		})
	}
}

func (h *maintenanceNative) seedMultipleRestoreBatches() {
	h.t.Helper()
	var values []string
	var args []any
	for i := 1; i <= 205; i++ {
		values = append(values, "(?,?,?,?,?,?,?,?,?,?)")
		args = append(args, "batch", i, 1, nil, 0, 0, 0, nil, "deferred", "原值😀")
	}
	query := "INSERT INTO testee_store_migration_items(migration_id,testee_id,org_id,before_store_id,target_store_id,before_version,applied_version,history_id,disposition,reason) VALUES" + strings.Join(values, ",")
	if _, e := h.db.ExecContext(h.t.Context(), query, args...); e != nil {
		h.t.Fatal("owned multi-batch original facts unavailable")
	}
}

func TestM4MaintenanceTablesNativeBatchCommitACKLossContinuation(t *testing.T) {
	h := newMaintenanceNative(t)
	h.seedMultipleRestoreBatches()
	before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
	h.capture()
	h.upgradePrefix(len(scope), true)
	args, ap, hash := h.restoreInput("2002-1")
	observed, rollback, undo := h.useACKTransport("items_batch_commit_ack_loss", nil)
	r := h.action("restore", args...)
	undo()
	h.assertFailedRestoreGateUnknown(r, "original_insert_commit_outcome_unknown")
	// ErrTxDone only closes a local Go transaction handle. The positive server
	// COMMIT ACK and an independent 100-row read prove the committed first batch.
	if !observed.Load() || rollback.Load() || h.query("SELECT COUNT(*) FROM testee_store_migration_items")[0][0].String != strconv.Itoa(insertBatchRows) {
		t.Fatal("committed first batch was not independently proved")
	}
	h.assertSQLHead(103, false)
	first := h.boundaryBytes()
	h.restoreUsingArgsAndVerify(h.continuation(ap, hash, "2002-2", nil), before, modern, head, identity)
	if !bytes.Equal(first, h.boundaryBytes()) {
		t.Fatal("batch continuation changed initial scope")
	}
}

func TestM4MaintenanceTablesNativeLedgerCommitACKLossReadOnlyProof(t *testing.T) {
	h := newMaintenanceNative(t)
	before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
	h.capture()
	h.upgradePrefix(len(scope), true)
	args, _, _ := h.restoreInput("2003-1")
	observed, rollback, undo := h.useACKTransport("ledger_commit_ack_loss", nil)
	r := h.action("restore", args...)
	undo()
	h.assertFailedRestoreGateUnknown(r, "ledger_commit_outcome_unknown")
	if !observed.Load() || rollback.Load() {
		t.Fatal("actual successful ledger COMMIT ACK missing or confused with abort")
	}
	h.assertSQLHead(102, false)
	h.assertCaptureRaw(before)
	h.assertRetained(modern)
	h.assertModernReads()
	if !bytes.Equal(head, h.mongoHead()) || identity != h.independentBinding() {
		t.Fatal("independent post-COMMIT identity changed")
	}
	first := h.boundaryBytes()
	proof := h.action("verify-restored", args...)
	h.assertReadOnlyReleased(proof)
	if !proof.Complete || proof.ApprovalConsumed || !proof.MandatoryEOF || !proof.RetainedEOF || !bytes.Equal(first, h.boundaryBytes()) {
		t.Fatal("unknown COMMIT was not resolved solely by fresh read-only proof", proof.Error)
	}
}

func TestM4MaintenanceTablesNativeForeignPartialRejected(t *testing.T) {
	for _, tc := range []struct{ name, mutation, category string }{
		{"foreign_row_key", "INSERT INTO operator_retirement_manifests(migration_id,stage,payload,payload_sha256,created_at,updated_at) VALUES('foreign','applied','{}',REPEAT('a',64),'2026-10-15','2026-10-15')", "restore_partial_table_foreign_values"},
		{"foreign_value", "INSERT INTO operator_retirement_manifests(migration_id,stage,payload,payload_sha256,created_at,updated_at) VALUES('A0','applied_unchanged','{}',REPEAT('a',64),'2026-10-15','2026-10-15')", "restore_partial_table_foreign_values"},
		{"foreign_index", "ALTER TABLE operator_retirement_manifests ADD KEY owned_foreign_stage(stage)", "original_075_078_table_definition_rejected"},
		{"foreign_primary_prefix", "ALTER TABLE operator_retirement_manifests DROP PRIMARY KEY,ADD PRIMARY KEY(migration_id(12))", "fixed_primary_key_or_release_rejected"},
		{"foreign_column", "ALTER TABLE operator_retirement_manifests ADD COLUMN owned_foreign BIGINT NULL", "fixed_column_scope_rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMaintenanceNative(t)
			h.capture()
			h.upgradePrefix(len(scope), true)
			args, ap, hash := h.restoreInput("2004-1")
			h.interruptCreate(4, args)
			if _, e := h.db.ExecContext(t.Context(), tc.mutation); e != nil {
				t.Fatal("owned foreign partial fixture unavailable")
			}
			before := h.query("SHOW CREATE TABLE operator_retirement_manifests")
			rows := h.query("SELECT * FROM operator_retirement_manifests ORDER BY BINARY migration_id")
			first := h.boundaryBytes()
			r := h.action("restore", h.continuation(ap, hash, "2004-2", nil)...)
			h.assertRestoreReleased(r)
			if r.Complete || r.ApprovalConsumed || r.Error != tc.category || !bytes.Equal(first, h.boundaryBytes()) || !reflect.DeepEqual(before, h.query("SHOW CREATE TABLE operator_retirement_manifests")) || !reflect.DeepEqual(rows, h.query("SELECT * FROM operator_retirement_manifests ORDER BY BINARY migration_id")) {
				t.Fatal("foreign partial was accepted, claimed or rewritten", r.Error)
			}
			h.assertSQLHead(103, false)
			if count := h.query("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + h.name + "' AND TABLE_NAME IN ('testee_store_migration_reversions','testee_store_migration_items','testee_store_migration_manifests','clinician_operator_binding_archive')"); count[0][0].String != "0" {
				t.Fatal("rejected continuation created a new table")
			}
		})
	}
}

func TestM4MaintenanceTablesNativeFirstBoundaryCannotReset(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	h.upgradePrefix(len(scope), true)
	args, ap, hash := h.restoreInput("2005-1")
	h.interruptCreate(4, args)
	first := h.boundaryBytes()
	for i, tc := range []struct {
		name   string
		change func(*approval)
	}{
		{"deadline", func(a *approval) { a.Deadline = time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339Nano) }},
		{"marker", func(a *approval) { a.Marker = filepath.Join(h.dir, "different-first-runtime") }},
		{"freeze", func(a *approval) { a.FreezeHash = fingerprint([]byte("new scope cannot reset old freeze")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := h.action("restore", h.continuation(ap, hash, fmt.Sprintf("2005-%d", i+2), tc.change)...)
			want := "recovery_boundary_scope_changed"
			if tc.name == "marker" {
				want = "restore_approval_rejected" // derived marker is now checked before ENV
			}
			if r.Complete || r.ApprovalConsumed || r.Error != want || r.SQLConnectionClosed || r.MongoClosed {
				t.Fatal("new operation reset the first boundary or opened DB", r.Error)
			}
		})
	}
	// Same semantic approval with a different JSON byte encoding still cannot
	// reuse the consumed operation. This reads the retained journal, not a ledger
	// already restored to102, so SQL103 remains a genuine reuse opportunity.
	raw, e := readPrivateLimit(ap, hash, 32768)
	if e != nil {
		t.Fatal("original approval missing")
	}
	var a approval
	if strictDecode(raw, &a) != nil {
		t.Fatal("original approval malformed")
	}
	encoded, e := json.MarshalIndent(a, "", " ")
	path := filepath.Join(h.dir, "renamed-reencoded-approval.json")
	if e != nil || writeExclusive(path, encoded) != nil {
		t.Fatal("owned reencoding fixture unavailable")
	}
	reused := h.action("restore", "--material-file="+h.materialPath, "--material-sha256="+h.materialHash, "--approval-file="+path, "--approval-sha256="+fingerprint(encoded))
	h.assertFailedRestore(reused, "recovery_operation_consumed_or_unknown", false)
	if !bytes.Equal(first, h.boundaryBytes()) {
		t.Fatal("rejected scope reset changed first boundary")
	}
	h.assertSQLHead(103, false)
}

func TestM4MaintenanceTablesNativeCreatePermissionFailure(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	h.upgradePrefix(len(scope), true)
	args, _, _ := h.restoreInput("2006-1")
	user := "m4d_" + fingerprint([]byte(h.name))[:16]
	password := fingerprint([]byte(h.name + "owned password"))
	if _, e := h.root.ExecContext(t.Context(), "CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"'"); e != nil {
		t.Fatal("owned restricted principal unavailable")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
		defer cancel()
		if _, e := h.root.ExecContext(ctx, "DROP USER '"+user+"'@'%'"); e != nil {
			t.Error("owned restricted principal not released")
		}
	})
	if _, e := h.root.ExecContext(t.Context(), "GRANT SELECT,INSERT ON `"+h.name+"`.* TO '"+user+"'@'%'"); e != nil {
		t.Fatal("owned restricted read/insert grants unavailable")
	}
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil {
		t.Fatal("owned DSN invalid")
	}
	cfg.User, cfg.Passwd = user, password
	before := h.dsn
	h.dsn = cfg.FormatDSN()
	r := h.action("restore", args...)
	h.dsn = before
	h.assertFailedRestore(r, "original_table_create_failed_or_unknown", true)
	h.assertSQLHead(103, false)
	if h.query("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + h.name + "' AND TABLE_NAME='operator_retirement_manifests'")[0][0].String != "0" {
		t.Fatal("permission failure changed original table presence")
	}
}

func TestM4MaintenanceTablesNativeLedgerFailureRollsBackAndResumes(t *testing.T) {
	h := newMaintenanceNative(t)
	before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
	h.capture()
	h.upgradePrefix(len(scope), true)
	if _, e := h.db.ExecContext(t.Context(), "CREATE TRIGGER owned_ledger_insert_failure BEFORE INSERT ON schema_migrations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned ledger fault'"); e != nil {
		t.Fatal("owned ledger insert fault unavailable")
	}
	args, ap, hash := h.restoreInput("2007-1")
	r := h.action("restore", args...)
	h.assertFailedRestore(r, "original_ledger_insert_failed", true)
	// Independent server state, rather than local Rollback's return value,
	// proves the original103 row survived the failed final transaction.
	h.assertSQLHead(103, false)
	h.assertOriginalTablesRaw(before)
	h.assertRetained(modern)
	if !reflect.DeepEqual(before, h.originalRaw()) || !reflect.DeepEqual(modern, h.retainedRaw()) {
		t.Fatal("failed ledger transaction changed original or retained raw facts")
	}
	first := h.boundaryBytes()
	if _, e := h.db.ExecContext(t.Context(), "DROP TRIGGER owned_ledger_insert_failure"); e != nil {
		t.Fatal("owned ledger fault not removed")
	}
	h.restoreUsingArgsAndVerify(h.continuation(ap, hash, "2007-2", nil), before, modern, head, identity)
	if !bytes.Equal(first, h.boundaryBytes()) {
		t.Fatal("ledger continuation reset boundary")
	}
}

func TestM4MaintenanceTablesNativeLockWaitMarkerPreventsCommit(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	h.upgradePrefix(len(scope), true)
	args, _, _ := h.restoreInput("2008-1")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	prepared := make(chan *sql.Conn)
	observed, rollback, undo := h.useACKTransport("items_prepare_lock_marker", func() error {
		q, stop := context.WithTimeout(ctx, queryLimit)
		defer stop()
		blocker, e := h.db.Conn(q)
		if e != nil {
			return e
		}
		if _, e = blocker.ExecContext(q, "LOCK TABLES testee_store_migration_items WRITE"); e != nil {
			return errors.Join(e, blocker.Close())
		}
		select {
		case prepared <- blocker:
			return nil
		case <-ctx.Done():
			cleanup, release := context.WithTimeout(context.Background(), queryLimit)
			_, unlockErr := blocker.ExecContext(cleanup, "UNLOCK TABLES")
			closeErr := blocker.Close()
			release()
			return errors.Join(ctx.Err(), unlockErr, closeErr)
		}
	})
	defer undo()
	observation := make(chan error, 1)
	go func() {
		var blocker *sql.Conn
		select {
		case blocker = <-prepared:
		case <-ctx.Done():
			observation <- errors.New("owned_insert_prepare_not_reached")
			return
		}
		// This observer uses a separate owned pool: h.db's sole connection holds the
		// real metadata lock. It never logs SQL bodies or interprets ErrTxDone as ACK.
		observeErr := func() error {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) && ctx.Err() == nil {
				q, stop := context.WithTimeout(ctx, time.Second)
				var pending int
				e := h.root.QueryRowContext(q, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE DB=? AND STATE='Waiting for table metadata lock' AND INFO LIKE 'INSERT INTO `testee_store_migration_items`%'", h.name).Scan(&pending)
				stop()
				if e != nil {
					return errors.New("owned_pending_observation_unavailable")
				}
				if pending == 1 {
					return writeExclusive(filepath.Join(h.dir, "first-runtime"), []byte("owned first-runtime command issued"))
				}
				time.Sleep(10 * time.Millisecond)
			}
			return errors.New("actual_owned_lock_wait_unproved")
		}()
		q, stop := context.WithTimeout(context.Background(), queryLimit)
		_, unlockErr := blocker.ExecContext(q, "UNLOCK TABLES")
		closeErr := blocker.Close()
		stop()
		observation <- errors.Join(observeErr, unlockErr, closeErr)
	}()
	r := h.actionContext(ctx, "restore", args...)
	cancel()
	select {
	case e := <-observation:
		if e != nil {
			t.Fatal("actual lock wait, marker or blocker release unproved")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("owned lock observer did not release within bound")
	}
	undo()
	h.assertFailedRestore(r, "first_runtime_marker_unknown_or_started", true)
	if observed.Load() || !rollback.Load() || h.query("SELECT COUNT(*) FROM testee_store_migration_items")[0][0].String != "0" {
		t.Fatal("marker permitted commit or actual ROLLBACK ACK not observed")
	}
	h.assertSQLHead(103, false)
	if _, e := os.Stat(filepath.Join(h.dir, "first-runtime")); e != nil {
		t.Fatal("first-runtime marker was modified")
	}
}
