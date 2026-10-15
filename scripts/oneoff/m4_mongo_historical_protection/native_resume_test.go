//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestM4MongoObjectNativeInterruptedIndexRestoreResumesWithNewApproval(t *testing.T) {
	for _, clean := range []bool{false, true} {
		state := "Dirty"
		if clean {
			state = "Clean"
		}
		for afterACK := 1; afterACK <= len(targets); afterACK++ {
			t.Run(fmt.Sprintf("%sAfterACK%d", state, afterACK), func(t *testing.T) {
				h := newNative(t)
				docs, metadata, sql := h.frozenDocuments(), h.retainedMetadata(), h.sqlLedger()
				originalIndexes := h.independentIndexes(true)
				h.capture()
				h.upgrade(4, clean)
				head, uuid := h.head()
				args, _, approvalPath := h.approval()

				// Cancel only after the real createIndexes response is successful.
				// CLI remains observer=nil; this is the existing internal test seam.
				ctx, cancel := context.WithCancel(t.Context())
				observer := h.observeIndexes()
				record := observer.completed
				acknowledged := 0
				observer.completed = func(o indexObservation) {
					record(o)
					if o.Class == "success" && o.OK != nil && *o.OK {
						acknowledged++
						if acknowledged == afterACK {
							cancel()
						}
					}
				}
				invocation := append([]string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=2m"}, args...)
				r := run(ctx, invocation, func(key string) string {
					switch key {
					case "QS_M4_OBJECT_MYSQL_DSN":
						return h.dsn
					case "QS_M4_OBJECT_MONGO_URI":
						return h.uri
					default:
						return ""
					}
				}, h.tool, observer)
				cancel()
				requireReleased(t, r)
				if acknowledged != afterACK || r.Complete || r.LedgerCommitIssued || !r.BudgetExceeded {
					t.Fatal("real ACK cancellation did not stop before ledger replay", acknowledged, r)
				}
				pausedHead, pausedUUID := h.head()
				if !bytes.Equal(head, pausedHead) || !bytes.Equal(uuid, pausedUUID) {
					t.Fatal("index interruption changed original actual40 ledger or UUID")
				}
				indexes := h.independentIndexes(true)
				missing := len(targets) - afterACK
				for i, target := range targets {
					raw, present := indexes[target.collection+"/"+target.name]
					if present != (i >= missing) || present && !bytes.Equal(raw, originalIndexes[target.collection+"/"+target.name]) {
						t.Fatal("actual acknowledged index state is not the original-spec suffix", i, missing)
					}
				}
				assertDocuments(t, docs, h.frozenDocuments())
				assertMetadata(t, metadata, h.retainedMetadata())
				if h.sqlLedger() != sql || h.action("verify-target", h.boundInput()...).Complete {
					t.Fatal("partial restoration changed SQL or passed B installation verification")
				}
				if _, e := os.Stat(nativeClaimPath(t, h, nativeApproval(t, approvalPath))); e != nil {
					t.Fatal("interrupted attempt did not consume its actual approval")
				}

				// The consumed approval cannot authorize continuation or any DDL.
				beforeReuse := h.indexEventCount()
				reused := h.action("restore", args...)
				requireReleased(t, reused)
				if reused.Complete || reused.LedgerCommitIssued || h.indexEventCount() != beforeReuse || !reflect.DeepEqual(indexes, h.independentIndexes(true)) {
					t.Fatal("consumed approval was reused or changed indexes", reused)
				}
				afterReuse, afterReuseUUID := h.head()
				if !bytes.Equal(head, afterReuse) || !bytes.Equal(uuid, afterReuseUUID) {
					t.Fatal("consumed approval rejection changed actual40 ledger")
				}

				// Keep the same original material, freeze, recovery decision and
				// absolute deadlines. Only the independent approval/operation is new.
				raw, e := readPrivate(approvalPath, fingerprintMustRead(t, approvalPath), 32768)
				var fresh approval
				if e != nil || strictDecode(raw, &fresh) != nil {
					t.Fatal("original approval unavailable for bounded continuation")
				}
				fresh.Operation = fmt.Sprintf("%d-2", time.Now().UnixNano())
				raw, _ = json.Marshal(fresh)
				freshPath := filepath.Join(h.dir, fresh.Operation+"-approval.json")
				if e := writeExclusive(freshPath, raw); e != nil {
					t.Fatal(e)
				}
				freshArgs := append(h.boundInput(), "--approval-file="+freshPath, "--approval-sha256="+fingerprint(raw))
				resumed := h.action("restore", freshArgs...)
				requireReleased(t, resumed)
				if !resumed.Complete || !resumed.LedgerCommitAcknowledged || resumed.LedgerCommitOutcomeUnknown {
					t.Fatal("fresh independent approval failed to resume original prefix", resumed)
				}
				finalHead, finalUUID := h.head()
				if !bytes.Equal(finalHead, h.original.Ledger.Document) || !bytes.Equal(finalUUID, uuid) || !reflect.DeepEqual(originalIndexes, h.independentIndexes(true)) {
					t.Fatal("continuation lost original BSON/specs or current40 UUID")
				}
				assertDocuments(t, docs, h.frozenDocuments())
				assertMetadata(t, metadata, h.retainedMetadata())
				if h.sqlLedger() != sql || !h.action("inspect").Complete {
					t.Fatal("continuation did not retain SQL and return to fresh10139")
				}
				if _, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes); e != nil {
					t.Fatal("continuation changed the first original material")
				}
				t.Logf("actual createIndexes ACKs=%d then cancellation: missing-prefix=%d; actual40 BSON/UUID retained, old approval refused, new approval resumed with original absolute deadline", acknowledged, missing)
			})
		}
	}
}
