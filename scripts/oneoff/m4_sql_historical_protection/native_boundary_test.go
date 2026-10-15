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
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestM4SQLObjectNativeRecoveryBoundaryAttacks(t *testing.T) {
	for _, name := range []string{"CopiedApprovalPath", "ReencodedSameOperation", "ExtendedDeadline", "ChangedDecision", "ChangedMarkerAfterFirstRuntime", "CopiedMaterialPath", "MissingBoundaryAfterClaim", "CorruptBoundary"} {
		t.Run(name, func(t *testing.T) {
			h := newNativeOriginalCharset(t, "utf8")
			h.capture()
			original := h.fullDDL()
			h.upgrade(2, false)
			ledger := readOwnedLedger(t, h)
			args, marker := h.restoreInput()
			action, cancel := context.WithCancel(t.Context())
			defer cancel()
			acks := 0
			r := run(action, append([]string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=1m"}, args...), func(key string) string {
				switch key {
				case "QS_M4_OBJECT_MYSQL_DSN":
					return h.dsn
				case "QS_M4_OBJECT_MONGO_URI":
					return h.uri
				}
				return ""
			}, h.tool, func(index int) {
				if index != 1 {
					t.Error("first actual ADD acknowledgment was not outcome")
				}
				acks++
				cancel()
			})
			if r.Complete || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.TransactionReleaseUnproven || acks != 1 || !reflect.DeepEqual(readOwnedLedger(t, h), ledger) {
				t.Fatal("real acknowledged first ADD interruption/release not proved", r.Error)
			}
			beforeDDL := h.fullDDL()
			if beforeDDL[1] != original[1] || strings.Contains(beforeDDL[0], "`"+objects[0].column+"` json DEFAULT NULL") {
				t.Fatal("first acknowledged ADD did not leave the exact admitted partial shape")
			}
			var approvalPath, approvalHash string
			for _, arg := range args {
				if strings.HasPrefix(arg, "--approval-file=") {
					approvalPath = strings.TrimPrefix(arg, "--approval-file=")
				}
				if strings.HasPrefix(arg, "--approval-sha256=") {
					approvalHash = strings.TrimPrefix(arg, "--approval-sha256=")
				}
			}
			raw, e := readPrivate(approvalPath, approvalHash, 32768)
			var first approval
			if e != nil || strictDecode(raw, &first) != nil {
				t.Fatal("actual first approval unavailable")
			}
			boundary, consumed, e := recoveryPaths(h.material, h.materialHash, first.Operation)
			if e != nil {
				t.Fatal("first recovery anchor unavailable")
			}
			firstClaim, e := readOwnedPrivate(consumed, 32768)
			if e != nil {
				t.Fatal("actual operation was not consumed")
			}
			next := first
			expected := "restore_approval_consumption_failed"
			if name != "CopiedApprovalPath" && name != "ReencodedSameOperation" {
				next.Operation = fmt.Sprintf("%d-2", time.Now().UnixNano())
			}
			materialPath := h.materialPath
			switch name {
			case "ExtendedDeadline":
				deadline, _ := time.Parse(time.RFC3339Nano, first.Deadline)
				next.Deadline = deadline.Add(time.Second).Format(time.RFC3339Nano)
				expected = "recovery_boundary_scope_changed"
			case "ChangedDecision":
				decision, _ := time.Parse(time.RFC3339Nano, first.RecoveryDecision)
				next.RecoveryDecision = decision.Add(time.Millisecond).Format(time.RFC3339Nano)
				expected = "recovery_boundary_scope_changed"
			case "ChangedMarkerAfterFirstRuntime":
				if writeExclusive(marker, []byte("actual first-runtime command intent")) != nil {
					t.Fatal("first-runtime marker fault unavailable")
				}
				next.Marker = filepath.Join(h.dir, "different-absent-marker")
				expected = "recovery_boundary_scope_changed"
			case "CopiedMaterialPath":
				materialPath = filepath.Join(h.dir, "copied-first-material.json")
				body, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes)
				if e != nil || writeExclusive(materialPath, body) != nil {
					t.Fatal("owned identical material copy unavailable")
				}
				expected = "material_rejected"
			case "MissingBoundaryAfterClaim":
				if os.Remove(boundary) != nil {
					t.Fatal("owned boundary deletion fault unavailable")
				}
				expected = "recovery_boundary_missing_after_claim"
			case "CorruptBoundary":
				if os.WriteFile(boundary, []byte("{"), 0600) != nil {
					t.Fatal("owned boundary truncation fault unavailable")
				}
				expected = "recovery_boundary_invalid"
			}
			if name == "ReencodedSameOperation" {
				raw, e = json.MarshalIndent(next, "", "  ")
			} else {
				raw, e = json.Marshal(next)
			}
			attackPath := filepath.Join(h.dir, "attack-approval.json")
			if e != nil || writeExclusive(attackPath, raw) != nil {
				t.Fatal("independently frozen attack fixture unavailable")
			}
			r = h.action("restore", "--material-file="+materialPath, "--material-sha256="+h.materialHash, "--approval-file="+attackPath, "--approval-sha256="+fingerprint(raw))
			if r.Complete || r.Error != expected || r.ReleaseError != "" || r.TransactionReleaseUnproven || h.fullDDL() != beforeDDL || !reflect.DeepEqual(readOwnedLedger(t, h), ledger) {
				t.Fatal("attack changed original recovery boundary or persisted schema/ledger", r.Error)
			}
			if (name == "CopiedApprovalPath" || name == "ReencodedSameOperation") && (!r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed) {
				t.Fatal("opened negative-path clients were not released", r.ReleaseError)
			}
			claim, e := readOwnedPrivate(consumed, 32768)
			if e != nil || !bytes.Equal(firstClaim, claim) {
				t.Fatal("attack changed the first operation claim")
			}
			if name == "MissingBoundaryAfterClaim" {
				if _, e := os.Stat(boundary); !os.IsNotExist(e) {
					t.Fatal("missing boundary was reconstructed after a real operation claim")
				}
			}
			if next.Operation != first.Operation {
				_, path, e := recoveryPaths(h.material, h.materialHash, next.Operation)
				if e != nil {
					t.Fatal("rejected continuation operation path unavailable")
				}
				if _, e := os.Stat(path); !os.IsNotExist(e) {
					t.Fatal("rejected attack consumed a new operation")
				}
			}
			if name == "ChangedMarkerAfterFirstRuntime" {
				if _, e := os.Stat(marker); e != nil {
					t.Fatal("first-runtime marker was removed")
				}
			}
			assertOwnedPartialFactsUnchanged(t, h)
			h.assertHead(101, true)
			t.Logf("real first ADD ACK followed by %s rejected; full retained hashes, actual101 ledger and original Mongo BSON unchanged", name)
		})
	}
}

