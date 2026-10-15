package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxRuntimeIntentBytes = 32768

type runtimeIntent struct {
	Format              int     `json:"format_version"`
	Kind                string  `json:"kind"`
	Operation           string  `json:"operation_id"`
	ToolSource          string  `json:"compiled_tool_source"`
	CSource             string  `json:"c_execution_source"`
	CResources          string  `json:"c_execution_resources_sha256"`
	CResourceFiles      int     `json:"c_execution_resource_files"`
	MaterialHash        string  `json:"independently_frozen_material_sha256"`
	CaptureEvidenceHash string  `json:"capture_execution_evidence_sha256"`
	RestoreApprovalHash string  `json:"original_restore_approval_sha256"`
	CapturePath         string  `json:"first_capture_path"`
	Marker              string  `json:"first_runtime_marker"`
	Binding             binding `json:"binding"`
	Mode                string  `json:"mode"`
	FreezeHash          string  `json:"complete_writer_freeze_receipt_sha256"`
	ResponsibilityHash  string  `json:"responsibility_receipt_sha256"`
	WindowStarted       string  `json:"window_started_at"`
	Decision            string  `json:"decision_at"`
	Deadline            string  `json:"deadline"`
	Expires             string  `json:"expires_at"`
	RuntimeFrozen       bool    `json:"all_six_instances_and_other_writers_frozen"`
	RuntimeNotStarted   bool    `json:"first_runtime_command_not_issued"`
	AllowMarker         bool    `json:"allow_first_runtime_marker"`
	ApplyNotDispatched  bool    `json:"offline_apply_command_not_dispatched"`
}

func derivedRuntimeMarker(m material) string {
	return filepath.Join(filepath.Dir(m.CapturePath), "first-runtime")
}

func noRecoveryState(m material, hash string) error {
	boundary, _, e := recoveryPaths(m, hash, "1-1")
	if e != nil {
		return e
	}
	entries, e := os.ReadDir(filepath.Dir(boundary))
	if e != nil {
		return errors.New("runtime_recovery_state_unavailable")
	}
	prefix := strings.TrimSuffix(filepath.Base(boundary), ".boundary.json") + "."
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			return errors.New("runtime_unexpected_recovery_state")
		}
	}
	return nil
}

func validateRuntimeIntent(v runtimeIntent, m material, a arguments, tool string, ap *approval, now time.Time) error {
	if v.Format != 1 || v.Kind != "m4_c_first_runtime_intent" || !operationRE.MatchString(v.Operation) || v.ToolSource != tool || v.CSource != cSource || v.CResources != cResources || v.CResourceFiles != cResourceFiles || v.MaterialHash != a.materialHash || !nonzeroHash(a.materialHash) || v.CaptureEvidenceHash != a.captureEvidenceHash || !nonzeroHash(v.CaptureEvidenceHash) || v.CapturePath != m.CapturePath || v.Marker != derivedRuntimeMarker(m) || v.Binding != m.Binding || !nonzeroHash(v.FreezeHash) || !nonzeroHash(v.ResponsibilityHash) || !v.RuntimeFrozen || !v.RuntimeNotStarted || !v.AllowMarker {
		return errors.New("runtime_intent_binding_rejected")
	}
	start, e1 := time.Parse(time.RFC3339Nano, v.WindowStarted)
	decision, e2 := time.Parse(time.RFC3339Nano, v.Decision)
	deadline, e3 := time.Parse(time.RFC3339Nano, v.Deadline)
	expiry, e4 := time.Parse(time.RFC3339Nano, v.Expires)
	captured, e5 := time.Parse(time.RFC3339Nano, m.CapturedAt)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || start.After(captured) || captured.After(decision) || decision.After(now) || !deadline.After(now) || deadline.After(decision.Add(time.Hour)) || !expiry.After(now) || expiry.After(now.Add(24*time.Hour)) {
		return errors.New("runtime_intent_budget_rejected")
	}
	switch v.Mode {
	case "target102":
		if ap != nil || v.RestoreApprovalHash != "" || v.ApplyNotDispatched || deadline.After(start.Add(30*time.Minute)) {
			return errors.New("runtime_target_intent_rejected")
		}
		if e := noRecoveryState(m, a.materialHash); e != nil {
			return e
		}
	case "cancel101":
		if ap != nil || v.RestoreApprovalHash != "" || !v.ApplyNotDispatched {
			return errors.New("runtime_cancel_intent_rejected")
		}
		if e := noRecoveryState(m, a.materialHash); e != nil {
			return e
		}
	case "restored101":
		if ap == nil || !nonzeroHash(v.RestoreApprovalHash) || v.RestoreApprovalHash != a.approvalHash || v.ApplyNotDispatched || v.WindowStarted != ap.WindowStarted || v.Deadline != ap.Deadline || v.FreezeHash != ap.FreezeHash || v.ResponsibilityHash != ap.ResponsibilityHash || expiry.After(mustTime(ap.Expires)) {
			return errors.New("runtime_original_recovery_scope_rejected")
		}
	default:
		return errors.New("runtime_intent_mode_rejected")
	}
	return nil
}

func mustTime(text string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, text)
	return t
}

func runtimeIntentContext(ctx context.Context, v runtimeIntent) (context.Context, context.CancelFunc) {
	end := mustTime(v.Deadline)
	if expiry := mustTime(v.Expires); expiry.Before(end) {
		end = expiry
	}
	return context.WithDeadline(ctx, end)
}

