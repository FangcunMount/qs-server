package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"time"
)

const completionAuthorizationMode = "idempotent_current_fact_confirmation"

// This approval authorizes a current fact confirmation, never a restore,
// generation advance or business start. It is intentionally not a consumed
// operation: same_existing confirmations are read-only and idempotent.
type runtimeCompletionApproval struct {
	Format              int                  `json:"format_version"`
	Kind                string               `json:"kind"`
	Operation           string               `json:"operation_id"`
	ToolSource          string               `json:"compiled_tool_source"`
	OriginalToolSource  string               `json:"original_compiled_tool_source"`
	DSource             string               `json:"d_execution_source"`
	DResources          string               `json:"d_execution_resources_sha256"`
	DResourceFiles      int                  `json:"d_execution_resource_files"`
	Binding             binding              `json:"binding"`
	MaterialHash        string               `json:"original_material_sha256"`
	CaptureEvidenceHash string               `json:"original_capture_execution_sha256"`
	IntentHash          string               `json:"original_runtime_intent_sha256"`
	RestoreApprovalHash string               `json:"original_consumed_restore_approval_sha256"`
	Boundary            runtimeBoundaryState `json:"committed_database_boundary"`
	RecordKind          string               `json:"committed_record_kind"`
	Mode                string               `json:"mode"`
	SQLVersion          uint64               `json:"clean_sql_version"`
	FactsHash           string               `json:"current_full_facts_sha256"`
	Marker              string               `json:"first_runtime_marker"`
	MarkerState         string               `json:"expected_marker_state"`
	MarkerHash          string               `json:"expected_marker_sha256"`
	FreezeHash          string               `json:"complete_writer_freeze_receipt_sha256"`
	ResponsibilityHash  string               `json:"responsibility_receipt_sha256"`
	Decision            string               `json:"decision_at"`
	Expires             string               `json:"expires_at"`
	DataOwner           string               `json:"data_owner"`
	RecoveryOwner       string               `json:"recovery_owner"`
	RuntimeFrozen       bool                 `json:"all_six_instances_and_other_writers_frozen"`
	RuntimeNotStarted   bool                 `json:"first_runtime_command_not_issued"`
	AllowCompletion     bool                 `json:"allow_committed_intent_confirmation"`
}

type runtimeCompletionReceipt struct {
	AuthorizationMode       string               `json:"authorization_mode"`
	BusinessStartAuthorized bool                 `json:"business_start_authorized"`
	ToolSource              string               `json:"compiled_tool_source"`
	OriginalToolSource      string               `json:"original_compiled_tool_source"`
	MaterialHash            string               `json:"original_material_sha256"`
	IntentHash              string               `json:"original_runtime_intent_sha256"`
	Boundary                runtimeBoundaryState `json:"confirmed_database_boundary"`
	ApprovalHash            string               `json:"current_approval_sha256"`
	Operation               string               `json:"current_confirmation_operation_id"`
	FactsHash               string               `json:"confirmed_full_facts_sha256"`
	MarkerHash              string               `json:"completion_ack_sha256"`
	MarkerCreated           bool                 `json:"completion_ack_written"`
	MarkerStatus            string               `json:"completion_ack_status"`
}

type runtimeCompletionInputs struct {
	approval runtimeCompletionApproval
	original material
	intent   runtimeIntent
	restored *approval
}

// The DTO reuses the existing fixed table proofs. It deliberately excludes
// execution timestamps/source/path and the separately bound committed record.
// It never changes the original material to bypass the old verify functions.
type runtimeCompletionFacts struct {
	Format         int                   `json:"format_version"`
	Mode           string                `json:"mode"`
	Binding        binding               `json:"binding"`
	DSource        string                `json:"d_execution_source"`
	DResources     string                `json:"d_execution_resources_sha256"`
	DResourceFiles int                   `json:"d_execution_resource_files"`
	Zone           string                `json:"session_time_zone"`
	Charset        string                `json:"session_character_set_results"`
	SQLMode        string                `json:"session_sql_mode"`
	LedgerDDL      string                `json:"sql_ledger_ddl"`
	Ledger         []ledgerRow           `json:"complete_sql_ledger"`
	Mongo          []byte                `json:"mongo40_bson"`
	Tables         [5]tableProof         `json:"historical_tables"`
	Retained       [8]retainedTableProof `json:"retained_modern_tables"`
}

