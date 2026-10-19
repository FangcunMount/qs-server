package outcome

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

func outcomeTransactionInput(t *testing.T) NewRecordInput {
	t.Helper()
	in := NewRecordInput{ID: meta.FromUint64(1), OrgID: 11, AssessmentID: meta.FromUint64(2), TesteeID: 3, RunID: "2:1",
		Model:   ModelIdentity{Kind: modelcatalog.KindScale, Code: "S-1", Version: "1.0.0"},
		Runtime: RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange},
		Payload: []byte(`{ "primary" : { "value": 12 } }`), EvaluatedAt: time.Unix(100, 0).UTC()}
	m, err := message.New(message.Input{Producer: "qs-server", ID: "committed-event", Destination: "qs.evaluation.lifecycle", EventType: eventcatalog.EvaluationOutcomeCommitted,
		SchemaVersion: "v1", Scope: "org:11", ContentType: "application/json", OccurredAt: in.EvaluatedAt.Format(time.RFC3339Nano), Payload: []byte("private-frozen-artifact")})
	if err != nil {
		t.Fatal(err)
	}
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mysql", Scope: "org:11", OperationID: "outcome-operation", IdempotencyKey: "outcome/2:1",
		Branches: []transaction.BranchInput{{ID: "committed", Message: m, Wire: []byte("first-original-wire"), PhysicalTopic: "transaction-topic", RouteRevision: "m3-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := p.Reference("committed", 1, "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	fp := m.Fingerprint()
	standard := evidence.StandardReference{EventID: m.Input().ID, Producer: "qs-server", Destination: m.Input().Destination, EventType: m.Input().EventType, SchemaVersion: "v1",
		Scope: "org:11", ContentType: "application/json", OccurredAt: m.Input().OccurredAt, Fingerprint: hex.EncodeToString(fp[:])}
	in.CommittedTransactionEvidence, err = evidence.NewTransactional(standard, ref, BusinessBindingSHA256(in), in.EvaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestOutcomeTransactionEvidencePreservesIdentityAndOriginalPayload(t *testing.T) {
	in := outcomeTransactionInput(t)
	record, err := NewRecord(in)
	if err != nil {
		t.Fatal(err)
	}
	if record.CommittedEventEvidence() != nil || record.CommittedEventID() != "committed-event" || record.SchemaVersion() != CurrentSchemaVersion {
		t.Fatal("transaction evidence fabricated an Outbox reference or changed business schema")
	}
	if string(record.Payload()) != string(in.Payload) {
		t.Fatal("original result bytes changed")
	}
	in.CommittedTransactionEvidence.TransactionReference[0] ^= 1
	copy := record.CommittedTransactionEvidence()
	copy.TransactionReference[0] ^= 1
	if record.CommittedTransactionEvidence().Validate() != nil {
		t.Fatal("record shares mutable transaction evidence")
	}
}

func TestOutcomeTransactionEvidenceRejectsConflictsAndMixedProtocols(t *testing.T) {
	for _, kind := range []string{"payload", "tenant", "event", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			in := outcomeTransactionInput(t)
			switch kind {
			case "payload":
				in.Payload = []byte(`{"changed":true}`)
			case "tenant":
				in.OrgID++
			case "event":
				in.CommittedTransactionEvidence.Reference.EventType = "evaluation.failed"
			case "mixed":
				var err error
				in.CommittedEventEvidence, err = evidence.NewStandard(in.CommittedTransactionEvidence.Reference, BusinessBindingSHA256(in))
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := NewRecord(in); err == nil {
				t.Fatal("inconsistent outcome evidence accepted")
			}
		})
	}
}
