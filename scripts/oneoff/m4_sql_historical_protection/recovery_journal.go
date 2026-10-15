package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxRecoveryOperations = 128
const maxRecoveryJournalBytes = 64 << 10

type consumedOperation struct {
	Operation    string `json:"operation_id"`
	ApprovalHash string `json:"approval_sha256"`
	MaterialHash string `json:"original_material_sha256"`
}

func recoveryJournalPath(m material, hash string) (string, error) {
	boundary, _, e := recoveryPaths(m, hash, "1-1")
	if e != nil {
		return "", e
	}
	// This name also matches the missing-boundary state scan. Removing the
	// boundary must never turn a retained journal into a new recovery window.
	return strings.TrimSuffix(boundary, ".boundary.json") + ".operation-journal.jsonl", nil
}

func parseRecoveryOperations(raw []byte, materialHash string) ([]consumedOperation, error) {
	if len(raw) == 0 || len(raw) > maxRecoveryJournalBytes || raw[len(raw)-1] != '\n' {
		return nil, errors.New("recovery_operation_journal_invalid_or_lost")
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte{'\n'})
	if len(lines) > maxRecoveryOperations {
		return nil, errors.New("recovery_operation_journal_invalid_or_lost")
	}
	seen := map[string]bool{}
	operations := make([]consumedOperation, 0, len(lines))
	for _, line := range lines {
		var op consumedOperation
		if len(line) > 1024 || strictDecode(line, &op) != nil || !operationRE.MatchString(op.Operation) || !nonzeroHash(op.ApprovalHash) || op.MaterialHash != materialHash || seen[op.Operation] {
			return nil, errors.New("recovery_operation_journal_invalid_or_lost")
		}
		seen[op.Operation] = true
		operations = append(operations, op)
	}
	return operations, nil
}

func verifyRecoveryJournal(m material, materialHash, firstOperation string) error {
	path, e := recoveryJournalPath(m, materialHash)
	if e != nil {
		return e
	}
	raw, e := readOwnedPrivate(path, maxRecoveryJournalBytes)
	operations, parseError := parseRecoveryOperations(raw, materialHash)
	if e != nil || parseError != nil || operations[0].Operation != firstOperation {
		return errors.New("recovery_operation_journal_invalid_or_lost")
	}
	claims := map[string]bool{filepath.Base(path): true}
	for _, op := range operations {
		_, claim, e := recoveryPaths(m, materialHash, op.Operation)
		if e != nil {
			return e
		}
		claims[filepath.Base(claim)] = true
		want, _ := json.Marshal([]string{op.Operation, op.ApprovalHash, op.MaterialHash})
		if _, e := readPrivate(claim, fingerprint(want), 32768); e != nil {
			return errors.New("recovery_operation_journal_invalid_or_lost")
		}
	}
	entries, e := os.ReadDir(filepath.Dir(path))
	if e != nil {
		return errors.New("recovery_operation_journal_invalid_or_lost")
	}
	prefix := strings.TrimSuffix(filepath.Base(path), "journal.jsonl")
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && !claims[entry.Name()] {
			return errors.New("recovery_operation_journal_invalid_or_lost")
		}
	}
	return nil
}

