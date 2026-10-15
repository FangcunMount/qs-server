package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type approval struct {
	Format             int     `json:"format_version"`
	Kind               string  `json:"kind"`
	Operation          string  `json:"operation_id"`
	ToolSource         string  `json:"compiled_tool_source"`
	DSource            string  `json:"d_execution_source"`
	DResources         string  `json:"d_execution_resources_sha256"`
	DResourceFiles     int     `json:"d_execution_resource_files"`
	MaterialHash       string  `json:"independently_frozen_material_sha256"`
	FreezeHash         string  `json:"complete_writer_freeze_receipt_sha256"`
	ResponsibilityHash string  `json:"responsibility_receipt_sha256"`
	Binding            binding `json:"binding"`
	RuntimeFrozen      bool    `json:"all_six_instances_and_other_writers_frozen"`
	RuntimeNotStarted  bool    `json:"first_runtime_command_not_issued"`
	Marker             string  `json:"first_runtime_marker"`
	WindowStarted      string  `json:"window_started_at"`
	RecoveryDecision   string  `json:"recovery_decision_at"`
	Deadline           string  `json:"restore_deadline"`
	Expires            string  `json:"expires_at"`
	RetentionDays      int     `json:"protection_retention_days"`
	DataOwner          string  `json:"data_owner"`
	RecoveryOwner      string  `json:"recovery_owner"`
}

type recoveryBoundary struct {
	InitialLedger  ledgerRow `json:"initial_sql103_ledger"`
	InitialMissing []bool    `json:"initial_missing_tables_in_drop_order"`
	Format         int       `json:"format_version"`
	Kind           string    `json:"kind"`
	MaterialHash   string    `json:"original_material_sha256"`
	First          approval  `json:"first_approved_recovery_boundary"`
}

func markerAbsent(path string) error {
	if !absolutePath(path) {
		return errors.New("first_runtime_marker_unknown_or_started")
	}
	parent, e := privateParent(path)
	if e != nil {
		return errors.New("first_runtime_marker_unknown_or_started")
	}
	var st unix.Stat_t
	e = unix.Fstatat(int(parent.Fd()), filepath.Base(path), &st, unix.AT_SYMLINK_NOFOLLOW)
	ce := parent.Close()
	if !errors.Is(e, syscall.ENOENT) || ce != nil {
		return errors.New("first_runtime_marker_unknown_or_started")
	}
	return nil
}

func validateApproval(a approval, m material, materialHash, tool string, now time.Time) error {
	if a.Format != 1 || a.Kind != "m4_d_pre_start_maintenance_table_restore" || !operationRE.MatchString(a.Operation) || a.ToolSource != tool || a.DSource != dSource || a.DResources != dResources || a.DResourceFiles != dResourceFiles || a.MaterialHash != materialHash || !nonzeroHash(materialHash) || !nonzeroHash(a.FreezeHash) || !nonzeroHash(a.ResponsibilityHash) || a.Binding != m.Binding || a.Marker != derivedRuntimeMarker(m) || !a.RuntimeFrozen || !a.RuntimeNotStarted || a.RetentionDays < 30 || a.DataOwner != "system (10001)" || a.RecoveryOwner != "system (10001)" {
		return errors.New("restore_approval_binding_rejected")
	}
	start, e1 := time.Parse(time.RFC3339Nano, a.WindowStarted)
	decision, e2 := time.Parse(time.RFC3339Nano, a.RecoveryDecision)
	deadline, e3 := time.Parse(time.RFC3339Nano, a.Deadline)
	expiry, e4 := time.Parse(time.RFC3339Nano, a.Expires)
	captured, e5 := time.Parse(time.RFC3339Nano, m.CapturedAt)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || start.After(captured) || captured.After(decision) || decision.After(now) || !deadline.After(now) || deadline.After(decision.Add(time.Hour)) || !expiry.After(now) || expiry.After(now.Add(24*time.Hour)) {
		return errors.New("restore_approval_budget_rejected")
	}
	return markerAbsent(a.Marker)
}

func recoveryPaths(m material, materialHash, operation string) (boundary, consumed string, err error) {
	if !absolutePath(m.CapturePath) || !nonzeroHash(materialHash) || !operationRE.MatchString(operation) {
		return "", "", errors.New("recovery_anchor_rejected")
	}
	parent, e := privateParent(m.CapturePath)
	if e != nil {
		return "", "", e
	}
	if parent.Close() != nil {
		return "", "", errors.New("recovery_private_parent_release_unproven")
	}
	prefix := filepath.Join(filepath.Dir(m.CapturePath), ".m4-d-"+materialHash)
	return prefix + ".boundary.json", prefix + ".operation-" + fingerprint([]byte(operation)) + ".consumed", nil
}