func completionFacts(m material, mode string) runtimeCompletionFacts {
	return runtimeCompletionFacts{1, mode, m.Binding, m.DSource, m.DResources, m.DResourceFiles, m.SessionZone, m.SessionCharset, m.SessionSQLMode, m.LedgerDDL, m.Ledger, m.MongoHead, m.Tables, m.Retained}
}

func expectedCompletionFacts(m material, mode string) runtimeCompletionFacts {
	facts := completionFacts(m, mode)
	if mode == "target103" {
		facts.Ledger = []ledgerRow{{103, false}}
		facts.Tables = [5]tableProof{}
	}
	return facts
}

func completionFactsHash(v runtimeCompletionFacts) string {
	raw, _ := json.Marshal(v)
	return fingerprint(raw)
}

func (v runtimeCompletionApproval) validate(a arguments, tool string, now time.Time) error {
	if v.Format != 1 || v.Kind != "m4_d_complete_committed_runtime_intent" || !operationRE.MatchString(v.Operation) || v.ToolSource != tool || !sourceRE.MatchString(v.OriginalToolSource) || v.DSource != dSource || v.DResources != dResources || v.DResourceFiles != dResourceFiles || v.Binding != a.binding || v.MaterialHash != a.materialHash || v.CaptureEvidenceHash != a.captureEvidenceHash || v.IntentHash != a.runtimeIntentHash || v.RestoreApprovalHash != a.approvalHash || !validRuntimeBoundaryState(v.Boundary, a.binding) || v.Boundary.Generation == 0 || v.RecordKind != "m4_d_runtime_boundary" || !nonzeroHash(v.FactsHash) || !nonzeroHash(v.MarkerHash) || !absolutePath(v.Marker) || (v.MarkerState != "absent" && v.MarkerState != "same_existing") || !nonzeroHash(v.FreezeHash) || !nonzeroHash(v.ResponsibilityHash) || v.DataOwner != "system (10001)" || v.RecoveryOwner != "system (10001)" || !v.RuntimeFrozen || !v.RuntimeNotStarted || !v.AllowCompletion {
		return errors.New("runtime_completion_approval_binding_rejected")
	}
	if v.Mode != "target103" && v.Mode != "cancel102" && v.Mode != "restored102" || v.Mode == "target103" && v.SQLVersion != 103 || v.Mode != "target103" && v.SQLVersion != 102 || (v.Mode == "restored102") != (a.approval != "") {
		return errors.New("runtime_completion_approval_mode_rejected")
	}
	decision, e1 := time.Parse(time.RFC3339Nano, v.Decision)
	expiry, e2 := time.Parse(time.RFC3339Nano, v.Expires)
	if e1 != nil || e2 != nil || decision.After(now) || !expiry.After(now) || expiry.After(decision.Add(maxAction)) {
		return errors.New("runtime_completion_approval_expired_or_unbounded")
	}
	return nil
}

