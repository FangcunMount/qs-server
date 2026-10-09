package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

// This synthetic serializer fixture never creates a borrowed SQL snapshot,
// source capability, qualification or completion receipt.
func reverseDiagnosticFixture() retirement.AIReverseSummary {
	return retirement.AIReverseSummary{MigrationVersion: 99, ActualReadOnlyRR: true, WholeLedgerEOF: true, Ledgers: make([]retirement.AIReverseLedgerSummary, 14), DatabaseIdentitySHA256: strings.Repeat("a", 64), DataSHA256: strings.Repeat("b", 64), BusinessAnchorsSHA256: strings.Repeat("c", 64), SourceScopeSHA256: strings.Repeat("d", 64), ExternalOriginRequired: true, ExternalQSAIClosureRequired: true, StoredWireAuthenticationRequired: true, WriterFenceRequired: true}
}
func TestHistoryAIReverseDiagnosticCoverageNeverGrantsAuthority(t *testing.T) {
	for _, which := range []string{"valid_local", "wrong_head", "missing_ledger", "missing_eof", "not_rrro", "missing_scope", "drop", "cas", "global", "external_origin", "external_ai", "wire_authentication", "writer_fence"} {
		t.Run(which, func(t *testing.T) {
			r := reverseDiagnosticFixture()
			switch which {
			case "wrong_head":
				r.MigrationVersion = 98
			case "missing_ledger":
				r.Ledgers = r.Ledgers[:13]
			case "missing_eof":
				r.WholeLedgerEOF = false
			case "not_rrro":
				r.ActualReadOnlyRR = false
			case "missing_scope":
				r.SourceAuthenticationRequired = true
			case "drop":
				r.DropReady = true
			case "cas":
				r.CASAuthority = true
			case "global":
				r.GlobalReverseQualified = true
			case "external_origin":
				r.ExternalOriginRequired = false
			case "external_ai":
				r.ExternalQSAIClosureRequired = false
			case "wire_authentication":
				r.StoredWireAuthenticationRequired = false
			case "writer_fence":
				r.WriterFenceRequired = false
			}
			facts, e := stableAIReverseObservation(r)
			if which == "valid_local" {
				if e != nil || len(facts.Ledgers) != 14 {
					t.Fatal("local diagnostic serializer")
				}
				empty := emptyReadiness(nil)
				if empty.CompletedReadOnlyPipeline || empty.DropReady || empty.CASComplete || empty.FullExternalAIClosureVerified || empty.WriterFenceProven {
					t.Fatal("diagnostic serialization created authority")
				}
			} else if e == nil {
				t.Fatal("incomplete or false-authority diagnostics accepted")
			}
		})
	}
}
func TestHistoryAIReverseSummaryEqualityCannotSubstituteForFreshCapability(t *testing.T) {
	testSource(t)
	first := &epochResult{sql: &retirement.SQLResponsibilitySnapshot{}, mongo: &retirement.MongoResponsibilitySnapshot{}, reasons: map[string]uint64{}}
	second := &epochResult{sql: &retirement.SQLResponsibilitySnapshot{}, mongo: &retirement.MongoResponsibilitySnapshot{}, reasons: map[string]uint64{}}
	if compareEpochs(first, second) != nil {
		t.Fatal("equal diagnostic fixtures should compare equal")
	}
	if proof, e := first.recheckAIReverse(context.Background(), second, &approvedInputs{}); proof != nil || safeCategory(e) != "history_ai_reverse_independent_epoch_failed" {
		t.Fatal("equal DTO facts fabricated actual fresh proof")
	}
	first.aiReverse = &retirement.AIReverseSnapshot{}
	second.aiReverse = &retirement.AIReverseSnapshot{}
	first.aiReverseCoordinator = &retirement.HistoricalCoordinator{}
	second.aiReverseCoordinator = &retirement.HistoricalCoordinator{}
	path, hash, _ := requestFixture(t)
	a, e := loadInputs(t.Context(), path, hash, "123-1", "125-1")
	if e != nil {
		t.Fatal("private fixture load")
	}
	defer func() {
		if a.close() != nil {
			t.Error("owned input close")
		}
	}()
	if proof, e := first.recheckAIReverse(t.Context(), second, a); proof != nil || safeCategory(e) != "history_ai_reverse_independent_epoch_failed" {
		t.Fatal("zero opaque snapshot/coordinator created fresh proof")
	}
}
func TestHistoryAIReverseEpochFactsParticipateInComparison(t *testing.T) {
	first := &epochResult{sql: &retirement.SQLResponsibilitySnapshot{}, mongo: &retirement.MongoResponsibilitySnapshot{}, reasons: map[string]uint64{}}
	second := &epochResult{sql: &retirement.SQLResponsibilitySnapshot{}, mongo: &retirement.MongoResponsibilitySnapshot{}, reasons: map[string]uint64{}}
	facts, e := stableAIReverseObservation(reverseDiagnosticFixture())
	if e != nil {
		t.Fatal(e)
	}
	first.aiReverseFacts = facts
	second.aiReverseFacts = facts
	if compareEpochs(first, second) != nil {
		t.Fatal("identical body-free facts changed")
	}
	second.aiReverseFacts.DataSHA256 = strings.Repeat("f", 64)
	if safeCategory(compareEpochs(first, second)) != "history_independent_epoch_facts_changed" {
		t.Fatal("inside-upper AI raw drift omitted from compare")
	}
}
func TestHistoryAIReversePublicFieldsAreFixedBodyFreeDiagnostics(t *testing.T) {
	r := emptyReadiness(nil)
	r.AIReverseGlobal = aiReverseGlobalSummary{LedgerCount: 14, Rows: 10, Related: 2, Outside: 8, Unknown: 0, Blocking: 1, OutsideActive: 1, DataSHA256: strings.Repeat("b", 64), SourceScopeSHA256: strings.Repeat("d", 64), WholeLedgerEOF: true}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var decoded map[string]json.RawMessage
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatal("diagnostic JSON")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(decoded["ai_reverse_global"], &fields) != nil || len(fields) != 11 {
		t.Fatal("fixed reverse public contract")
	}
	for _, key := range []string{"ledger_count", "rows", "retirement_related", "outside_retirement", "unknown", "blocking", "outside_active", "data_sha256", "source_scope_sha256", "whole_ledger_eof", "independent_epoch_rechecked"} {
		if fields[key] == nil {
			t.Fatal("missing fixed diagnostic field")
		}
	}
	for _, key := range []string{"command_id", "request_id", "session_id", "run_id", "body", "wire", "projection"} {
		if fields[key] != nil {
			t.Fatal("private AI identity/body exposed")
		}
	}
	if r.CompletedReadOnlyPipeline || r.IndependentEpochs != 0 || r.CASComplete || r.DropReady {
		t.Fatal("diagnostic DTO created completion/write authority")
	}
}

