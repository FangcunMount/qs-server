package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These are static candidate unit tests, not native or production proofs. The
// originals are decoded through the existing complete source fixture/parser;
// synthetic typed graph nodes exercise policy only, never mint a public batch.
func aiHandoffNodeFixture(t *testing.T) (*AIReverseSnapshot, authFixture) {
	t.Helper()
	f := sourceAuthFixture(t, true, true)
	b, l := f.bridge, f.legacy
	s := &AIReverseSnapshot{byTable: map[string]map[string]*aiReverseNode{}}
	for _, name := range []string{AIBridgeCommandSource, AILegacyCommandSource, "ai_messaging_operations", "ai_messaging_outbox", "ai_bridge_requests"} {
		n := &aiReverseNode{id: b.CommandID, request: b.RequestID, aggregate: b.RequestID, resource: b.ResourceID, org: b.OrganizationID, subject: b.SubjectID, bodyHash: l.Transport.MessagingBodySHA256, sequence: 1, observation: AIReverseObservation{Store: name, Scope: "retirement_related"}}
		s.byTable[name] = map[string]*aiReverseNode{b.CommandID: n}
		s.nodes = append(s.nodes, n)
	}
	s.byTable[AIBridgeCommandSource][b.CommandID].sourceRowHash = b.Source.Digest.SHA256
	s.byTable[AILegacyCommandSource][b.CommandID].sourceRowHash = l.Source.Digest.SHA256
	s.byTable["ai_bridge_requests"][b.CommandID].state = "pending"
	s.byTable["ai_bridge_requests"][b.CommandID].observation.Unfinished = true
	s.byTable["ai_messaging_operations"][b.CommandID].observation.Unfinished = true
	s.byTable["ai_messaging_outbox"][b.CommandID].state = "staged"
	s.byTable["ai_messaging_outbox"][b.CommandID].observation.Unfinished = true
	return s, f
}

func TestAICommandHandoffKnownPendingPreservesCurrentResponsibility(t *testing.T) {
	s, f := aiHandoffNodeFixture(t)
	before := make([]aiReverseNode, len(s.nodes))
	for i, node := range s.nodes {
		before[i] = *node
	}
	if err := aiCommandHandoffNodes(s, f.bridge, f.legacy); err != nil {
		t.Fatal("known original handoff rejected")
	}
	after := make([]aiReverseNode, len(s.nodes))
	for i, node := range s.nodes {
		after[i] = *node
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("classification mutated current protocol")
	}
	conclusion := aiCommandHandoffEvidence(AIResolverBinding{OperationID: "12345-1", AdmissionRevision: 7}, f.bridge, f.legacy, time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC))
	if conclusion.BusinessTerminal || conclusion.ResponsibilityClosed || conclusion.Conclusion != "transferred_verified" || conclusion.CommandID != f.bridge.CommandID || conclusion.RequestID != f.bridge.RequestID || conclusion.LiveBodySHA256 != f.legacy.Transport.MessagingBodySHA256 || conclusion.Sources[0].BytesSHA256 != f.bridge.PayloadBytesDigest.SHA256 || conclusion.Sources[1].BusinessPayloadHash != f.legacy.WriterPayloadDigest.SHA256 {
		t.Fatal("handoff changed original identity or claimed provider closure")
	}
}

func TestAICommandHandoffRejectsHeldUnknownMissingMappingAndIdentityChange(t *testing.T) {
	cases := []struct {
		name   string
		change func(*AIReverseSnapshot, *authFixture)
	}{
		{"operation_held", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable["ai_messaging_operations"][f.bridge.CommandID].observation.Held = true
		}},
		{"outbox_held", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable["ai_messaging_outbox"][f.bridge.CommandID].observation.Held = true
		}},
		{"invalid_receipt", func(s *AIReverseSnapshot, f *authFixture) {
			s.nodes = append(s.nodes, &aiReverseNode{command: f.bridge.CommandID, observation: AIReverseObservation{Store: "ai_messaging_inbox", Invalid: true}})
		}},
		{"pending_old_publisher", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable[AIBridgeCommandSource][f.bridge.CommandID].observation.Unfinished = true
		}},
		{"source_row_changed", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable[AIBridgeCommandSource][f.bridge.CommandID].sourceRowHash = strings.Repeat("d", 64)
		}},
		{"budget_changed", func(s *AIReverseSnapshot, f *authFixture) { f.legacy.Transport.SourceAttempts++ }},
		{"different_org", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable["ai_messaging_operations"][f.bridge.CommandID].org = "2"
		}},
		{"retired_live_id", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable["ai_messaging_operations"][f.bridge.CommandID].retired = true
		}},
		{"no_actual_mapping", func(s *AIReverseSnapshot, f *authFixture) {
			delete(s.byTable[AILegacyCommandSource], f.bridge.CommandID)
		}},
		{"delivered_is_not_handoff", func(s *AIReverseSnapshot, f *authFixture) { *f.bridge.Transport.Delivered = true }},
		{"unknown_owner_scope", func(s *AIReverseSnapshot, f *authFixture) {
			s.byTable["ai_bridge_requests"][f.bridge.CommandID].observation.Scope = "unknown"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, f := aiHandoffNodeFixture(t)
			tc.change(s, &f)
			if aiCommandHandoffNodes(s, f.bridge, f.legacy) == nil {
				t.Fatal("conflicting/unknown handoff accepted")
			}
		})
	}
}

