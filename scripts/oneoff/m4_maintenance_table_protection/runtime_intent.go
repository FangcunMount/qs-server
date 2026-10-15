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
	Format             int     `json:"format_version"`
	Kind               string  `json:"kind"`
	Operation          string  `json:"operation_id"`
	ToolSource         string  `json:"compiled_tool_source"`
	DSource            string  `json:"d_execution_source"`
	DResources         string  `json:"d_execution_resources_sha256"`
	DResourceFiles     int     `json:"d_execution_resource_files"`
	MaterialHash       string  `json:"independently_frozen_material_sha256"`
	CapturePath        string  `json:"first_capture_path"`
	Marker             string  `json:"first_runtime_marker"`
	Binding            binding `json:"binding"`
	Mode               string  `json:"mode"`
	FreezeHash         string  `json:"complete_writer_freeze_receipt_sha256"`
	ResponsibilityHash string  `json:"responsibility_receipt_sha256"`
	WindowStarted      string  `json:"window_started_at"`
	Decision           string  `json:"decision_at"`
	Deadline           string  `json:"deadline"`
	Expires            string  `json:"expires_at"`
	RuntimeFrozen      bool    `json:"all_six_instances_and_other_writers_frozen"`
	RuntimeNotStarted  bool    `json:"first_runtime_command_not_issued"`
	AllowMarker        bool    `json:"allow_first_runtime_marker"`
	ApplyNotDispatched bool    `json:"offline_apply_command_not_dispatched"`
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

func validateRuntimeIntent(v runtimeIntent, m material, hash, tool string, ap *approval, now time.Time) error {
	if v.Format != 1 || v.Kind != "m4_d_first_runtime_intent" || !operationRE.MatchString(v.Operation) || v.ToolSource != tool || v.DSource != dSource || v.DResources != dResources || v.DResourceFiles != dResourceFiles || v.MaterialHash != hash || !nonzeroHash(hash) || v.CapturePath != m.CapturePath || v.Marker != derivedRuntimeMarker(m) || v.Binding != m.Binding || !nonzeroHash(v.FreezeHash) || !nonzeroHash(v.ResponsibilityHash) || !v.RuntimeFrozen || !v.RuntimeNotStarted || !v.AllowMarker {
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
	case "target103":
		if ap != nil || v.ApplyNotDispatched || deadline.After(start.Add(30*time.Minute)) {
			return errors.New("runtime_target_intent_rejected")
		}
		if e := noRecoveryState(m, hash); e != nil {
			return e
		}
	case "cancel102":
		if ap != nil || !v.ApplyNotDispatched {
			return errors.New("runtime_cancel_intent_rejected")
		}
		if e := noRecoveryState(m, hash); e != nil {
			return e
		}
	case "restored102":
		if ap == nil || v.ApplyNotDispatched || v.WindowStarted != ap.WindowStarted || v.Deadline != ap.Deadline || v.FreezeHash != ap.FreezeHash || v.ResponsibilityHash != ap.ResponsibilityHash || expiry.After(mustTime(ap.Expires)) || validateApproval(*ap, m, hash, tool, now) != nil {
			return errors.New("runtime_original_recovery_scope_rejected")
		}
	default:
		return errors.New("runtime_intent_mode_rejected")
	}
	return markerAbsent(v.Marker)
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
	if e != nil || strictDecode(raw, &v) != nil {
		return v, errors.New("runtime_independent_intent_rejected")
	}
	return v, nil
}

func rereadRuntimeInputs(v runtimeIntent, m material, a arguments, tool string, ap *approval) error {
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
		if e := checkRestoredAuthorization(m, *ap, a.material, a.materialHash, a.approval, a.approvalHash, tool); e != nil {
			return e
		}
	}
	return validateRuntimeIntent(v, m, a.materialHash, tool, ap, time.Now())
}

// The actual CLI producer and restore contend on one server-session lock. No
// filesystem writer outside this action is an admitted D runtime-start author.
func (d database) markFirstRuntime(ctx context.Context, m material, v runtimeIntent, a arguments, tool string, ap *approval, r *receipt) error {
	if e := d.gate.require(ctx); e != nil {
		return e
	}
	if e := rereadRuntimeInputs(v, m, a, tool, ap); e != nil {
		return e
	}
	version := uint64(102)
	if v.Mode == "target103" {
		version = 103
	}
	current, e := d.snapshot(ctx, tool, version, true)
	if e != nil {
		return e
	}
	if version == 103 {
		e = verifyTarget(m, current)
	} else {
		e = verifyRestored(m, current)
	}
	if e != nil {
		return e
	}
	if e := rereadRuntimeInputs(v, m, a, tool, ap); e != nil {
		return e
	}
	if e := d.gate.require(ctx); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("runtime_marker_deadline_exhausted")
	}
	marker := struct {
		Kind, ToolSource, DSource, Resources, Material, Intent, Mode, Gate, Created string
		SQLVersion                                                                  uint64
	}{"m4_d_first_runtime_boundary", tool, dSource, dResources, a.materialHash, a.runtimeIntentHash, v.Mode, d.gate.key, time.Now().UTC().Format(time.RFC3339Nano), version}
	raw, _ := json.Marshal(marker)
	// Any attempted O_EXCL write is a conservative forward-only boundary until
	// proved otherwise. A failed/partial marker is never deleted or retried.
	r.MarkerWriteUnknown = true
	if e := writeExclusive(v.Marker, raw); e != nil {
		return errors.New("runtime_marker_write_failed_or_unknown")
	}
	got, e := readPrivateLimit(v.Marker, fingerprint(raw), maxRuntimeIntentBytes)
	if e != nil || !bytes.Equal(raw, got) || ctx.Err() != nil {
		return errors.New("runtime_marker_readback_or_budget_unproven")
	}
	r.MarkerHash, r.MarkerWritten, r.MarkerWriteUnknown = fingerprint(raw), true, false
	return nil
}
