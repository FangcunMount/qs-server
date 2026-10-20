package eventtransaction

import (
	"context"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
)

var _ port.RetryCommitter = (*Committer)(nil)

func (o *Owner) RetryAborted(ctx context.Context, previous transaction.Execution, options transaction.ReserveOptions) (transaction.Reservation, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	if !options.Valid() {
		return transaction.Reservation{}, transaction.ErrInvalidPlan
	}
	if err := o.validatePlan(previous.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	if o.runtime == nil || !o.runtime.Stats().Ready {
		return transaction.Reservation{}, messagingruntime.ErrTransactionAdmissionClosed
	}
	read, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if o.Confirm(read, previous) != transaction.LocalAborted {
		return transaction.Reservation{}, transaction.ErrStaleExecution
	}
	return o.authority.Retry(read, transaction.RetryAuthorization{Previous: previous}, options)
}

func (c *Committer) RetryAborted(ctx context.Context, previous transaction.Execution, options transaction.ReserveOptions) (transaction.Reservation, error) {
	if err := c.validatePlan(previous.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return c.owner.RetryAborted(ctx, previous, options)
}
