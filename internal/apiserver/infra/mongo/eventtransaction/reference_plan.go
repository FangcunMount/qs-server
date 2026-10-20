package eventtransaction

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PlanForReference is a read-only audit boundary for an exact historical
// operation/fence/epoch. It does not use the request's current active execution,
// expose execution tokens, prepare messages or grant a new execution.
func (o *Owner) PlanForReference(ctx context.Context, ref transaction.Reference) (transaction.FrozenPlan, error) {
	if o == nil || !ref.Valid() || ref.Owner != ownerName || !o.HasTopic(ref.LogicalTopic) {
		return transaction.FrozenPlan{}, transaction.ErrBinding
	}
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.FrozenPlan{}, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// SDK Evidence validates the configured database binding on every new
	// authority read. A reference can never choose another database or owner.
	if _, err := o.Evidence(ctx, ref); err != nil {
		return transaction.FrozenPlan{}, err
	}
	key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
	var row struct {
		Plan  []byte `bson:"plan"`
		Fence int64  `bson:"fence"`
		Epoch string `bson:"writer_epoch"`
	}
	err := o.primary.Collection("rm_tx_execution").FindOne(ctx, bson.M{"_id": key[:]}, options.FindOne().SetProjection(bson.M{"plan": 1, "fence": 1, "writer_epoch": 1})).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return transaction.FrozenPlan{}, transaction.ErrNotFound
	}
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	plan, err := transaction.DecodePlan(row.Plan)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	if err := o.validatePlan(plan); err != nil {
		return transaction.FrozenPlan{}, err
	}
	actual, err := plan.Reference(ref.BranchID, uint64(row.Fence), row.Epoch)
	if err != nil || actual != ref {
		return transaction.FrozenPlan{}, transaction.ErrBinding
	}
	return plan, nil
}
