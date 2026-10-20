package eventtransaction

import (
	"context"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
)

// Committer borrows the shared owner and the original business runner. It
// adds the complete private artifact codec without creating another producer.
type Committer struct {
	owner    *Owner
	runner   OriginalRunner
	resolver catalog.TopicResolver
}

var _ port.Committer = (*Committer)(nil)

func NewCommitter(db *mongo.Database, owner *Owner, runner OriginalRunner, resolver catalog.TopicResolver) (*Committer, error) {
	if !owner.UsesDatabase(db) || runner == nil || resolver == nil {
		return nil, transaction.ErrBinding
	}
	return &Committer{owner: owner, runner: runner, resolver: resolver}, nil
}

func (c *Committer) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return shared.Artifact(p, ownerName, routeRevision)
}

func (c *Committer) validatePlan(p transaction.FrozenPlan) error {
	if _, err := c.Artifact(p); err != nil {
		return err
	}
	if err := c.owner.validatePlan(p); err != nil {
		return err
	}
	for _, branch := range p.Input().Branches {
		in := branch.Message.Input()
		topic, ok := c.resolver.GetTopicForEvent(in.EventType)
		if !ok || topic != in.Destination {
			return transaction.ErrBinding
		}
	}
	return nil
}

func (c *Committer) Reserve(ctx context.Context, q port.Request) (transaction.Reservation, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	p, err := shared.Freeze(q, c.resolver, ownerName, routeRevision)
	if err != nil {
		return transaction.Reservation{}, err
	}
	if err := c.validatePlan(p); err != nil {
		return transaction.Reservation{}, err
	}
	r, err := c.owner.Reserve(ctx, transaction.RequestFor(p), p)
	if err != nil {
		return r, err
	}
	if err := c.validatePlan(r.Execution.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return r, nil
}

func (c *Committer) Find(ctx context.Context, scope, key string) (transaction.Reservation, error) {
	r, err := c.owner.Find(ctx, scope, key)
	if err != nil {
		return r, err
	}
	if err := c.validatePlan(r.Execution.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return r, nil
}

func (c *Committer) Commit(ctx context.Context, e transaction.Execution, write func(context.Context) error) (transaction.Result, error) {
	if err := c.validatePlan(e.Plan); err != nil {
		return transaction.InitialResult(e), err
	}
	return c.owner.ExecuteOriginal(ctx, e, c.runner, write)
}

func (c *Committer) Confirm(ctx context.Context, e transaction.Execution) transaction.LocalOutcome {
	if c.validatePlan(e.Plan) != nil {
		return transaction.LocalUnknown
	}
	return c.owner.Confirm(ctx, e)
}
