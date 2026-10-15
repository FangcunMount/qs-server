//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/event"
)

func nativeApproval(t *testing.T, path string) approval {
	t.Helper()
	raw, e := readPrivate(path, fingerprintMustRead(t, path), 32768)
	var a approval
	if e != nil || strictDecode(raw, &a) != nil {
		t.Fatal("independent approval bytes unavailable")
	}
	return a
}

func nativeClaimPath(t *testing.T, h *nativeFixture, a approval) string {
	t.Helper()
	_, path, e := recoveryPaths(h.original, h.materialHash, a.Operation)
	if e != nil {
		t.Fatal(e)
	}
	return path
}

func requireReleasedOrUnopened(t *testing.T, r receipt) {
	t.Helper()
	for _, p := range []*bool{r.SQLConnectionClosed, r.SQLPoolClosed, r.MongoClosed, r.MongoCursorsClosed} {
		if p != nil && !*p {
			t.Fatal("opened resource release failed", r)
		}
	}
	if r.ReleaseError != "" || r.TransactionReleaseUnproven || r.MongoTransactionReleaseUnproven {
		t.Fatal("negative preclaim path left unproven release", r)
	}
}

func TestM4MongoObjectNativeRecoveryApprovalIdentityAndBoundary(t *testing.T) {
	for _, kind := range []string{"CopiedApprovalPath", "ReencodedSameOperation", "ExtendedDeadline", "ChangedDecision", "ChangedMarkerAfterFirstRuntime", "CopiedMaterialPath", "MissingBoundaryAfterClaim", "CorruptBoundary", "DeletedConsumedClaim"} {
		t.Run(kind, func(t *testing.T) {
			h := newNative(t)
			docs, metadata, sql := h.frozenDocuments(), h.retainedMetadata(), h.sqlLedger()
			h.capture()
			h.upgrade(4, false)
			head, uuid := h.head()
			args, marker, path := h.approval()
			first := nativeApproval(t, path)
			originalRaw, e := readPrivate(path, fingerprintMustRead(t, path), 32768)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(t.Context())
			observer := h.observeIndexes()
			record, acknowledged := observer.completed, 0
			observer.completed = func(o indexObservation) {
				record(o)
				if o.Class == "success" && o.OK != nil && *o.OK {
					acknowledged++
					cancel()
				}
			}
			invocation := append([]string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=2m"}, args...)
			r := run(ctx, invocation, h.connectionInput, h.tool, observer)
			cancel()
			requireReleased(t, r)
			if acknowledged != 1 || r.Complete || r.LedgerCommitIssued || !r.BudgetExceeded {
				h.logIndexEvents(0)
				t.Fatal("first real index ACK did not stop before ledger replay", acknowledged, r)
			}
			paused, pausedUUID := h.head()
			if !bytes.Equal(head, paused) || !bytes.Equal(uuid, pausedUUID) {
				t.Fatal("ACK interruption changed actual40 ledger")
			}
			indexes := h.independentIndexes(true)
			for i, target := range targets {
				_, present := indexes[target.collection+"/"+target.name]
				if present != (i == len(targets)-1) {
					t.Fatal("first ACK did not leave exact missing-prefix3")
				}
			}
			boundary, _, e := recoveryPaths(h.original, h.materialHash, first.Operation)
			if e != nil {
				t.Fatal(e)
			}
			claim := nativeClaimPath(t, h, first)
			claimHash := fingerprintMustRead(t, claim)
			next, approvalRaw, materialPath := first, originalRaw, h.materialPath
			next.Operation = fmt.Sprintf("%d-2", time.Now().UnixNano())
			wantError := "recovery_boundary_scope_changed"
			switch kind {
			case "CopiedApprovalPath":
				next = first
				wantError = "recovery_operation_consumed_or_unknown"
			case "ReencodedSameOperation":
				next = first
				approvalRaw, _ = json.MarshalIndent(next, "", "  ")
				if fingerprint(approvalRaw) == fingerprint(originalRaw) {
					t.Fatal("reencoding did not change raw approval")
				}
				wantError = "recovery_operation_consumed_or_unknown"
			case "ExtendedDeadline":
				d, _ := time.Parse(time.RFC3339Nano, first.Deadline)
				next.Deadline = d.Add(time.Second).Format(time.RFC3339Nano)
			case "ChangedDecision":
				d, _ := time.Parse(time.RFC3339Nano, first.RecoveryDecision)
				next.RecoveryDecision = d.Add(time.Millisecond).Format(time.RFC3339Nano)
			case "ChangedMarkerAfterFirstRuntime":
				if writeExclusive(marker, []byte("first real runtime command intent")) != nil {
					t.Fatal("original marker fault unavailable")
				}
				next.Marker = filepath.Join(h.dir, "different-absent-runtime-marker")
			case "CopiedMaterialPath":
				raw, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes)
				materialPath = filepath.Join(h.dir, "copied-material.json")
				if e != nil || writeExclusive(materialPath, raw) != nil {
					t.Fatal("material copy fault unavailable")
				}
				wantError = "material_rejected"
			case "MissingBoundaryAfterClaim":
				if os.Remove(boundary) != nil {
					t.Fatal("owned boundary fault unavailable")
				}
				wantError = "recovery_boundary_missing_after_claim"
			case "CorruptBoundary":
				if os.WriteFile(boundary, []byte("{"), 0600) != nil {
					t.Fatal("owned boundary fault unavailable")
				}
				wantError = "recovery_boundary_invalid"
			case "DeletedConsumedClaim":
				if os.Remove(claim) != nil {
					t.Fatal("owned consumed claim deletion unavailable")
				}
				next = first
				wantError = "recovery_operation_journal_invalid_or_lost"
			}
			if kind != "CopiedApprovalPath" && kind != "ReencodedSameOperation" {
				approvalRaw, _ = json.Marshal(next)
			}
			nextPath := filepath.Join(h.dir, "copied-or-new-approval.json")
			if writeExclusive(nextPath, approvalRaw) != nil {
				t.Fatal("independent negative approval unavailable")
			}
			negativeArgs := []string{"--material-file=" + materialPath, "--material-sha256=" + h.materialHash, "--approval-file=" + nextPath, "--approval-sha256=" + fingerprint(approvalRaw)}
			beforeEvents := h.indexEventCount()
			rejected := h.action("restore", negativeArgs...)
			requireReleasedOrUnopened(t, rejected)
			if rejected.Complete || rejected.Error != wantError || rejected.LedgerCommitIssued || rejected.LedgerAbortCalled || h.indexEventCount() != beforeEvents {
				t.Fatal("moved/reset recovery authority issued DDL or reached ledger mutation", rejected)
			}
			if kind == "DeletedConsumedClaim" {
				// Deleting one claim loses recovery state for every operation,
				// not just the operation whose retained journal entry remains.
				fresh := first
				fresh.Operation = fmt.Sprintf("%d-3", time.Now().UnixNano())
				raw, _ := json.Marshal(fresh)
				freshPath := filepath.Join(h.dir, "new-operation-after-deletion.json")
				if writeExclusive(freshPath, raw) != nil {
					t.Fatal("new negative operation unavailable")
				}
				freshArgs := append(h.boundInput(), "--approval-file="+freshPath, "--approval-sha256="+fingerprint(raw))
				freshRejected := h.action("restore", freshArgs...)
				requireReleasedOrUnopened(t, freshRejected)
				if freshRejected.Complete || freshRejected.Error != wantError || freshRejected.LedgerCommitIssued || freshRejected.LedgerAbortCalled || h.indexEventCount() != beforeEvents {
					t.Fatal("new operation bypassed lost original claim", freshRejected)
				}
				if _, e := os.Stat(nativeClaimPath(t, h, fresh)); !os.IsNotExist(e) {
					t.Fatal("lost journal/claim state consumed a new operation")
				}
			}
			after, afterUUID := h.head()
			if !bytes.Equal(head, after) || !bytes.Equal(uuid, afterUUID) || !reflect.DeepEqual(indexes, h.independentIndexes(true)) || h.sqlLedger() != sql {
				t.Fatal("rejected authority changed original40 state")
			}
			assertDocuments(t, docs, h.frozenDocuments())
			assertMetadata(t, metadata, h.retainedMetadata())
			if kind == "DeletedConsumedClaim" {
				if _, e := os.Stat(claim); !os.IsNotExist(e) {
					t.Fatal("deleted operation claim was recreated")
				}
			} else if fingerprintMustRead(t, claim) != claimHash {
				t.Fatal("rejected authority overwrote original operation claim")
			}
			if next.Operation != first.Operation {
				if _, e := os.Stat(nativeClaimPath(t, h, next)); !os.IsNotExist(e) {
					t.Fatal("rejected authority consumed a new operation")
				}
			}
			if _, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes); e != nil {
				t.Fatal("negative recovery changed original capture")
			}
			if kind == "MissingBoundaryAfterClaim" {
				if _, e := os.Stat(boundary); !os.IsNotExist(e) {
					t.Fatal("lost boundary was recreated")
				}
			}
			t.Log("actual ACK1 cancellation retained missing-prefix3; copied/reset authority rejected before new DDL or ledger mutation")
		})
	}
}

