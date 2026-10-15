package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestVerifyRestoredRequiresClaimedOriginalApprovalBeforeEnvironment(t *testing.T) {
	for _, fault := range []string{"no_boundary", "new_operation", "reencoded_approval", "missing_claim", "missing_boundary", "missing_journal", "started_marker", "extended_deadline"} {
		t.Run(fault, func(t *testing.T) {
			m := fixtureMaterial(t)
			a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
			proof := fixtureCaptureEvidence(t, m, mh)
			if fault != "no_boundary" && consumeApproval(m, a, ah) != nil {
				t.Fatal("owned original recovery state unavailable")
			}
			boundary, claim, _ := recoveryPaths(m, mh, a.Operation)
			journal, _ := recoveryJournalPath(m, mh)
			switch fault {
			case "new_operation", "reencoded_approval", "extended_deadline":
				switch fault {
				case "new_operation":
					a.Operation = "124-1"
				case "extended_deadline":
					a.Deadline = a.Expires
				}
				raw, _ := json.MarshalIndent(a, "", "  ")
				ap = filepath.Join(filepath.Dir(mp), "different-approval.json")
				if writeExclusive(ap, raw) != nil {
					t.Fatal("owned approval mutation unavailable")
				}
				ah = fingerprint(raw)
			case "missing_claim", "missing_boundary", "missing_journal":
				path := map[string]string{"missing_claim": claim, "missing_boundary": boundary, "missing_journal": journal}[fault]
				if os.Remove(path) != nil {
					t.Fatal("owned recovery state loss unavailable")
				}
			case "started_marker":
				if writeExclusive(a.Marker, []byte("first-runtime intent")) != nil {
					t.Fatal("owned first-runtime marker unavailable")
				}
			}
			before, _ := os.ReadFile(journal)
			args := append([]string{"--action=verify-restored", "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + mp, "--material-sha256=" + mh, "--approval-file=" + ap, "--approval-sha256=" + ah}, captureEvidenceArgs(proof)...)
			r := run(context.Background(), args, func(string) string {
				t.Fatal("connection input read before original consumed recovery state proved")
				return ""
			}, testSource, nil)
			if r.Complete || !r.ReadOnly || r.Error == "none" || r.ReleaseError != "" || r.SQLConnectionClosed != nil || r.SQLPoolClosed != nil || r.MongoClosed != nil || r.TransactionReleaseUnproven || r.MongoTransactionReleaseUnproven || r.RestoredVerification != nil {
				t.Fatal("unproved verification opened resources or returned positive evidence", r)
			}
			after, _ := os.ReadFile(journal)
			if !bytes.Equal(before, after) {
				t.Fatal("read-only verification changed retained journal bytes")
			}
			if fault == "no_boundary" {
				for _, path := range []string{boundary, claim, journal} {
					if _, e := os.Stat(path); !os.IsNotExist(e) {
						t.Fatal("verification initialized authorization state")
					}
				}
			}
			if fault == "new_operation" {
				_, freshClaim, _ := recoveryPaths(m, mh, a.Operation)
				if _, e := os.Stat(freshClaim); !os.IsNotExist(e) {
					t.Fatal("verification consumed a new operation")
				}
			}
		})
	}
}

func TestVerifyRestoredAuthorizationBorrowsExactConsumedBytesWithoutMutation(t *testing.T) {
	m := fixtureMaterial(t)
	a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
	proof := fixtureCaptureEvidence(t, m, mh)
	if consumeApproval(m, a, ah) != nil {
		t.Fatal("owned first recovery state unavailable")
	}
	boundary, claim, _ := recoveryPaths(m, mh, a.Operation)
	journal, _ := recoveryJournalPath(m, mh)
	before := map[string]string{}
	for _, path := range []string{boundary, claim, journal, mp, ap} {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		before[path] = fingerprint(raw)
	}
	if e := checkRestoredAuthorization(m, a, mp, mh, ap, ah, testSource, proof); e != nil {
		t.Fatal("original consumed approval refused", e)
	}
	for path, hash := range before {
		raw, e := os.ReadFile(path)
		if e != nil || fingerprint(raw) != hash {
			t.Fatal("verification changed retained material or authorization state")
		}
	}
	raw, _ := json.MarshalIndent(a, "", "  ")
	copyPath := filepath.Join(filepath.Dir(ap), "different-encoding.json")
	if writeExclusive(copyPath, raw) != nil || checkRestoredAuthorization(m, a, mp, mh, copyPath, fingerprint(raw), testSource, proof) == nil {
		t.Fatal("new raw approval bytes bypassed consumed hash")
	}
}

func TestVerifyOriginalRestoredSnapshotsRequiresCompleteOriginalFactsAndStableCurrentUUID(t *testing.T) {
	m := fixtureMaterial(t)
	restored := func() observation {
		o := currentState(t, m, 0, true)
		o.Ledger.Document = append([]byte(nil), m.Ledger.Document...)
		return o
	}
	if e := verifyOriginalRestoredSnapshots(m, restored(), restored()); e != nil {
		t.Fatal(e)
	}
	for _, fault := range []string{"original39_uuid", "current_uuid_changed", "original_bson_type", "dirty39", "ledger_options", "ledger_index", "sql_dirty", "sql_version", "sql_identity", "mongo_identity", "collection_uuid", "collection_options", "retained_index", "target_spec", "count", "missing_count", "first_snapshot_drift"} {
		t.Run(fault, func(t *testing.T) {
			before, after := restored(), restored()
			switch fault {
			case "original39_uuid":
				before.Ledger.Namespace.UUID = m.Ledger.Namespace.UUID
				after.Ledger.Namespace.UUID = m.Ledger.Namespace.UUID
			case "current_uuid_changed":
				after.Ledger.Namespace.UUID = bytes.Repeat([]byte{8}, 16)
			case "original_bson_type":
				after.Ledger.Document = changeSpec(t, after.Ledger.Document, "version", int32(39))
			case "dirty39":
				after.Ledger.Document = changeSpec(t, after.Ledger.Document, "dirty", true)
			case "ledger_options":
				after.Ledger.Namespace.Options = rawBSON(t, bson.D{{Key: "capped", Value: true}})
			case "ledger_index":
				after.Ledger.Namespace.Indexes = append(after.Ledger.Namespace.Indexes, targetSpec(t, 0))
			case "sql_dirty":
				after.SQLDirty = true
			case "sql_version":
				after.SQLVersion = 102
			case "sql_identity":
				after.SQLIdentity = strings.Repeat("c", 64)
			case "mongo_identity":
				after.MongoIdentity = strings.Repeat("c", 64)
			case "collection_uuid":
				after.Collections[0].UUID = bytes.Repeat([]byte{9}, 16)
			case "collection_options":
				after.Collections[0].Options = rawBSON(t, bson.D{{Key: "capped", Value: true}})
			case "retained_index":
				after.Collections[0].Indexes = append(after.Collections[0].Indexes, rawBSON(t, bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "different", Value: int32(1)}}}, {Key: "name", Value: "extra"}}))
			case "target_spec":
				after.Collections[0].Indexes[1] = changeSpec(t, after.Collections[0].Indexes[1], "unique", false)
			case "count":
				n := *after.Collections[0].Count + 1
				after.Collections[0].Count = &n
			case "missing_count":
				after.Collections[0].Count = nil
			case "first_snapshot_drift":
				n := *before.Collections[0].Count + 1
				before.Collections[0].Count = &n
			}
			if verifyOriginalRestoredSnapshots(m, before, after) == nil {
				t.Fatal("changed or incomplete original state accepted")
			}
		})
	}
}

func TestVerifyRestoredArgumentsRequireOriginalApproval(t *testing.T) {
	base := []string{"--action=verify-restored", "--mysql-database=qs", "--mongo-database=qs", "--material-file=/private/original.json", "--material-sha256=" + strings.Repeat("a", 64)}
	base = append(base, "--capture-evidence-file=/private/capture-execution.json", "--capture-evidence-sha256="+strings.Repeat("c", 64))
	if _, e := parseArgs(base); e == nil {
		t.Fatal("verification accepted no independent approval")
	}
	args := append(base, "--approval-file=/private/original-approval.json", "--approval-sha256="+strings.Repeat("b", 64))
	if _, e := parseArgs(args); e != nil {
		t.Fatal("fixed verification inputs refused", e)
	}
	if _, e := parseArgs(append(args, "--capture-file=/private/replacement.json")); e == nil {
		t.Fatal("verification accepted capture output")
	}
}
