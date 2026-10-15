package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func runtimeInputsFixture(t *testing.T, state *runtimeBoundaryState) (material, approval, arguments, runtimeIntent) {
	t.Helper()
	m, ap, hash, path, ah := recoveryInputs(t)
	if state != nil {
		m.RuntimeBoundary = *state
		m.CapturePath = filepath.Join(filepath.Dir(m.CapturePath), "next-capture.json")
		raw, _ := json.Marshal(m)
		if e := writeExclusive(m.CapturePath, raw); e != nil {
			t.Fatal(e)
		}
		hash = fingerprint(raw)
	}
	evidence := unitCaptureEvidence(t, m, hash, "runtime")
	a := arguments{material: m.CapturePath, materialHash: hash, approval: path, approvalHash: ah, captureEvidence: strings.TrimPrefix(evidence[0], "--capture-evidence-file="), captureEvidenceHash: strings.TrimPrefix(evidence[1], "--capture-evidence-sha256=")}
	now := time.Now().UTC()
	v := runtimeIntent{Format: 1, Kind: "m4_c_first_runtime_intent", Operation: "125-1", ToolSource: fixtureSource, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, MaterialHash: hash, CaptureEvidenceHash: a.captureEvidenceHash, CapturePath: m.CapturePath, Marker: derivedRuntimeMarker(m), Binding: m.Binding, Mode: "target102", FreezeHash: ap.FreezeHash, ResponsibilityHash: ap.ResponsibilityHash, WindowStarted: ap.WindowStarted, Decision: now.Format(time.RFC3339Nano), Deadline: now.Add(10 * time.Minute).Format(time.RFC3339Nano), Expires: now.Add(15 * time.Minute).Format(time.RFC3339Nano), RuntimeFrozen: true, RuntimeNotStarted: true, AllowMarker: true}
	return m, ap, a, v
}

func freezeRuntimeIntent(t *testing.T, v runtimeIntent, a *arguments) {
	t.Helper()
	raw, _ := json.Marshal(v)
	a.runtimeIntent = filepath.Join(filepath.Dir(v.CapturePath), "runtime-intent.json")
	a.runtimeIntentHash = fingerprint(raw)
	if e := writeExclusive(a.runtimeIntent, raw); e != nil {
		t.Fatal(e)
	}
}

func TestRuntimeIntentRequiresSuccessfulCaptureAndModeSpecificAuthority(t *testing.T) {
	m, ap, a, v := runtimeInputsFixture(t, nil)
	if validateRuntimeIntent(v, m, a, fixtureSource, nil, time.Now()) != nil {
		t.Fatal("target intent rejected")
	}
	v.Mode, v.ApplyNotDispatched = "cancel101", true
	freezeRuntimeIntent(t, v, &a)
	if e := rereadRuntimeInputs(t.Context(), v, m, a, fixtureSource, nil); e != nil {
		t.Fatal("successful capture cancellation rejected", e)
	}
	if consumeApproval(m, ap, a.approvalHash) != nil {
		t.Fatal("original restoration responsibility unavailable")
	}
	if validateRuntimeIntent(v, m, a, fixtureSource, nil, time.Now()) == nil {
		t.Fatal("cancel borrowed a restore journal")
	}
	v.Mode, v.ApplyNotDispatched, v.RestoreApprovalHash, v.Deadline = "restored101", false, a.approvalHash, ap.Deadline
	if validateRuntimeIntent(v, m, a, fixtureSource, &ap, time.Now()) != nil {
		t.Fatal("same restored scope rejected")
	}
	v.Deadline = time.Now().Add(31 * time.Minute).Format(time.RFC3339Nano)
	if validateRuntimeIntent(v, m, a, fixtureSource, &ap, time.Now()) == nil {
		t.Fatal("mark reset restoration absolute deadline")
	}
	v.Deadline = ap.Deadline
	v.RestoreApprovalHash = fingerprint([]byte("other approval"))
	if validateRuntimeIntent(v, m, a, fixtureSource, &ap, time.Now()) == nil {
		t.Fatal("different consumed approval rebound")
	}
}