func readRuntimeIntent(path, hash string) (runtimeIntent, error) {
	var v runtimeIntent
	raw, e := readPrivateLimit(path, hash, maxRuntimeIntentBytes)
	if e != nil || decodeCaptureDocument(raw, &v) != nil {
		return v, errors.New("runtime_independent_intent_rejected")
	}
	return v, nil
}

func rereadRuntimeDocuments(v runtimeIntent, m material, a arguments, tool string, ap *approval) error {
	if e := readCaptureEvidence(m, a.materialHash, tool, a.captureEvidence, a.captureEvidenceHash); e != nil {
		return e
	}
	raw, e := readPrivate(a.material, a.materialHash)
	var again material
	if e != nil || strictDecode(raw, &again) != nil || validateMaterial(again, tool, m.Binding) != nil || again.CapturePath != a.material {
		return errors.New("runtime_original_material_changed")
	}
	first, _ := json.Marshal(m)
	next, _ := json.Marshal(again)
	if !bytes.Equal(first, next) {
		return errors.New("runtime_original_material_changed")
	}
	againIntent, e := readRuntimeIntent(a.runtimeIntent, a.runtimeIntentHash)
	if e != nil || againIntent != v {
		return errors.New("runtime_frozen_intent_changed")
	}
	if ap != nil {
		raw, e := readPrivateLimit(a.approval, a.approvalHash, 32768)
		var approved approval
		if e != nil || strictDecode(raw, &approved) != nil || approved != *ap {
			return errors.New("runtime_original_recovery_approval_changed")
		}
	}
	return validateRuntimeIntent(v, m, a, tool, ap, time.Now())
}

// Before any write, both the independent intent and the original consumed
// restoration responsibility remain live. After the marker is durable only
// document equality is rechecked; marker absence can no longer be claimed.
func rereadRuntimeInputs(ctx context.Context, v runtimeIntent, m material, a arguments, tool string, ap *approval) error {
	if e := rereadRuntimeDocuments(v, m, a, tool, ap); e != nil {
		return e
	}
	if ap != nil {
		if e := verifyConsumedRecoveryInputs(ctx, m, *ap, a.material, a.materialHash, a.approval, a.approvalHash, a.captureEvidence, a.captureEvidenceHash, tool); e != nil {
			return e
		}
	}
	if e := markerAbsent(v.Marker); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("runtime_marker_deadline_exhausted")
	}
	return nil
}

// This action establishes a durable first-start boundary, never starts an old
// or new process, and never repairs a previously committed/unknown operation.
func (d database) markFirstRuntime(ctx context.Context, m material, v runtimeIntent, a arguments, tool string, ap *approval, r *receipt) error {
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	if e := rereadRuntimeInputs(ctx, v, m, a, tool, ap); e != nil {
		return e
	}
	version := uint64(101)
	if v.Mode == "target102" {
		version = 102
	}
	current, e := d.snapshot(ctx, tool, version, true)
	if e != nil {
		return e
	}
	if version == 102 {
		e = verifyTarget(m, current)
	} else {
		e = verifyRestored(m, current)
	}
	if e != nil {
		return e
	}
	if e := d.advanceRuntimeBoundary(ctx, m, v, a, tool, ap, r); e != nil {
		return e
	}
	if e := d.gate.require(ctx); e != nil {
		return e
	}
	if e := rereadRuntimeInputs(ctx, v, m, a, tool, ap); e != nil {
		return e
	}
	// Exactly the pre-commit bytes retained inside the SQL record are written.
	// Partial/unknown marker writes are preserved; there is no automatic retry.
	r.MarkerWriteUnknown = true
	raw := r.DatabaseBoundaryRecord.MarkerRaw
	if e := writeExclusive(v.Marker, raw); e != nil {
		return errors.New("runtime_marker_write_failed_or_unknown")
	}
	got, e := readPrivateLimit(v.Marker, r.DatabaseBoundaryRecord.MarkerHash, maxRuntimeIntentBytes)
	if e != nil || !bytes.Equal(got, raw) || ctx.Err() != nil {
		return errors.New("runtime_marker_readback_or_budget_unproven")
	}
	r.MarkerHash, r.MarkerWritten, r.MarkerWriteUnknown = r.DatabaseBoundaryRecord.MarkerHash, true, false
	return d.requireCompletedRuntimeMark(ctx, m, v, a, tool, ap, r)
}

func (d database) requireCompletedRuntimeMark(ctx context.Context, m material, v runtimeIntent, a arguments, tool string, ap *approval, r *receipt) error {
	if r.DatabaseBoundary == nil || r.DatabaseBoundaryRecord == nil || !r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown || !r.MarkerWritten || r.MarkerWriteUnknown {
		return errors.New("runtime_marker_outcome_unproven")
	}
	if e := d.gate.require(ctx); e != nil {
		return e
	}
	if e := requireRuntimeBoundary(ctx, d.conn, m.Binding, *r.DatabaseBoundary); e != nil {
		return e
	}
	if e := rereadRuntimeDocuments(v, m, a, tool, ap); e != nil {
		return e
	}
	raw, e := readPrivateLimit(v.Marker, r.MarkerHash, maxRuntimeIntentBytes)
	if e != nil || !bytes.Equal(raw, r.DatabaseBoundaryRecord.MarkerRaw) {
		return errors.New("runtime_marker_readback_or_budget_unproven")
	}
	if ctx.Err() != nil {
		return errors.New("runtime_marker_deadline_exhausted")
	}
	return nil
}