// Only the invocation that just created the immutable first boundary may
// initialize an empty journal. Any later missing/empty journal is lost state.
// The journal and identity claim must both be durable and mutually complete
// before this function can authorize a mutable phase. A partial write is kept.
func claimRecoveryOperation(m material, a approval, hash string, initialize bool) (err error) {
	path, e := recoveryJournalPath(m, a.MaterialHash)
	if e != nil {
		return e
	}
	var before []byte
	flags := os.O_RDWR | os.O_APPEND | syscall.O_NOFOLLOW
	if initialize {
		flags |= os.O_CREATE | os.O_EXCL
	} else {
		before, e = readOwnedPrivate(path, maxRecoveryJournalBytes)
		if e != nil {
			return errors.New("recovery_operation_journal_invalid_or_lost")
		}
	}
	f, e := os.OpenFile(path, flags, 0600)
	if e != nil {
		return errors.New("recovery_operation_journal_open_or_unknown")
	}
	locked := false
	defer func() {
		var unlock error
		if locked {
			unlock = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		}
		closeError := f.Close()
		if unlock != nil || closeError != nil {
			err = errors.New("recovery_operation_journal_release_unknown")
		}
	}()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errors.New("recovery_operation_journal_busy_or_unknown")
	}
	locked = true
	opened, e := f.Stat()
	current, pe := privatePath(path, true)
	if e != nil || pe != nil || !os.SameFile(opened, current) || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 || opened.Sys().(*syscall.Stat_t).Nlink != 1 || (opened.Sys().(*syscall.Stat_t).Uid != 0 && opened.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid())) {
		return errors.New("recovery_operation_journal_invalid_or_lost")
	}
	var operations []consumedOperation
	if initialize {
		if opened.Size() != 0 {
			return errors.New("recovery_operation_journal_invalid_or_lost")
		}
	} else {
		// A different operation may have appended after our first read. Never
		// overwrite or claim from that stale snapshot, even under the new lock.
		afterLock, e := readOwnedPrivate(path, maxRecoveryJournalBytes)
		if e != nil || !bytes.Equal(before, afterLock) || checkRecoveryBoundary(m, a, a.MaterialHash, true) != nil {
			return errors.New("recovery_operation_journal_changed_or_unknown")
		}
		operations, e = parseRecoveryOperations(afterLock, a.MaterialHash)
		if e != nil {
			return e
		}
	}
	for _, op := range operations {
		if op.Operation == a.Operation {
			return errors.New("recovery_operation_consumed_or_unknown")
		}
	}
	if len(operations) >= maxRecoveryOperations {
		return errors.New("recovery_operation_limit_reached")
	}
	record, _ := json.Marshal(consumedOperation{a.Operation, hash, a.MaterialHash})
	record = append(record, '\n')
	if len(before)+len(record) > maxRecoveryJournalBytes {
		return errors.New("recovery_operation_limit_reached")
	}
	n, e := f.Write(record)
	if e != nil || n != len(record) || f.Sync() != nil || syncDirectory(filepath.Dir(path)) != nil {
		return errors.New("recovery_operation_journal_append_unknown")
	}
	_, claim, e := recoveryPaths(m, a.MaterialHash, a.Operation)
	if e != nil {
		return e
	}
	raw, _ := json.Marshal([]string{a.Operation, hash, a.MaterialHash})
	if writeExclusive(claim, raw) != nil {
		return errors.New("recovery_operation_consumed_or_unknown")
	}
	expected := append(append([]byte(nil), before...), record...)
	if _, e := readPrivate(path, fingerprint(expected), maxRecoveryJournalBytes); e != nil || checkRecoveryBoundary(m, a, a.MaterialHash, true) != nil {
		return errors.New("recovery_operation_journal_append_unknown")
	}
	return nil
}

// Read-only verification uses an already claimed independent recovery approval.
// It never creates a boundary, journal record or operation claim.
func checkRestoredAuthorization(m material, a approval, materialPath, materialHash, approvalPath, approvalHash, tool string) error {
	if m.CapturePath != materialPath {
		return errors.New("frozen_material_path_changed_before_verification")
	}
	if _, e := readPrivate(materialPath, materialHash, maxMaterialBytes); e != nil {
		return e
	}
	raw, e := readPrivate(approvalPath, approvalHash, 32768)
	var frozen approval
	if e != nil || strictDecode(raw, &frozen) != nil || frozen != a {
		return errors.New("restore_approval_changed_before_verification")
	}
	path, e := recoveryJournalPath(m, materialHash)
	if e != nil {
		return e
	}
	raw, e = readOwnedPrivate(path, maxRecoveryJournalBytes)
	operations, parseError := parseRecoveryOperations(raw, materialHash)
	if e != nil || parseError != nil {
		return errors.New("recovery_operation_journal_invalid_or_lost")
	}
	for _, op := range operations {
		if op.Operation == a.Operation && op.ApprovalHash == approvalHash {
			return validateMutableBoundary(m, a, materialHash, tool)
		}
	}
	return errors.New("restored_verification_operation_not_claimed")
}