func TestRuntimeIntentRejectsReboundSourceMarkerEvidenceAndBudgets(t *testing.T) {
	m, _, a, base := runtimeInputsFixture(t, nil)
	for name, change := range map[string]func(*runtimeIntent){
		"marker":         func(v *runtimeIntent) { v.Marker += "-other" },
		"material":       func(v *runtimeIntent) { v.MaterialHash = fingerprint([]byte("other")) },
		"capture_proof":  func(v *runtimeIntent) { v.CaptureEvidenceHash = fingerprint([]byte("other")) },
		"source":         func(v *runtimeIntent) { v.CSource = fixtureSource },
		"permission":     func(v *runtimeIntent) { v.AllowMarker = false },
		"started":        func(v *runtimeIntent) { v.RuntimeNotStarted = false },
		"mode":           func(v *runtimeIntent) { v.Mode = "other" },
		"window":         func(v *runtimeIntent) { v.Deadline = time.Now().Add(40 * time.Minute).Format(time.RFC3339Nano) },
		"expired":        func(v *runtimeIntent) { v.Expires = time.Now().Add(-time.Second).Format(time.RFC3339Nano) },
		"before_capture": func(v *runtimeIntent) { v.Decision = time.Now().Add(-2 * time.Minute).Format(time.RFC3339Nano) },
	} {
		t.Run(name, func(t *testing.T) {
			v := base
			change(&v)
			if validateRuntimeIntent(v, m, a, fixtureSource, nil, time.Now()) == nil {
				t.Fatal("rebound intent accepted")
			}
		})
	}
	_, ap, hash, _, _ := recoveryInputs(t)
	ap.Marker += "-other"
	if validateApproval(ap, m, hash, fixtureSource, time.Now()) == nil {
		t.Fatal("restore escaped derived marker")
	}
}