func readRuntimeCompletionInputs(a arguments, tool string, now time.Time) (out runtimeCompletionInputs, err error) {
	raw, e := readPrivateLimit(a.completionApproval, a.completionApprovalHash, maxRuntimeIntentBytes)
	if e != nil || decodeCaptureDocument(raw, &out.approval) != nil {
		return out, errors.New("runtime_completion_approval_rejected")
	}
	if e := out.approval.validate(a, tool, now); e != nil {
		return out, e
	}
	raw, e = readPrivate(a.material, a.materialHash)
	if e != nil || strictDecode(raw, &out.original) != nil || validateMaterial(out.original, out.approval.OriginalToolSource, a.binding) != nil || out.original.CapturePath != a.material {
		return out, errors.New("runtime_completion_original_material_rejected")
	}
	if e := readCaptureEvidence(out.original, a.materialHash, out.approval.OriginalToolSource, a.captureEvidence, a.captureEvidenceHash); e != nil {
		return out, e
	}
	out.intent, e = readRuntimeIntent(a.runtimeIntent, a.runtimeIntentHash)
	if e != nil {
		return out, e
	}
	if out.intent.Mode != out.approval.Mode || out.intent.Marker != out.approval.Marker || out.approval.Marker != derivedRuntimeMarker(out.original) || completionFactsHash(expectedCompletionFacts(out.original, out.intent.Mode)) != out.approval.FactsHash {
		return out, errors.New("runtime_completion_original_scope_rejected")
	}
	if out.intent.Mode == "restored102" {
		var old approval
		raw, e = readPrivateLimit(a.approval, a.approvalHash, maxRuntimeIntentBytes)
		if e != nil || strictDecode(raw, &old) != nil {
			return out, errors.New("runtime_completion_historical_approval_rejected")
		}
		out.restored = &old
	}
	// This is only historical field/window qualification, not renewed authority.
	if e := out.validateHistory(a, mustTime(out.intent.Decision)); e != nil {
		return out, e
	}
	return out, nil
}

func (v runtimeCompletionInputs) validateHistory(a arguments, at time.Time) error {
	if validateRuntimeIntentScope(v.intent, v.original, a.materialHash, v.approval.OriginalToolSource, v.restored, at) != nil {
		return errors.New("runtime_completion_original_intent_history_rejected")
	}
	if v.restored != nil {
		return v.checkConsumedHistory(a, at)
	}
	return nil
}

// Read an already consumed original scope without calling the live approval,
// retry, claim or journal-writing entry points. The current approval pins the
// supplied old approval SHA; the old DB record did not store that SHA.
func (v runtimeCompletionInputs) checkConsumedHistory(a arguments, at time.Time) error {
	old := *v.restored
	if validateApprovalScope(old, v.original, a.materialHash, v.approval.OriginalToolSource, at) != nil {
		return errors.New("runtime_completion_historical_approval_rejected")
	}
	path, _, e := recoveryPaths(v.original, a.materialHash, old.Operation)
	if e != nil {
		return e
	}
	raw, e := readOwnedPrivate(path, maxRuntimeIntentBytes)
	var boundary recoveryBoundary
	if e != nil || strictDecode(raw, &boundary) != nil || boundary.Format != 1 || boundary.Kind != "m4_d_first_recovery_boundary" || boundary.MaterialHash != a.materialHash || !validInitialRecoveryState(boundary.InitialLedger, boundary.InitialMissing) {
		return errors.New("runtime_completion_historical_boundary_rejected")
	}
	first := boundary.First
	first.Operation = old.Operation
	if first != old || validateApprovalScope(boundary.First, v.original, a.materialHash, v.approval.OriginalToolSource, at) != nil || verifyRecoveryJournal(v.original, a.materialHash, boundary.First.Operation) != nil {
		return errors.New("runtime_completion_historical_boundary_rejected")
	}
	path, e = recoveryJournalPath(v.original, a.materialHash)
	if e != nil {
		return e
	}
	raw, e = readOwnedPrivate(path, maxRecoveryJournalBytes)
	ops, pe := parseRecoveryOperations(raw, a.materialHash)
	if e == nil && pe == nil {
		for _, op := range ops {
			if op.Operation == old.Operation && op.ApprovalHash == a.approvalHash {
				return nil
			}
		}
	}
	return errors.New("runtime_completion_original_operation_not_consumed")
}

