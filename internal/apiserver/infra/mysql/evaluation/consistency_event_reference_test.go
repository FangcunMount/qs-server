package evaluation

import (
	"encoding/hex"
	"testing"
	"time"

	evalevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func testCommittedReference(t *testing.T, id, assessmentID uint64, eventID string) (*domainoutcome.Record, committedStandardRow) {
	t.Helper()
	at := time.Date(2026, 10, 8, 7, 0, 0, 123678901, time.UTC)
	in := domainoutcome.NewRecordInput{ID: meta.FromUint64(id), OrgID: 7, AssessmentID: meta.FromUint64(assessmentID), TesteeID: 21, RunID: meta.FromUint64(assessmentID).String() + ":1",
		Model:   domainoutcome.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "MBTI-16P", Version: "1.0.0", Title: "MBTI"},
		Runtime: domainoutcome.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindPoleComposition}, Payload: []byte(`{"result":"INTJ"}`), ReportInput: []byte(`{"input":"frozen"}`), SchemaVersion: 2, EvaluatedAt: at}
	evt := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: eventID, EventTypeValue: eventcatalog.EvaluationOutcomeCommitted, AggregateTypeValue: evalevent.AggregateType, AggregateIDValue: in.AssessmentID.String(), OccurredAtValue: at.Add(time.Second)},
		Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: in.OrgID, AssessmentID: int64(assessmentID), TesteeID: in.TesteeID, OutcomeID: in.ID.String(), EvaluationRunID: in.RunID, CommittedAt: at}}
	cfg, err := eventcatalog.Load("../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := standard.PrepareIntents([]event.DomainEvent{evt}, eventcatalog.NewCatalog(cfg), "api-server")
	if err != nil {
		t.Fatal(err)
	}
	reference := standard.ReferenceFromMessage(prepared[0].Message)
	in.CommittedEventEvidence, err = eventevidence.NewStandard(reference, domainoutcome.BusinessBindingSHA256(in))
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(in)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := hex.DecodeString(reference.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	msg := prepared[0].Message.Input()
	return record, committedStandardRow{Producer: msg.Producer, MessageID: msg.ID, Destination: msg.Destination, EventType: msg.EventType, SchemaVersion: msg.SchemaVersion, Scope: msg.Scope, ContentType: msg.ContentType, OccurredAt: msg.OccurredAt, Payload: msg.Payload, Fingerprint: fp, State: "pending"}
}
func TestCommittedReferenceChecksWireBusinessAndImmutableIdentity(t *testing.T) {
	record, row := testCommittedReference(t, 9001, 42, "committed-1")
	if err := verifyCommittedStandard(row, record); err != nil {
		t.Fatal(err)
	}
	po := outcomeToPO(record)
	po.EvaluatedAt = po.EvaluatedAt.Add(999999 * time.Nanosecond)
	reloaded, err := outcomeFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCommittedStandard(row, reloaded); err != nil {
		t.Fatalf("same persisted millisecond: %v", err)
	}
	row.State = "quarantined"
	if err := verifyCommittedStandard(row, record); err != nil {
		t.Fatalf("mutable delivery state: %v", err)
	}
	row.Scope = "org:8"
	if err := verifyCommittedStandard(row, record); err == nil {
		t.Fatal("cross-organization reference accepted")
	}
	po = outcomeToPO(record)
	po.PayloadJSON = `{"result":"different"}`
	if _, err := outcomeFromPO(po); err == nil {
		t.Fatal("business binding mutation accepted")
	}
}
