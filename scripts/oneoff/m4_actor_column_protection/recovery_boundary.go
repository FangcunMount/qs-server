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
	CSource            string  `json:"c_execution_source"`
	CResources         string  `json:"c_execution_resources_sha256"`
	CResourceFiles     int     `json:"c_execution_resource_files"`
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
	Format       int      `json:"format_version"`
	Kind         string   `json:"kind"`
	MaterialHash string   `json:"original_material_sha256"`
	First        approval `json:"first_approved_recovery_boundary"`
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
	if a.Format != 1 || a.Kind != "m4_c_pre_start_actor_column_restore" || !operationRE.MatchString(a.Operation) || a.ToolSource != tool || a.CSource != cSource || a.CResources != cResources || a.CResourceFiles != cResourceFiles || a.MaterialHash != materialHash || !nonzeroHash(materialHash) || !nonzeroHash(a.FreezeHash) || !nonzeroHash(a.ResponsibilityHash) || a.Binding != m.Binding || !a.RuntimeFrozen || !a.RuntimeNotStarted || a.RetentionDays < 30 || a.DataOwner != "system (10001)" || a.RecoveryOwner != "system (10001)" {
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
	prefix := filepath.Join(filepath.Dir(m.CapturePath), ".m4-c-"+materialHash)
	return prefix + ".boundary.json", prefix + ".operation-" + fingerprint([]byte(operation)) + ".consumed", nil
}

func readRecoveryBoundary(m material, a approval, materialHash string) (bool, error) {
	path, _, e := recoveryPaths(m, materialHash, a.Operation)
	if e != nil {
		return false, e
	}
	if _, e = os.Lstat(path); os.IsNotExist(e) {
		entries, e := os.ReadDir(filepath.Dir(path))
		if e != nil {
			return false, errors.New("recovery_boundary_state_unknown")
		}
		prefix := strings.TrimSuffix(filepath.Base(path), ".boundary.json") + ".operation-"
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), prefix) {
				return false, errors.New("recovery_boundary_missing_after_claim")
			}
		}
		return false, nil
	} else if e != nil {
		return false, errors.New("recovery_boundary_state_unknown")
	}
	raw, e := readOwnedPrivate(path, 32768)
	var state recoveryBoundary
	if e != nil || strictDecode(raw, &state) != nil || state.Format != 1 || state.Kind != "m4_c_first_recovery_boundary" || state.MaterialHash != materialHash {
		return false, errors.New("recovery_boundary_invalid")
	}
	first := state.First
	first.Operation = a.Operation
	if first != a {
		return false, errors.New("recovery_boundary_scope_changed")
	}
	if validateApproval(state.First, m, materialHash, m.ToolSource, time.Now()) != nil {
		return false, errors.New("original_recovery_boundary_expired_or_started")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	se, ce := f.Sync(), f.Close()
	parent, pe := privateParent(path)
	if pe != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	de, dc := parent.Sync(), parent.Close()
	if se != nil || ce != nil || de != nil || dc != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	if _, e := readPrivate(path, fingerprint(raw)); e != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	if e := verifyRecoveryJournal(m, materialHash, state.First.Operation); e != nil {
		return false, e
	}
	return true, nil
}

func checkRecoveryBoundary(m material, a approval, hash string, required bool) error {
	exists, e := readRecoveryBoundary(m, a, hash)
	if e != nil {
		return e
	}
	if required && !exists {
		return errors.New("recovery_boundary_required")
	}
	return nil
}

func consumeApproval(m material, a approval, hash string) error {
	if !nonzeroHash(hash) || validateApproval(a, m, a.MaterialHash, m.ToolSource, time.Now()) != nil {
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
	if !exists {
		raw, _ := json.Marshal(recoveryBoundary{1, "m4_c_first_recovery_boundary", a.MaterialHash, a})
		if writeExclusive(path, raw) != nil {
			return errors.New("recovery_boundary_persistence_unproven")
		}
	}
	if e := claimRecoveryOperation(m, a, hash, !exists); e != nil {
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
