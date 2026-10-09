package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
)

// This policy fixture tests exact persisted NULL/owner/evidence semantics. It
// never constructs a public execution qualification or claims database proof.
func aiPersistenceTombstoneFixture(t *testing.T) ([]string, aiReverseRow, store.CommandRetirementEvidence) {
	t.Helper()
	_, f := aiHandoffNodeFixture(t)
	e := aiCommandHandoffEvidence(AIResolverBinding{OperationID: "12345-1", AdmissionRevision: 7}, f.bridge, f.legacy, time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC))
	e.LiveBodySHA256 = ""
	e.Sources = e.Sources[:1]
	e.Conclusion = "verified"
	e.Reason = "history_terminal_verified"
	e.VerificationMethod = "source_identity_hash_and_business_closure"
	e.BusinessTerminal = true
	e.ResponsibilityClosed = true
	columns := aiReverseSpecByTable("ai_messaging_operations").columns
	row := aiReverseRow{}
	for _, column := range columns {
		row[column] = nil
	}
	for key, value := range map[string]string{"command_id": e.CommandID, "kind": "1", "organization_id": e.OrganizationID, "subject_id": e.SubjectID, "resource_id": e.ResourceID, "aggregate_key": e.RequestID, "decision": "", "code": "", "retired": "1", "retired_at": "2026-10-09 01:02:03.000000"} {
		row[key] = []byte(value)
	}
	raw, err := json.MarshalIndent(e, "", " ")
	if err != nil {
		t.Fatal("synthetic dedicated evidence marshal")
	}
	row["retirement_evidence"] = raw
	return columns, row, e
}

func TestAICommandPersistenceTombstoneReadbackRejectsIdentityNullAndClosureConflicts(t *testing.T) {
	columns, row, e := aiPersistenceTombstoneFixture(t)
	if aiCommandRetirementOperationExpected(columns, row, e) != nil {
		t.Fatal("exact original ID tombstone rejected")
	}
	clone := func() aiReverseRow {
		out := aiReverseRow{}
		for k, v := range row {
			if v != nil {
				out[k] = bytes.Clone(v)
			} else {
				out[k] = nil
			}
		}
		return out
	}
	for _, column := range []string{"command_id", "kind", "organization_id", "subject_id", "resource_id", "aggregate_key", "retired", "decision", "code"} {
		t.Run("changed_"+column, func(t *testing.T) {
			changed := clone()
			changed[column] = []byte("conflict")
			if aiCommandRetirementOperationExpected(columns, changed, e) == nil {
				t.Fatal("wrong original identity/owner/state accepted")
			}
		})
	}
	for _, column := range []string{"body_sha256", "aggregate_sequence", "receipt_id", "receipt", "created_at", "decided_at"} {
		t.Run("null_to_empty_"+column, func(t *testing.T) {
			changed := clone()
			changed[column] = []byte{}
			if aiCommandRetirementOperationExpected(columns, changed, e) == nil {
				t.Fatal("absent original current fact silently invented")
			}
		})
	}
	for _, suffix := range []string{"{}", " null", " []"} {
		changed := clone()
		changed["retirement_evidence"] = append(changed["retirement_evidence"], suffix...)
		if aiCommandRetirementOperationExpected(columns, changed, e) == nil {
			t.Fatal("trailing evidence accepted")
		}
	}
	for _, change := range []func(*store.CommandRetirementEvidence){func(e *store.CommandRetirementEvidence) { e.BusinessTerminal = false }, func(e *store.CommandRetirementEvidence) { e.ResponsibilityClosed = false }, func(e *store.CommandRetirementEvidence) { e.CommandID = "different" }, func(e *store.CommandRetirementEvidence) { e.Conclusion = "transferred_verified" }} {
		altered := e
		change(&altered)
		raw, err := json.Marshal(altered)
		if err != nil {
			t.Fatal("synthetic altered evidence marshal")
		}
		changed := clone()
		changed["retirement_evidence"] = raw
		if aiCommandRetirementOperationExpected(columns, changed, e) == nil {
			t.Fatal("changed historical conclusion accepted")
		}
	}
}

func TestAICommandPersistenceMixedCurrentPendingNeverBecomesProviderTerminal(t *testing.T) {
	s, f := aiHandoffNodeFixture(t)
	before := make([]aiReverseNode, len(s.nodes))
	for i, n := range s.nodes {
		before[i] = *n
	}
	if aiCommandHandoffNodes(s, f.bridge, f.legacy) != nil {
		t.Fatal("real decoded original pair known current pending rejected")
	}
	for i, n := range s.nodes {
		if !reflect.DeepEqual(before[i], *n) {
			t.Fatal("qualification changed current pending facts")
		}
	}
	e := aiCommandHandoffEvidence(AIResolverBinding{OperationID: "12345-1", AdmissionRevision: 7}, f.bridge, f.legacy, time.Now().UTC())
	if e.ResponsibilityClosed || e.BusinessTerminal || e.Conclusion != "transferred_verified" {
		t.Fatal("publisher transfer became provider closure")
	}
}

func TestAICommandPersistencePublicZeroCopiedAndDTOCannotRecord(t *testing.T) {
	var batch AICommandPersistenceBatch
	if _, err := batch.Record(context.Background(), nil); err == nil {
		t.Fatal("zero object wrote")
	}
	if _, err := batch.VerifyReadback(context.Background(), nil); err == nil {
		t.Fatal("zero object verified")
	}
	if _, err := json.Marshal(&batch); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("private batch serialized")
	}
	batch.self = &batch
	copy := AICommandPersistenceBatch{self: &batch}
	if _, err := copy.Record(context.Background(), nil); err == nil {
		t.Fatal("copied object wrote")
	}
	var coordinator HistoricalCoordinator
	if page, err := coordinator.PrepareAICommandPersistencePage(context.Background(), nil, &AIExternalExecutionQualification{}); page != nil || err == nil {
		t.Fatal("ordinary report minted a page")
	}
	if proof, err := coordinator.SealAICommandPersistencePages(context.Background(), nil); proof != nil || err == nil {
		t.Fatal("missing source EOF minted batch")
	}
}

func TestAICommandPersistenceV2ProducerFactsCannotMintMixedQualification(t *testing.T) {
	raw := []byte(`{"protocol":"qs-ai-actual-execution-facts/v2","known_handoffs":[]}`)
	if _, err := aiExternalDecodeActualResult(raw); err == nil {
		t.Fatal("old readonly result admitted persistence protocol")
	}
}