func assertOwnedPartialFactsUnchanged(t *testing.T, h *nativeFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	q, stop := context.WithTimeout(ctx, queryLimit)
	_, e := h.db.ExecContext(q, "SET SESSION time_zone='+00:00', character_set_results='utf8mb4',sql_mode='"+sessionMode+"'")
	stop()
	if e != nil {
		t.Fatal("owned independent retained-fact session unavailable")
	}
	tx, e := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		t.Fatal("owned independent retained-fact snapshot unavailable")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			t.Error("independent retained-fact snapshot release unproved")
		}
	}()
	for i := range objects {
		columns, present, e := tableColumns(ctx, tx, i)
		if e != nil {
			t.Fatal("owned retained columns unavailable")
		}
		current, e := scanTable(ctx, tx, i, columns, present)
		if e != nil || !current.EOF || !reflect.DeepEqual(current.Columns, h.material.Tables[i].Columns) || !reflect.DeepEqual(current.Rows, h.material.Tables[i].Rows) {
			t.Fatal("attack changed original retained facts or complete coverage", e)
		}
	}
	if e := tx.Rollback(); e != nil {
		t.Fatal("independent retained-fact snapshot rollback failed")
	}
	q, stop = context.WithTimeout(ctx, queryLimit)
	defer stop()
	head, e := h.client.Database(h.name).Collection("schema_migrations").FindOne(q, bson.D{}).Raw()
	if e != nil || !bytes.Equal(head, h.material.MongoHead) {
		t.Fatal("original Mongo ledger BSON changed during rejected SQL recovery")
	}
}