func (v runtimeCompletionInputs) matchCommitted(o runtimeBoundaryObservation, a arguments) error {
	if o.Record == nil || v.original.RuntimeBoundary.Generation == math.MaxUint64 || o.State != v.approval.Boundary || o.State.Generation != v.original.RuntimeBoundary.Generation+1 || fingerprint(o.Raw) != o.State.Fingerprint || validateRuntimeBoundaryRecord(*o.Record, o.Raw, a.binding) != nil {
		return errors.New("runtime_completion_committed_record_rejected")
	}
	r := *o.Record
	if r.Generation != o.State.Generation || r.Previous != v.original.RuntimeBoundary.Fingerprint || r.ToolSource != v.approval.OriginalToolSource || r.MaterialHash != a.materialHash || r.IntentHash != a.runtimeIntentHash || r.Mode != v.intent.Mode || r.SQLVersion != v.approval.SQLVersion || r.Kind != v.approval.RecordKind {
		return errors.New("runtime_completion_committed_intent_rejected")
	}
	created, e := time.Parse(time.RFC3339Nano, r.Created)
	if e != nil || created.After(mustTime(v.approval.Decision)) {
		return errors.New("runtime_completion_committed_time_rejected")
	}
	return v.validateHistory(a, created)
}

// The original marker's Created time was not frozen in the committed record.
// A missing marker therefore uses a distinct deterministic representation. No
// current source, authorization, operation or new timestamp enters its bytes.
type completedRuntimeMarker struct {
	Format         int                   `json:"format_version"`
	Kind           string                `json:"kind"`
	Binding        binding               `json:"binding"`
	Boundary       runtimeBoundaryState  `json:"committed_database_boundary"`
	OriginalRecord runtimeBoundaryRecord `json:"original_committed_record"`
	Gate           string                `json:"runtime_gate"`
}

type originalRuntimeMarker struct {
	Kind, ToolSource, DSource, Resources, Material, Intent, Mode, Gate, Created string
	SQLVersion                                                                  uint64
}

func deterministicCompletionMarker(o runtimeBoundaryObservation, b binding) []byte {
	key, _ := runtimeGateKey(b)
	raw, _ := json.Marshal(completedRuntimeMarker{1, "m4_d_completed_committed_runtime_boundary", b, o.State, *o.Record, key})
	return raw
}

func (v runtimeCompletionInputs) verifyMarker(o runtimeBoundaryObservation, a arguments) error {
	raw, e := readPrivateLimit(v.approval.Marker, v.approval.MarkerHash, maxRuntimeIntentBytes)
	if e != nil {
		return errors.New("runtime_completion_marker_rejected")
	}
	expected := deterministicCompletionMarker(o, a.binding)
	if bytes.Equal(raw, expected) {
		return nil
	}
	var old originalRuntimeMarker
	key, _ := runtimeGateKey(a.binding)
	if decodeCaptureDocument(raw, &old) != nil || old.Kind != "m4_d_first_runtime_boundary" || old.ToolSource != v.approval.OriginalToolSource || old.DSource != dSource || old.Resources != dResources || old.Material != a.materialHash || old.Intent != a.runtimeIntentHash || old.Mode != v.intent.Mode || old.Gate != key || old.SQLVersion != v.approval.SQLVersion {
		return errors.New("runtime_completion_marker_rejected")
	}
	created, e := time.Parse(time.RFC3339Nano, old.Created)
	if e != nil || created.Before(mustTime(o.Record.Created)) || created.After(mustTime(v.approval.Decision)) || v.validateHistory(a, created) != nil {
		return errors.New("runtime_completion_marker_history_rejected")
	}
	return nil
}

func (v runtimeCompletionInputs) reread(a arguments, tool string) error {
	again, e := readRuntimeCompletionInputs(a, tool, time.Now())
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(v, again) {
		return errors.New("runtime_completion_fixed_input_changed")
	}
	return nil
}