func TestAICommandHandoffReadbackPreservesAllOriginalOperationColumns(t *testing.T) {
	_, f := aiHandoffNodeFixture(t)
	conclusion := aiCommandHandoffEvidence(AIResolverBinding{OperationID: "12345-1", AdmissionRevision: 7}, f.bridge, f.legacy, time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC))
	columns := aiReverseSpecByTable("ai_messaging_operations").columns
	before := aiReverseRow{}
	for _, col := range columns {
		before[col] = []byte("synthetic-original-" + col)
	}
	before["command_id"] = []byte(conclusion.CommandID)
	before["receipt"] = nil
	before["decided_at"] = nil
	before["retirement_evidence"] = nil
	fresh := func() aiReverseRow {
		after := aiReverseRow{}
		for col, raw := range before {
			if raw != nil {
				after[col] = bytes.Clone(raw)
			} else {
				after[col] = nil
			}
		}
		raw, err := json.MarshalIndent(conclusion, "", "  ")
		if err != nil {
			t.Fatal("synthetic evidence marshal")
		}
		after["retirement_evidence"] = raw
		return after
	}
	if err := aiCommandHandoffOperationUnchanged(columns, before, fresh(), conclusion); err != nil {
		t.Fatal("only evidence changed but readback rejected")
	}
	for _, col := range columns {
		if col == "retirement_evidence" {
			continue
		}
		t.Run("changed_"+col, func(t *testing.T) {
			after := fresh()
			after[col] = []byte("changed")
			if aiCommandHandoffOperationUnchanged(columns, before, after, conclusion) == nil {
				t.Fatal("original operation column changed")
			}
		})
	}
	for _, col := range []string{"receipt", "decided_at"} {
		t.Run("null_to_empty_"+col, func(t *testing.T) {
			after := fresh()
			after[col] = []byte{}
			if aiCommandHandoffOperationUnchanged(columns, before, after, conclusion) == nil {
				t.Fatal("NULL became empty")
			}
		})
	}
	for _, suffix := range []string{"{}", "\nnull", "\n[1]"} {
		after := fresh()
		after["retirement_evidence"] = append(after["retirement_evidence"], suffix...)
		if aiCommandHandoffOperationUnchanged(columns, before, after, conclusion) == nil {
			t.Fatal("trailing evidence accepted")
		}
	}
	after := fresh()
	after["retirement_evidence"] = bytes.Replace(after["retirement_evidence"], []byte(`"business_terminal": false`), []byte(`"business_terminal": true`), 1)
	if aiCommandHandoffOperationUnchanged(columns, before, after, conclusion) == nil {
		t.Fatal("provider closure was invented")
	}
}

func TestAICommandHandoffPublicZeroAndCopiedBatchCannotWrite(t *testing.T) {
	var coordinator *HistoricalCoordinator
	if _, err := coordinator.PrepareAICommandHandoffBatch(context.Background(), nil, nil); !errors.Is(err, ErrAILocalBinding) {
		t.Fatal("zero coordinator accepted")
	}
	original := &AICommandHandoffBatch{}
	original.self = original
	copied := &AICommandHandoffBatch{self: original}
	for _, p := range []*AICommandHandoffBatch{nil, {}, copied} {
		if _, err := p.Record(context.Background(), nil); !errors.Is(err, ErrAILocalBinding) {
			t.Fatal("zero/copied batch accepted")
		}
	}
	if _, err := original.MarshalJSON(); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque plan was exported")
	}
	if err := original.UnmarshalJSON([]byte(`{"authority":true}`)); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("summary imported as authority")
	}
}