func TestRuntimeIntentRawShapeRejectedBeforeEnvironment(t *testing.T) {
	for _, kind := range []string{"wrong_hash", "unknown", "alias", "null", "missing_false", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			m, _, a, v := runtimeInputsFixture(t, nil)
			raw, _ := json.Marshal(v)
			switch kind {
			case "unknown":
				raw = append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"extra":true}`)...)
			case "alias":
				raw = bytes.Replace(raw, []byte(`"allow_first_runtime_marker"`), []byte(`"Allow_First_Runtime_Marker"`), 1)
			case "null":
				raw = bytes.Replace(raw, []byte(`"offline_apply_command_not_dispatched":false`), []byte(`"offline_apply_command_not_dispatched":null`), 1)
			case "missing_false":
				raw = bytes.Replace(raw, []byte(`,"offline_apply_command_not_dispatched":false`), nil, 1)
			case "oversize":
				raw = bytes.Repeat([]byte(" "), maxRuntimeIntentBytes+1)
			}
			a.runtimeIntent = filepath.Join(filepath.Dir(m.CapturePath), "invalid-intent.json")
			if e := writeExclusive(a.runtimeIntent, raw); e != nil {
				t.Fatal(e)
			}
			a.runtimeIntentHash = fingerprint(raw)
			if kind == "wrong_hash" {
				a.runtimeIntentHash = fingerprint([]byte("other"))
			}
			args := actorConsumerArgs(m, a.materialHash, "mark-first-runtime", "", "")
			args = append(args, "--runtime-intent-file="+a.runtimeIntent, "--runtime-intent-sha256="+a.runtimeIntentHash, "--capture-evidence-file="+a.captureEvidence, "--capture-evidence-sha256="+a.captureEvidenceHash)
			r := run(context.Background(), args, func(string) string { t.Fatal("invalid intent reached ENV"); return "" }, fixtureSource)
			if r.Complete || r.Error != "runtime_independent_intent_rejected" || r.DatabaseBoundaryAdvanced || r.MarkerWritten {
				t.Fatal("invalid intent progressed", r)
			}
		})
	}
}

func TestMarkerIsFrozenBeforeCommitAndNeverReferencesContainingRecordHash(t *testing.T) {
	m, _, a, v := runtimeInputsFixture(t, nil)
	a.runtimeIntentHash = fingerprint([]byte("exact independent raw intent"))
	record, raw, e := nextRuntimeBoundary(m, v, a, fixtureSource)
	if e != nil {
		t.Fatal(e)
	}
	_, again, e := nextRuntimeBoundary(m, v, a, fixtureSource)
	var marker runtimeMarker
	if e != nil || !bytes.Equal(raw, again) || strictDecode(record.MarkerRaw, &marker) != nil || marker.Created != v.Decision || marker.IntentHash != a.runtimeIntentHash || marker.Previous != m.RuntimeBoundary.Fingerprint || marker.Generation != 1 || bytes.Contains(record.MarkerRaw, []byte(fingerprint(raw))) {
		t.Fatal("marker regenerated after commit or circular binding")
	}
	if record.MarkerHash != fingerprint(record.MarkerRaw) {
		t.Fatal("record lost original marker bytes")
	}
	m.RuntimeBoundary.Generation = math.MaxUint64
	if _, _, e := nextRuntimeBoundary(m, v, a, fixtureSource); e == nil {
		t.Fatal("generation overflow reset state")
	}
}

func TestFirstBoundaryCreateUnknownNeverWritesMarker(t *testing.T) {
	m, _, a, v := runtimeInputsFixture(t, nil)
	v.Mode, v.ApplyNotDispatched = "cancel101", true
	freezeRuntimeIntent(t, v, &a)
	db, mock := recoveryGateSQLMock(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	key, _ := recoveryGateKey(m.Binding)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	expectInitialRuntimeBoundary(mock)
	mock.ExpectExec(regexp.QuoteMeta(runtimeBoundaryCreate)).WillReturnError(errors.New("private ACK missing"))
	d := database{conn: conn, binding: m.Binding, gate: &recoveryGate{conn: conn, key: key, acquired: true}}
	r := receipt{}
	e = d.advanceRuntimeBoundary(t.Context(), m, v, a, fixtureSource, nil, &r)
	if e == nil || e.Error() != "runtime_database_boundary_create_outcome_unknown" || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten {
		t.Fatal("unknown CTAS granted a marker", e, r)
	}
	if _, e := os.Stat(v.Marker); !os.IsNotExist(e) {
		t.Fatal("unknown CTAS wrote a local marker")
	}
}

func TestRuntimeBoundaryCASAndUnknownCommitDoNotGrantStartup(t *testing.T) {
	for _, kind := range []string{"committed", "conflict", "commit_unknown"} {
		t.Run(kind, func(t *testing.T) {
			before, oldRaw := boundaryRecordFixture(t, fixtureMaterial(t))
			state := runtimeBoundaryState{Generation: before.Generation, Fingerprint: fingerprint(oldRaw)}
			m, _, a, v := runtimeInputsFixture(t, &state)
			v.Mode, v.ApplyNotDispatched = "cancel101", true
			freezeRuntimeIntent(t, v, &a)
			next, raw, e := nextRuntimeBoundary(m, v, a, fixtureSource)
			if e != nil {
				t.Fatal(e)
			}
			hash, _ := hex.DecodeString(fingerprint(raw))
			oldHash, _ := hex.DecodeString(state.Fingerprint)
			db, mock := recoveryGateSQLMock(t)
			conn, e := db.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = conn.Close() }()
			key, _ := recoveryGateKey(m.Binding)
			d := database{conn: conn, binding: m.Binding, gate: &recoveryGate{conn: conn, key: key, acquired: true}}
			expectGate := func() {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
			}
			mock.ExpectBegin()
			expectStoredRuntimeBoundary(mock, before, oldRaw)
			expectGate()
			count := int64(1)
			if kind == "conflict" {
				count = 0
			}
			mock.ExpectExec(regexp.QuoteMeta(runtimeBoundaryUpdate)).WithArgs(next.Generation, hash, raw, state.Generation, oldHash, oldRaw).WillReturnResult(sqlmock.NewResult(0, count))
			if kind == "conflict" {
				mock.ExpectRollback()
			} else {
				expectStoredRuntimeBoundary(mock, next, raw)
				expectGate()
				if kind == "commit_unknown" {
					mock.ExpectCommit().WillReturnError(errors.New("private lost ACK"))
				} else {
					mock.ExpectCommit()
				}
			}
			r := receipt{}
			e = d.updateRuntimeBoundary(t.Context(), runtimeBoundaryObservation{State: state, Record: &before, Raw: oldRaw}, next, raw, hash, m, v, a, fixtureSource, nil, &r)
			if (e == nil) != (kind == "committed") || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten || m.RuntimeBoundary != state {
				t.Fatal("CAS/unknown commit granted start or changed material", e, r)
			}
		})
	}
}

func TestRuntimeIntentFlagsCannotLeakIntoOtherActions(t *testing.T) {
	b := fixtureBinding()
	base := []string{"--mysql-database=" + b.SQLName, "--mongo-database=" + b.MongoName, "--mysql-identity-sha256=" + b.SQLIdentity, "--mongo-identity-sha256=" + b.MongoIdentity, "--mongo-ledger-uuid-sha256=" + b.MongoUUID}
	for _, action := range []string{"inspect", "capture", "verify-target", "restore", "verify-restored"} {
		args := append(append([]string{}, base...), "--action="+action)
		for _, flag := range []string{"--runtime-intent-file=", "--runtime-intent-sha256="} {
			if _, e := parseArgs(append(args, flag)); e == nil || e.Error() != "runtime_intent_action_mismatch" {
				t.Fatal("unrelated action accepted explicit intent", action, flag, e)
			}
		}
	}
	m, _, a, _ := runtimeInputsFixture(t, nil)
	r := qualifyingCaptureReceipt(m, a.materialHash)
	r.MarkerWritten = true
	if validateCaptureReceipt(r, m, a.materialHash, fixtureSource) == nil {
		t.Fatal("first-runtime receipt impersonated capture success")
	}
}
