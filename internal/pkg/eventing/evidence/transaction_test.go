package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func transactionEvidence(t *testing.T) (*EventEvidenceV2, transaction.Reference) {
	t.Helper()
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	m, err := message.New(message.Input{Producer: "qs-server", ID: "first-event", Destination: "qs.survey.lifecycle", EventType: "answersheet.submitted", SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{"original":true}`)})
	require.NoError(t, err)
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: "org:7", OperationID: "operation", IdempotencyKey: "key", InputHash: sha256.Sum256([]byte("business-input")), Branches: []transaction.BranchInput{{ID: "submission", Message: m, Wire: []byte{0, 255, 3}, PhysicalTopic: "transaction.physical", RouteRevision: "m3-1"}}})
	require.NoError(t, err)
	tx, err := p.Reference("submission", transaction.MaxFence, "epoch")
	require.NoError(t, err)
	fp := m.Fingerprint()
	ref := StandardReference{EventID: "first-event", Producer: "qs-server", Destination: "qs.survey.lifecycle", EventType: "answersheet.submitted", SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Fingerprint: hex.EncodeToString(fp[:])}
	e, err := NewTransactional(ref, tx, BindingDigest("answersheet.submitted", String("7")), at)
	require.NoError(t, err)
	return e, tx
}

func TestTransactionEvidenceRetainsExactSDKReferenceAcrossJSONAndBSON(t *testing.T) {
	e, tx := transactionEvidence(t)
	for _, codec := range []struct {
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{{json.Marshal, json.Unmarshal}, {bson.Marshal, bson.Unmarshal}} {
		raw, err := codec.encode(e)
		require.NoError(t, err)
		var got EventEvidenceV2
		require.NoError(t, codec.decode(raw, &got))
		require.NoError(t, got.Validate())
		require.Equal(t, e.TransactionReference, got.TransactionReference)
		ref, err := transaction.DecodeReference(got.TransactionReference)
		require.NoError(t, err)
		require.Equal(t, tx, ref)
	}
	clone := e.Clone()
	clone.TransactionReference[0] ^= 1
	require.NoError(t, e.Validate())
	require.Error(t, clone.Validate())
}

func TestVersionedReaderDoesNotPromoteHistoricalOutboxEvidence(t *testing.T) {
	e, _ := transactionEvidence(t)
	legacy, err := NewStandard(e.Reference, e.BusinessBindingSHA256)
	require.NoError(t, err)
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	old, err := DecodeRecord(raw)
	require.NoError(t, err)
	require.NotNil(t, old.V1)
	require.Nil(t, old.V2)
	raw, err = json.Marshal(e)
	require.NoError(t, err)
	current, err := DecodeRecord(raw)
	require.NoError(t, err)
	require.Nil(t, current.V1)
	require.NotNil(t, current.V2)
	var v1 EventEvidenceV1
	require.NoError(t, json.Unmarshal(raw, &v1))
	require.Error(t, v1.Validate(), "legacy entry cannot silently treat v2 as Outbox proof")
	_, err = DecodeRecord([]byte(`{"version":3}`))
	require.Error(t, err)
}

func TestTransactionEvidenceRejectsWrongOwnerWireFenceAndBusinessIdentity(t *testing.T) {
	for _, mutate := range []func(*EventEvidenceV2, *transaction.Reference){
		func(_ *EventEvidenceV2, r *transaction.Reference) { r.Owner = "ai.mysql" },
		func(_ *EventEvidenceV2, r *transaction.Reference) { r.WireHash = [32]byte{} },
		func(_ *EventEvidenceV2, r *transaction.Reference) { r.MessageID = "another-event" },
		func(_ *EventEvidenceV2, r *transaction.Reference) { r.Scope = "org:8" },
		func(_ *EventEvidenceV2, r *transaction.Reference) { r.RouteRevision = "another-revision" },
		func(e *EventEvidenceV2, _ *transaction.Reference) { e.Origin = "backfilled_existing" },
		func(e *EventEvidenceV2, _ *transaction.Reference) { e.Reference.Fingerprint = string(make([]byte, 64)) },
	} {
		e, r := transactionEvidence(t)
		mutate(e, &r)
		raw, err := transaction.EncodeReference(r)
		require.NoError(t, err)
		e.TransactionReference = raw
		require.Error(t, e.Validate())
	}
	e, r := transactionEvidence(t)
	r.Fence = 0
	_, err := NewTransactional(e.Reference, r, e.BusinessBindingSHA256, e.RecordedAt)
	require.Error(t, err)
}
