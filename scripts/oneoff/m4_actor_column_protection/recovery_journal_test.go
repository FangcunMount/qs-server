package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDeletedOperationClaimRefusesSameAndNewOperation(t *testing.T) {
	m, a, mh, _, ah := recoveryInputs(t)
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	boundary, claim, _ := recoveryPaths(m, mh, a.Operation)
	journal, _ := recoveryJournalPath(m, mh)
	before, e := readOwnedPrivate(journal, maxRecoveryJournalBytes)
	if e != nil || os.Remove(claim) != nil {
		t.Fatal("owned claim deletion unavailable")
	}
	for _, operation := range []string{a.Operation, "124-1"} {
		a.Operation = operation
		if consumeApproval(m, a, ah) == nil || validateMutableBoundary(m, a, mh, fixtureSource) == nil {
			t.Fatal("a deleted claim authorized same/new operation")
		}
	}
	after, _ := readOwnedPrivate(journal, maxRecoveryJournalBytes)
	if !bytes.Equal(before, after) {
		t.Fatal("lost claim rewrote original journal")
	}
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		t.Fatal("deleted claim was reconstructed")
	}
	if _, e := readOwnedPrivate(boundary, 32768); e != nil {
		t.Fatal("original immutable boundary lost")
	}
}

func TestMissingPartialOrTruncatedJournalNeverReinitializes(t *testing.T) {
	for _, fault := range []string{"missing", "empty", "partial", "lost_later_record", "missing_boundary"} {
		t.Run(fault, func(t *testing.T) {
			m, a, mh, _, ah := recoveryInputs(t)
			if consumeApproval(m, a, ah) != nil {
				t.Fatal("first durable claim failed")
			}
			journal, _ := recoveryJournalPath(m, mh)
			first, _ := readOwnedPrivate(journal, maxRecoveryJournalBytes)
			a.Operation = "124-1"
			if consumeApproval(m, a, ah) != nil {
				t.Fatal("legitimate bounded continuation failed")
			}
			boundary, _, _ := recoveryPaths(m, mh, a.Operation)
			switch fault {
			case "missing":
				if os.Remove(journal) != nil {
					t.Fatal("owned journal fault unavailable")
				}
			case "empty":
				if os.WriteFile(journal, nil, 0600) != nil {
					t.Fatal("owned journal fault unavailable")
				}
			case "partial":
				if os.WriteFile(journal, append(first, '{'), 0600) != nil {
					t.Fatal("owned journal fault unavailable")
				}
			case "lost_later_record":
				if os.WriteFile(journal, first, 0600) != nil {
					t.Fatal("owned journal fault unavailable")
				}
			case "missing_boundary":
				if os.Remove(boundary) != nil {
					t.Fatal("owned boundary fault unavailable")
				}
			}
			a.Operation = "125-1"
			if consumeApproval(m, a, ah) == nil {
				t.Fatal("lost journal/boundary was silently reset")
			}
			_, claim, _ := recoveryPaths(m, mh, a.Operation)
			if _, e := os.Stat(claim); !os.IsNotExist(e) {
				t.Fatal("lost state consumed a new operation")
			}
			if fault == "missing" {
				if _, e := os.Stat(journal); !os.IsNotExist(e) {
					t.Fatal("missing journal recreated")
				}
			}
		})
	}
}

func TestBoundaryWithoutJournalAndJournalWithoutBoundaryAreLostState(t *testing.T) {
	for _, only := range []string{"boundary", "journal"} {
		t.Run(only, func(t *testing.T) {
			m, a, mh, _, ah := recoveryInputs(t)
			boundary, _, _ := recoveryPaths(m, mh, a.Operation)
			journal, _ := recoveryJournalPath(m, mh)
			if only == "boundary" {
				raw, _ := json.Marshal(recoveryBoundary{1, "m4_c_first_recovery_boundary", mh, a})
				if writeExclusive(boundary, raw) != nil {
					t.Fatal("boundary-only crash fault unavailable")
				}
			} else {
				raw, _ := json.Marshal(consumedOperation{a.Operation, ah, mh})
				if writeExclusive(journal, append(raw, '\n')) != nil {
					t.Fatal("journal-only crash fault unavailable")
				}
			}
			if consumeApproval(m, a, ah) == nil {
				t.Fatal("partial first initialization was treated as fresh")
			}
		})
	}
}

func TestConcurrentJournalLockCannotClaimAndJournalParserIsBounded(t *testing.T) {
	m, a, mh, _, ah := recoveryInputs(t)
	if consumeApproval(m, a, ah) != nil {
		t.Fatal("first durable claim failed")
	}
	journal, _ := recoveryJournalPath(m, mh)
	f, e := os.OpenFile(journal, os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = f.Close() }()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		t.Fatal("owned contention lock unavailable")
	}
	a.Operation = "124-1"
	if claimRecoveryOperation(m, a, ah, false) == nil {
		t.Fatal("concurrent journal mutation acquired authority")
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_UN) != nil {
		t.Fatal("owned contention lock release failed")
	}
	_, claim, _ := recoveryPaths(m, mh, a.Operation)
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		t.Fatal("contended operation created a claim")
	}
	raw, _ := json.Marshal(consumedOperation{a.Operation, ah, mh})
	for _, invalid := range [][]byte{nil, raw, append(raw, '\n', '{'), bytes.Repeat(append(raw, '\n'), maxRecoveryOperations+1), bytes.Repeat([]byte{'x'}, maxRecoveryJournalBytes+1)} {
		if _, e := parseRecoveryOperations(invalid, mh); e == nil {
			t.Fatal("partial, duplicate or unbounded journal accepted")
		}
	}
	if filepath.Dir(journal) != filepath.Dir(m.CapturePath) {
		t.Fatal("journal escaped the original material parent")
	}
}
