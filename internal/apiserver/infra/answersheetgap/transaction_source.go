package answersheetgap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type sourceExecution struct {
	ID        []byte            `bson:"_id"`
	Owner     string            `bson:"owner"`
	Plan      []byte            `bson:"plan"`
	Digest    []byte            `bson:"plan_digest"`
	Fence     int64             `bson:"fence"`
	Epoch     string            `bson:"writer_epoch"`
	State     transaction.State `bson:"state"`
	Witnesses []struct {
		Reference []byte            `bson:"reference"`
		State     transaction.State `bson:"state"`
	} `bson:"witnesses"`
}

// originalIntent reads only the exact source in the caller's primary snapshot.
// V2 never queries a retired publishing collection or starts a sender/Checker.
func (s *Scanner) originalIntent(ctx context.Context, sheet sheetRow) (outboxRow, recoveryConfirmation, error) {
	switch sheet.DurableAcceptance.SchemaVersion {
	case 1:
		if sheet.DurableAcceptance.TransactionEvidence != nil {
			return outboxRow{}, recoveryConfirmation{}, ErrUnsafeRecoverySource
		}
		id := bson.D{{Key: "producer", Value: producer}, {Key: "message_id", Value: sheet.DurableAcceptance.EventID}, {Key: "destination", Value: destination}}
		raw, err := s.mongo.Collection("rm_outbox").FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetHint("_id_").SetMaxTime(5*time.Second)).Raw()
		if err != nil {
			return outboxRow{}, recoveryConfirmation{}, err
		}
		var row outboxRow
		var confirmation recoveryConfirmation
		if err := bson.Unmarshal(raw, &row); err != nil {
			return row, confirmation, err
		}
		err = bson.Unmarshal(raw, &confirmation)
		return row, confirmation, err
	case 2:
		ref, err := gapTransactionReference(sheet)
		if err != nil {
			return outboxRow{}, recoveryConfirmation{}, err
		}
		if mongo.SessionFromContext(ctx) == nil {
			return outboxRow{}, recoveryConfirmation{}, ErrUnsafeRecoverySource
		}
		key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
		var row sourceExecution
		err = s.mongo.Collection("rm_tx_execution").FindOne(ctx, bson.M{"_id": key[:]}, options.FindOne().SetMaxTime(5*time.Second).SetProjection(bson.M{"owner": 1, "plan": 1, "plan_digest": 1, "fence": 1, "writer_epoch": 1, "state": 1, "witnesses.reference": 1, "witnesses.state": 1})).Decode(&row)
		if err != nil {
			return outboxRow{}, recoveryConfirmation{}, err
		}
		return currentIntent(sheet, row)
	default:
		return outboxRow{}, recoveryConfirmation{}, ErrUnsafeRecoverySource
	}
}

func gapTransactionReference(sheet sheetRow) (transaction.Reference, error) {
	a := sheet.DurableAcceptance
	proof := a.TransactionEvidence
	if a.SchemaVersion != 2 || a.EventEvidence != nil || proof == nil || proof.Validate() != nil {
		return transaction.Reference{}, ErrUnsafeRecoverySource
	}
	ref, err := transaction.DecodeReference(proof.TransactionReference)
	if err != nil || ref.Owner != "qs.mongo" || ref.Scope != fmt.Sprintf("org:%d", sheet.OrgID) || ref.MessageID != a.EventID || ref.OperationID != a.EventID || ref.LogicalTopic != destination || ref.BranchID != eventType || !proof.RecordedAt.Equal(a.AcceptedAt) {
		return transaction.Reference{}, ErrUnsafeRecoverySource
	}
	return ref, nil
}

func currentIntent(sheet sheetRow, row sourceExecution) (outboxRow, recoveryConfirmation, error) {
	bad := func() (outboxRow, recoveryConfirmation, error) {
		return outboxRow{}, recoveryConfirmation{}, ErrUnsafeRecoverySource
	}
	ref, err := gapTransactionReference(sheet)
	if err != nil {
		return bad()
	}
	plan, err := transaction.DecodePlan(row.Plan)
	if err != nil {
		return bad()
	}
	digest := plan.Digest()
	key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
	input := plan.Input()
	if row.Owner != ref.Owner || row.Fence <= 0 || !bytes.Equal(row.ID, key[:]) || !bytes.Equal(row.Digest, digest[:]) || input.Owner != ref.Owner || input.Scope != ref.Scope || input.OperationID != ref.OperationID || len(input.Branches) != 1 {
		return bad()
	}
	actual, err := plan.Reference(ref.BranchID, uint64(row.Fence), row.Epoch)
	if err != nil || actual != ref {
		return bad()
	}
	confirmation := recoveryConfirmation{TransactionReference: bytes.Clone(sheet.DurableAcceptance.TransactionEvidence.TransactionReference), PlanDigest: bytes.Clone(row.Digest), ExecutionState: row.State}
	matches := 0
	for _, witness := range row.Witnesses {
		if bytes.Equal(witness.Reference, confirmation.TransactionReference) {
			matches++
			confirmation.BranchState = witness.State
		}
	}
	if matches != 1 || transaction.Resolve(ref, &transaction.Evidence{Reference: ref, ExecutionState: confirmation.ExecutionState, BranchState: confirmation.BranchState}) != transaction.Commit {
		return bad()
	}
	b := input.Branches[0]
	proof := sheet.DurableAcceptance.TransactionEvidence
	if eventmessage.ReferenceFromMessage(b.Message) != proof.Reference {
		return bad()
	}
	in := b.Message.Input()
	in.Payload = bytes.Clone(b.Wire)
	wireMessage, err := message.New(in)
	if err != nil {
		return bad()
	}
	wireRef := eventmessage.ReferenceFromMessage(wireMessage)
	envelope, err := eventmessage.VerifyReference(in, wireRef.Fingerprint, wireRef)
	if err != nil || !envelope.OccurredAt.Truncate(time.Millisecond).Equal(sheet.DurableAcceptance.AcceptedAt) {
		return bad()
	}
	var data payload.AnswerSheetSubmittedData
	if json.Unmarshal(envelope.Data, &data) != nil {
		return bad()
	}
	binding, err := eventevidencebinding.AnswerSheet(data)
	if err != nil || binding != proof.BusinessBindingSHA256 {
		return bad()
	}
	fingerprint := wireMessage.Fingerprint()
	intent := outboxRow{Producer: in.Producer, MessageID: in.ID, Destination: in.Destination, EventType: in.EventType, SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Payload: in.Payload, Fingerprint: fingerprint[:], State: string(transaction.Committed)}
	if validateOriginal(sheet, intent) != nil {
		return bad()
	}
	return intent, confirmation, nil
}
