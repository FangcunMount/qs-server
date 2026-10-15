package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// These files belong to the retained original capture, not to a caller-chosen
// approval pathname. They must not be removed or reconstructed during recovery.
type recoveryBoundary struct {
	Format       int      `json:"format_version"`
	Kind         string   `json:"kind"`
	MaterialHash string   `json:"original_material_sha256"`
	First        approval `json:"first_approved_recovery_boundary"`
}

func recoveryPaths(m material, materialHash, operation string) (boundary, consumed string, err error) {
	if !absolutePath(m.CapturePath) || !nonzeroHash(materialHash) || !operationRE.MatchString(operation) {
		return "", "", errors.New("recovery_anchor_rejected")
	}
	if e := recoveryParent(m.CapturePath); e != nil {
		return "", "", e
	}
	parent := filepath.Dir(m.CapturePath)
	prefix := filepath.Join(parent, ".m4-b-"+materialHash)
	return prefix + ".boundary.json", prefix + ".operation-" + fingerprint([]byte(operation)) + ".consumed", nil
}

func recoveryParent(path string) error {
	if !absolutePath(path) {
		return errors.New("recovery_private_parent_rejected")
	}
	s, e := privatePath(filepath.Dir(path), true)
	if e != nil || !recoveryParentAllowed(s, uint32(os.Getuid())) {
		return errors.New("recovery_private_parent_rejected")
	}
	return nil
}

func recoveryParentAllowed(s os.FileInfo, uid uint32) bool {
	if s == nil || !s.IsDir() || s.Mode().Perm()&022 != 0 {
		return false
	}
	stat, ok := s.Sys().(*syscall.Stat_t)
	return ok && (stat.Uid == 0 || stat.Uid == uid)
}

func readRecoveryBoundary(m material, a approval, materialHash string) (bool, error) {
	path, _, e := recoveryPaths(m, materialHash, a.Operation)
	if e != nil {
		return false, e
	}
	if _, e = os.Lstat(path); os.IsNotExist(e) {
		// A missing boundary after any operation claim is lost recovery state,
		// never permission to reset the marker or the original absolute budget.
		entries, re := os.ReadDir(filepath.Dir(path))
		if re != nil {
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
	if e != nil || strictDecode(raw, &state) != nil || state.Format != 1 || state.Kind != "m4_b_first_recovery_boundary" || state.MaterialHash != materialHash {
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
	// An earlier failed fsync must not be inferred successful merely because
	// complete bytes can now be read. Reconfirm durability without overwriting
	// the existing anchor before any subsequent mutable phase.
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	se, ce := f.Sync(), f.Close()
	if se != nil || ce != nil || syncDirectory(filepath.Dir(path)) != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	if _, e := readPrivate(path, fingerprint(raw), 32768); e != nil {
		return false, errors.New("recovery_boundary_durability_unproven")
	}
	if e := verifyRecoveryJournal(m, materialHash, state.First.Operation); e != nil {
		return false, e
	}
	return true, nil
}

func checkRecoveryBoundary(m material, a approval, materialHash string, required bool) error {
	exists, e := readRecoveryBoundary(m, a, materialHash)
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
	boundary, _, e := recoveryPaths(m, a.MaterialHash, a.Operation)
	if e != nil {
		return e
	}
	if !exists {
		raw, _ := json.Marshal(recoveryBoundary{1, "m4_b_first_recovery_boundary", a.MaterialHash, a})
		if writeExclusive(boundary, raw) != nil {
			return errors.New("recovery_boundary_persistence_unproven")
		}
	}
	if e := claimRecoveryOperation(m, a, hash, !exists); e != nil {
		return e
	}
	return checkRecoveryBoundary(m, a, a.MaterialHash, true)
}

func validateMutableBoundary(m material, a approval, materialHash, tool string) error {
	if e := validateApproval(a, m, materialHash, tool, time.Now()); e != nil {
		return e
	}
	return checkRecoveryBoundary(m, a, materialHash, true)
}