func (v runtimeCompletionInputs) checkMarkerBefore(o runtimeBoundaryObservation, a arguments) error {
	if v.approval.MarkerState == "same_existing" {
		return v.verifyMarker(o, a)
	}
	if fingerprint(deterministicCompletionMarker(o, a.binding)) != v.approval.MarkerHash {
		return errors.New("runtime_completion_deterministic_marker_hash_rejected")
	}
	return markerAbsent(v.approval.Marker)
}

func (d database) confirmCompletionFacts(ctx context.Context, v runtimeCompletionInputs, a arguments, tool string) (runtimeBoundaryObservation, error) {
	if e := d.gate.require(ctx); e != nil {
		return runtimeBoundaryObservation{}, e
	}
	o, e := readRuntimeBoundary(ctx, d.conn, a.binding)
	if e != nil {
		return o, e
	}
	if e := v.matchCommitted(o, a); e != nil {
		return o, e
	}
	current, e := d.snapshot(ctx, tool, v.approval.SQLVersion, true)
	if e != nil {
		return o, e
	}
	if current.RuntimeBoundary != o.State || completionFactsHash(completionFacts(current, v.intent.Mode)) != v.approval.FactsHash {
		return o, errors.New("runtime_completion_current_facts_changed")
	}
	if e := requireRuntimeBoundary(ctx, d.conn, a.binding, o.State); e != nil {
		return o, e
	}
	if e := d.gate.require(ctx); e != nil {
		return o, e
	}
	if e := v.reread(a, tool); e != nil {
		return o, e
	}
	if ctx.Err() != nil {
		return o, errors.New("runtime_completion_deadline_exhausted")
	}
	return o, nil
}

func (d database) completeFirstRuntime(ctx context.Context, v runtimeCompletionInputs, a arguments, tool string, r *receipt) (*runtimeCompletionReceipt, error) {
	before, e := d.confirmCompletionFacts(ctx, v, a, tool)
	if e != nil {
		return nil, e
	}
	if e := v.checkMarkerBefore(before, a); e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		return nil, errors.New("runtime_completion_deadline_exhausted")
	}
	created := v.approval.MarkerState == "absent"
	if created {
		if e := writeExclusive(v.approval.Marker, deterministicCompletionMarker(before, a.binding)); e != nil {
			return nil, errors.New("runtime_completion_marker_write_failed_or_unknown")
		}
	}
	if e := v.verifyMarker(before, a); e != nil {
		return nil, e
	}
	after, e := d.confirmCompletionFacts(ctx, v, a, tool)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(before, after) {
		return nil, errors.New("runtime_completion_committed_record_changed")
	}
	if e := v.verifyMarker(after, a); e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		return nil, errors.New("runtime_completion_deadline_exhausted")
	}
	r.DatabaseBoundary, r.DatabaseBoundaryRecord = &after.State, after.Record
	status := "verified_existing"
	if created {
		status = "created_deterministic_completion_marker"
	}
	return &runtimeCompletionReceipt{AuthorizationMode: completionAuthorizationMode, BusinessStartAuthorized: false, ToolSource: tool, OriginalToolSource: v.approval.OriginalToolSource, MaterialHash: a.materialHash, IntentHash: a.runtimeIntentHash, Boundary: after.State, ApprovalHash: a.completionApprovalHash, Operation: v.approval.Operation, FactsHash: v.approval.FactsHash, MarkerHash: v.approval.MarkerHash, MarkerCreated: created, MarkerStatus: status}, nil
}

func (v runtimeCompletionInputs) verifyTerminal(a arguments, tool string, r *runtimeCompletionReceipt, record *runtimeBoundaryRecord) error {
	if r == nil || record == nil {
		return errors.New("runtime_completion_terminal_record_required")
	}
	if e := v.reread(a, tool); e != nil {
		return e
	}
	raw, _ := json.Marshal(record)
	o := runtimeBoundaryObservation{State: r.Boundary, Record: record, Raw: raw}
	if e := v.matchCommitted(o, a); e != nil {
		return e
	}
	return v.verifyMarker(o, a)
}