func (h *nativeFixture) connectionInput(key string) string {
	switch key {
	case "QS_M4_OBJECT_MYSQL_DSN":
		return h.dsn
	case "QS_M4_OBJECT_MONGO_URI":
		return h.uri
	default:
		return ""
	}
}

func TestM4MongoObjectNativeFirstRuntimeDuringFinalSQLRead(t *testing.T) {
	h := newNative(t)
	h.capture()
	h.upgrade(4, false)
	docs, metadata, sql := h.frozenDocuments(), h.retainedMetadata(), h.sqlLedger()
	head, uuid := h.head()
	args, marker, _ := h.approval()
	p := newNativeWireProxy(t, h, "observe")
	lock, e := h.root.Conn(t.Context())
	if e != nil {
		t.Fatal("owned final-read lock connection unavailable")
	}
	t.Cleanup(func() { _ = lock.Close() })
	done := make(chan error, 1)
	observer := h.observeIndexes()
	previous := observer.monitor.Succeeded
	observer.monitor.Succeeded = func(ctx context.Context, e *event.CommandSucceededEvent) {
		previous(ctx, e)
		if e.CommandName != "insert" {
			return
		}
		q, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		_, err := lock.ExecContext(q, "LOCK TABLES `"+h.name+"`.schema_migrations WRITE")
		cancel()
		if err != nil {
			done <- errors.New("owned final-read lock failed")
			return
		}
		go func() {
			q, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			var fault error
			for {
				var pending int
				if h.db.QueryRowContext(q, "SELECT COUNT(*) FROM information_schema.processlist WHERE DB=? AND STATE='Waiting for table metadata lock' AND INFO=?", h.name, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2").Scan(&pending) != nil {
					fault = errors.New("final SQL wait observation failed")
					break
				}
				if pending == 1 {
					fault = writeExclusive(marker, []byte("first runtime intent during actual final SQL read"))
					break
				}
				select {
				case <-q.Done():
					fault = errors.New("final SQL read never reached owned lock")
				case <-time.After(10 * time.Millisecond):
					continue
				}
				break
			}
			unq, uncancel := context.WithTimeout(context.Background(), queryLimit)
			_, unlockError := lock.ExecContext(unq, "UNLOCK TABLES")
			uncancel()
			if fault == nil && unlockError != nil {
				fault = errors.New("owned final SQL lock release failed")
			}
			done <- fault
		}()
	}
	invocation := append([]string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=2m"}, args...)
	r := run(t.Context(), invocation, h.connectionInput, h.tool, observer)
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(queryLimit):
		t.Fatal("final SQL marker witness did not finish")
	}
	if lock.Close() != nil {
		t.Fatal("owned final SQL lock connection release failed")
	}
	requirePhysicalReleased(t, r)
	if r.Complete || r.LedgerCommitIssued || !r.LedgerAbortCalled || !r.MongoTransactionReleaseUnproven || r.Error != "first_runtime_already_started_or_marker_unknown" {
		t.Fatal("marker issued during final SQL read did not prevent commit", r)
	}
	p.mu.Lock()
	commits, aborts := p.commitRequests, p.successfulAbortReplies
	p.mu.Unlock()
	if commits != 0 || aborts < 1 {
		t.Fatal("actual abort/no-COMMIT witness missing", commits, aborts)
	}
	after, afterUUID := h.head()
	if !bytes.Equal(head, after) || !bytes.Equal(uuid, afterUUID) || h.sqlLedger() != sql {
		t.Fatal("late marker exposed transaction writes")
	}
	assertDocuments(t, docs, h.frozenDocuments())
	assertMetadata(t, metadata, h.retainedMetadata())
	h.assertOriginalTargetSpecs()
	t.Log("actual final SQL SELECT waited on owned metadata lock; original marker created before release; zero COMMIT requests, upstream abort acknowledged, public transaction release remains unproven")
}