func readRecoveryBoundary(m material, a approval, materialHash string) (*recoveryBoundary, error) {
	path, _, e := recoveryPaths(m, materialHash, a.Operation)
	if e != nil {
		return nil, e
	}
	if _, e = os.Lstat(path); os.IsNotExist(e) {
		entries, e := os.ReadDir(filepath.Dir(path))
		if e != nil {
			return nil, errors.New("recovery_boundary_state_unknown")
		}
		prefix := strings.TrimSuffix(filepath.Base(path), ".boundary.json") + ".operation-"
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), prefix) {
				return nil, errors.New("recovery_boundary_missing_after_claim")
			}
		}
		return nil, nil
	} else if e != nil {
		return nil, errors.New("recovery_boundary_state_unknown")
	}
	raw, e := readOwnedPrivate(path, 32768)
	var state recoveryBoundary
	if e != nil || strictDecode(raw, &state) != nil || state.Format != 1 || state.Kind != "m4_d_first_recovery_boundary" || state.MaterialHash != materialHash || !validInitialRecoveryState(state.InitialLedger, state.InitialMissing) {
		return nil, errors.New("recovery_boundary_invalid")
	}
	first := state.First
	first.Operation = a.Operation
	if first != a {
		return nil, errors.New("recovery_boundary_scope_changed")
	}
	if validateApproval(state.First, m, materialHash, m.ToolSource, time.Now()) != nil {
		return nil, errors.New("original_recovery_boundary_expired_or_started")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, errors.New("recovery_boundary_durability_unproven")
	}
	se, ce := f.Sync(), f.Close()
	parent, pe := privateParent(path)
	if pe != nil {
		return nil, errors.New("recovery_boundary_durability_unproven")
	}
	de, dc := parent.Sync(), parent.Close()
	if se != nil || ce != nil || de != nil || dc != nil {
		return nil, errors.New("recovery_boundary_durability_unproven")
	}
	if _, e := readPrivate(path, fingerprint(raw)); e != nil {
		return nil, errors.New("recovery_boundary_durability_unproven")
	}
	if e := verifyRecoveryJournal(m, materialHash, state.First.Operation); e != nil {
		return nil, e
	}
	return &state, nil
}

func checkRecoveryBoundary(m material, a approval, hash string, required bool) error {
	exists, e := readRecoveryBoundary(m, a, hash)
	if e != nil {
		return e
	}
	if required && exists == nil {
		return errors.New("recovery_boundary_required")
	}
	return nil
}

func consumeApproval(m material, a approval, hash string, initial ledgerRow, missing []bool) error {
	if !validInitialRecoveryState(initial, missing) || !nonzeroHash(hash) || validateApproval(a, m, a.MaterialHash, m.ToolSource, time.Now()) != nil {
		return errors.New("recovery_claim_rejected")
	}
	exists, e := readRecoveryBoundary(m, a, a.MaterialHash)
	if e != nil {
		return e
	}
	path, _, e := recoveryPaths(m, a.MaterialHash, a.Operation)
	if e != nil {
		return e
	}
	if exists != nil && (exists.InitialLedger != initial || !sameMissing(exists.InitialMissing, missing)) {
		return errors.New("initial_recovery_scope_changed")
	}
	if exists == nil {
		raw, _ := json.Marshal(recoveryBoundary{InitialLedger: initial, InitialMissing: missing, Format: 1, Kind: "m4_d_first_recovery_boundary", MaterialHash: a.MaterialHash, First: a})
		if writeExclusive(path, raw) != nil {
			return errors.New("recovery_boundary_persistence_unproven")
		}
	}
	if e := claimRecoveryOperation(m, a, hash, exists == nil); e != nil {
		return e
	}
	return checkRecoveryBoundary(m, a, a.MaterialHash, true)
}

func validateMutableBoundary(m material, a approval, hash, tool string) error {
	if e := validateApproval(a, m, hash, tool, time.Now()); e != nil {
		return e
	}
	return checkRecoveryBoundary(m, a, hash, true)
}

// First admission accepts only the actual fixed103 DROP prefix. A freshly made
// boundary cannot legitimize a foreign partial table or reset its initial set.
func validInitialRecoveryState(ledger ledgerRow, missing []bool) bool {
	if ledger.Version != 103 || len(missing) != len(scope) {
		return false
	}
	count := 0
	for _, v := range missing {
		if v {
			count++
		} else {
			break
		}
	}
	for _, v := range missing[count:] {
		if v {
			return false
		}
	}
	return ledger.Dirty || count == len(scope)
}

func sameMissing(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
