package hotrank

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type rankExecution struct {
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

func hotRankTransactionReference(sheet sourceSheet) (transaction.Reference, error) {
	a := sheet.DurableAcceptance
	if a.SchemaVersion != 2 || a.EventEvidence != nil || a.TransactionEvidence == nil || a.TransactionEvidence.Validate() != nil {
		return transaction.Reference{}, ErrRebuildConflict
	}
	ref, err := transaction.DecodeReference(a.TransactionEvidence.TransactionReference)
	if err != nil || ref.Owner != "qs.mongo" || ref.MessageID != a.EventID || ref.Scope != fmt.Sprintf("org:%d", sheet.OrgID) || ref.LogicalTopic != "qs.evaluation.lifecycle" || ref.BranchID != "answersheet.submitted" || !a.AcceptedAt.Equal(a.TransactionEvidence.RecordedAt) {
		return transaction.Reference{}, ErrRebuildConflict
	}
	return ref, nil
}

// transactionIntents reads exact immutable executions in the same day snapshot.
// It never starts a sender, changes an execution, or creates a new event identity.
func (s *MongoDaySource) transactionIntents(ctx context.Context, sheets []sourceSheet) (map[string]sourceIntent, error) {
	result := map[string]sourceIntent{}
	wanted := map[string][]sourceSheet{}
	ids := bson.A{}
	for _, sheet := range sheets {
		a := sheet.DurableAcceptance
		if a.SchemaVersion != 2 {
			if a.TransactionEvidence != nil {
				return nil, ErrRebuildConflict
			}
			continue
		}
		ref, err := hotRankTransactionReference(sheet)
		if err != nil {
			return nil, err
		}
		key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
		id := hex.EncodeToString(key[:])
		if _, exists := wanted[id]; !exists {
			ids = append(ids, key[:])
		}
		wanted[id] = append(wanted[id], sheet)
	}
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := s.db.Collection("rm_tx_execution").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetBatchSize(10).SetLimit(int64(len(ids)+1)).SetMaxTime(5*time.Second).SetProjection(bson.M{"owner": 1, "plan": 1, "plan_digest": 1, "fence": 1, "writer_epoch": 1, "state": 1, "witnesses.reference": 1, "witnesses.state": 1}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row rankExecution
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		group, exists := wanted[hex.EncodeToString(row.ID)]
		if !exists {
			return nil, ErrRebuildConflict
		}
		for _, sheet := range group {
			intent, err := transactionIntent(sheet, row)
			if err != nil {
				return nil, err
			}
			if _, exists := result[intent.ID]; exists {
				return nil, ErrRebuildConflict
			}
			result[intent.ID] = intent
		}
		delete(wanted, hex.EncodeToString(row.ID))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("original hot rank transaction coverage is incomplete")
	}
	return result, nil
}

func transactionIntent(sheet sourceSheet, row rankExecution) (sourceIntent, error) {
	ref, err := hotRankTransactionReference(sheet)
	if err != nil {
		return sourceIntent{}, err
	}
	proof := sheet.DurableAcceptance.TransactionEvidence
	plan, err := transaction.DecodePlan(row.Plan)
	if err != nil {
		return sourceIntent{}, err
	}
	digest := plan.Digest()
	input := plan.Input()
	key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
	if row.Owner != "qs.mongo" || row.Fence <= 0 || !bytes.Equal(row.ID, key[:]) || !bytes.Equal(row.Digest, digest[:]) || input.Owner != ref.Owner || input.Scope != ref.Scope || input.OperationID != sheet.DurableAcceptance.EventID || len(input.Branches) != 1 {
		return sourceIntent{}, ErrRebuildConflict
	}
	actual, err := plan.Reference(ref.BranchID, uint64(row.Fence), row.Epoch)
	if err != nil || actual != ref {
		return sourceIntent{}, ErrRebuildConflict
	}
	matches := 0
	for _, w := range row.Witnesses {
		if bytes.Equal(w.Reference, proof.TransactionReference) {
			matches++
			if transaction.Resolve(ref, &transaction.Evidence{Reference: ref, ExecutionState: row.State, BranchState: w.State}) != transaction.Commit {
				return sourceIntent{}, ErrRebuildConflict
			}
		}
	}
	if matches != 1 {
		return sourceIntent{}, ErrRebuildConflict
	}
	for _, branch := range plan.Input().Branches {
		if branch.ID != ref.BranchID {
			continue
		}
		if standard.ReferenceFromMessage(branch.Message) != proof.Reference {
			return sourceIntent{}, ErrRebuildConflict
		}
		in := branch.Message.Input()
		in.Payload = bytes.Clone(branch.Wire)
		wireMessage, err := message.New(in)
		if err != nil {
			return sourceIntent{}, err
		}
		wireRef := standard.ReferenceFromMessage(wireMessage)
		env, err := standard.VerifyReference(in, wireRef.Fingerprint, wireRef)
		if err != nil {
			return sourceIntent{}, err
		}
		var data payload.AnswerSheetSubmittedData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return sourceIntent{}, err
		}
		binding, err := eventevidencebinding.AnswerSheet(data)
		if err != nil || binding != proof.BusinessBindingSHA256 {
			return sourceIntent{}, ErrRebuildConflict
		}
		fingerprint := wireMessage.Fingerprint()
		return sourceIntent{Producer: in.Producer, ID: in.ID, Destination: in.Destination, EventType: in.EventType, SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Payload: in.Payload, Fingerprint: fingerprint[:]}, nil
	}
	return sourceIntent{}, ErrRebuildConflict
}