// A DTO, a zero anchor or a copied/retained first graph cannot manufacture the
// actual joint recheck capability, even if all public observations are equal.
func TestHistoryAIReverseGraphlessAnchorRejectsZeroAndRetainedFirstGraphs(t *testing.T) {
	testSource(t)
	first := &epochResult{reverseAnchor: &retirement.AIReverseRecheckAnchor{}, reasons: map[string]uint64{}}
	second := &epochResult{sql: &retirement.SQLResponsibilitySnapshot{}, mongo: &retirement.MongoResponsibilitySnapshot{}, aiReverse: &retirement.AIReverseSnapshot{}, aiReverseCoordinator: &retirement.HistoricalCoordinator{}, reasons: map[string]uint64{}}
	path, hash, _ := requestFixture(t)
	a, e := loadInputs(t.Context(), path, hash, "123-1", "125-1")
	if e != nil {
		t.Fatal("private fixture load")
	}
	defer func() {
		if a.close() != nil {
			t.Error("owned input close")
		}
	}()
	for _, which := range []string{"zero_anchor", "origin", "sql", "mongo", "reverse", "coordinator", "old_anchor"} {
		t.Run(which, func(t *testing.T) {
			old := *first
			switch which {
			case "origin":
				old.origin = &retirement.SourceOriginEpoch{}
			case "sql":
				old.sql = &retirement.SQLResponsibilitySnapshot{}
			case "mongo":
				old.mongo = &retirement.MongoResponsibilitySnapshot{}
			case "reverse":
				old.aiReverse = &retirement.AIReverseSnapshot{}
			case "coordinator":
				old.aiReverseCoordinator = &retirement.HistoricalCoordinator{}
			case "old_anchor":
				old.anchor = &retirement.FreshRecheckAnchor{}
			}
			proof, err := old.recheckAIReverse(t.Context(), second, a)
			if proof != nil || safeCategory(err) != "history_ai_reverse_independent_epoch_failed" {
				t.Fatal("retained graph/zero anchor accepted")
			}
		})
	}
	if first.compactOrigin(t.Context()) == nil {
		t.Fatal("summary/zero anchor created first-epoch compaction")
	}
}
