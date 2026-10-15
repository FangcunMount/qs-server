package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVerifyRestoredRequiresAlreadyClaimedApprovalBeforeEnvironment(t *testing.T) {
	for _, fault := range []string{"no_boundary", "new_operation", "reencoded_approval", "missing_claim", "missing_boundary", "started_marker"} {
		t.Run(fault, func(t *testing.T) {
			m := testMaterial()
			a, mp, mh, ap, ah := boundTestRestoreInputs(t, &m, privateTestDir(t))
			if fault != "no_boundary" && consumeApproval(m, a, ah) != nil {
				t.Fatal("actual private first recovery state unavailable")
			}
			boundary, claim, _ := recoveryPaths(m, mh, a.Operation)
			journal, _ := recoveryJournalPath(m, mh)
			switch fault {
			case "new_operation", "reencoded_approval":
				if fault == "new_operation" {
					a.Operation = "124-1"
				}
				raw, _ := json.MarshalIndent(a, "", "  ")
				ap = filepath.Join(filepath.Dir(mp), "different-independent-approval.json")
				if writeExclusive(ap, raw) != nil {
					t.Fatal("owned approval fixture unavailable")
				}
				ah = fingerprint(raw)
			case "missing_claim":
				if os.Remove(claim) != nil {
					t.Fatal("owned claim loss unavailable")
				}
			case "missing_boundary":
				if os.Remove(boundary) != nil {
					t.Fatal("owned boundary loss unavailable")
				}
			case "started_marker":
				if writeExclusive(a.Marker, []byte("actual first-runtime command intent")) != nil {
					t.Fatal("owned runtime marker unavailable")
				}
			}
			before, _ := os.ReadFile(journal)
			r := run(context.Background(), []string{"--action=verify-restored", "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + mp, "--material-sha256=" + mh, "--approval-file=" + ap, "--approval-sha256=" + ah}, func(string) string {
				t.Fatal("connection input read before existing recovery state was proved")
				return ""
			}, testSource, nil)
			if r.Complete || !r.ReadOnly || r.Error == "none" || r.ReleaseError != "" || r.SQLConnectionClosed || r.SQLPoolClosed || r.MongoClosed || r.TransactionReleaseUnproven {
				t.Fatal("unproved verification was accepted or opened connections", r)
			}
			after, _ := os.ReadFile(journal)
			if !bytes.Equal(before, after) {
				t.Fatal("verification changed the operation journal")
			}
			if fault == "no_boundary" {
				for _, path := range []string{boundary, claim, journal} {
					if _, e := os.Stat(path); !os.IsNotExist(e) {
						t.Fatal("verification initialized recovery state")
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

func TestVerifyRestoredAuthorizationRequiresExactConsumedBytes(t *testing.T) {
	m := testMaterial()
	a, mp, mh, ap, ah := boundTestRestoreInputs(t, &m, privateTestDir(t))
	if consumeApproval(m, a, ah) != nil {
		t.Fatal("actual private first recovery state unavailable")
	}
	journal, _ := recoveryJournalPath(m, mh)
	before, e := readOwnedPrivate(journal, maxRecoveryJournalBytes)
	if e != nil || checkRestoredAuthorization(m, a, mp, mh, ap, ah, testSource) != nil {
		t.Fatal("existing original approval was not accepted", e)
	}
	after, e := readOwnedPrivate(journal, maxRecoveryJournalBytes)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only authorization mutated the retained journal")
	}
	// A different file encoding is not the already consumed raw approval.
	raw, _ := json.MarshalIndent(a, "", "  ")
	copyPath := filepath.Join(filepath.Dir(mp), "same-fields-different-bytes.json")
	if writeExclusive(copyPath, raw) != nil || checkRestoredAuthorization(m, a, mp, mh, copyPath, fingerprint(raw), testSource) == nil {
		t.Fatal("unclaimed raw approval encoding accepted")
	}
}

func TestCompareRestoredRequiresFullOriginalState(t *testing.T) {
	want := testMaterial()
	if compareRestored(want, want) != nil {
		t.Fatal("identical complete original state refused")
	}
	for _, fault := range []string{"ledger_rows", "ledger_ddl", "sql_identity", "mongo_head", "check_encoding", "retained_hash", "missing_eof", "missing_original_object"} {
		t.Run(fault, func(t *testing.T) {
			raw, _ := json.Marshal(want)
			var got material
			if json.Unmarshal(raw, &got) != nil {
				t.Fatal("fixed material copy unavailable")
			}
			switch fault {
			case "ledger_rows":
				got.Ledger[0].Version = 101
			case "ledger_ddl":
				got.LedgerDDL += " "
			case "sql_identity":
				got.SQLIdentity = strings.Repeat("f", 64)
			case "mongo_head":
				got.MongoHead = append(got.MongoHead, 0)
			case "check_encoding":
				got.Tables[0].OriginalDDL = strings.Replace(got.Tables[0].OriginalDDL, "_utf8mb4'", "_utf8mb3'", 1)
			case "retained_hash":
				got.Tables[0].Rows[0].SHA256 = strings.Repeat("f", 64)
			case "missing_eof":
				got.Tables[0].EOF = false
			case "missing_original_object":
				got.Tables[0].SQLNullOnly = false
			}
			if reflect.DeepEqual(want, got) || compareRestored(want, got) == nil {
				t.Fatal("incomplete or changed original state accepted")
			}
		})
	}
}

func TestVerifyRestoredArgumentsRequireOriginalApproval(t *testing.T) {
	base := []string{"--action=verify-restored", "--mysql-database=qs", "--mongo-database=qs", "--material-file=/private/original.json", "--material-sha256=" + strings.Repeat("a", 64)}
	if _, e := parseArgs(base); e == nil {
		t.Fatal("read-only verification accepted no independent approval")
	}
	args := append(base, "--approval-file=/private/original-approval.json", "--approval-sha256="+strings.Repeat("b", 64))
	if _, e := parseArgs(args); e != nil {
		t.Fatal("fixed verification inputs refused", e)
	}
	if _, e := parseArgs(append(args, "--capture-file=/private/replacement.json")); e == nil {
		t.Fatal("verification accepted capture output")
	}
}
