package eventtransaction

import (
	"context"
	"time"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var _ port.RetryCommitter = (*Committer)(nil)

func (c *Committer) RetryAborted(ctx context.Context, previous transaction.Execution, options transaction.ReserveOptions) (transaction.Reservation, error) {
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	if !options.Valid() {
		return transaction.Reservation{}, transaction.ErrInvalidPlan
	}
	if err := c.validatePlan(previous.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	if err := shared.RequireActiveRoutes(previous.Plan, c.routes); err != nil {
		return transaction.Reservation{}, err
	}
	if c.runtime == nil || !c.runtime.Stats().Ready {
		return transaction.Reservation{}, messagingruntime.ErrTransactionAdmissionClosed
	}
	read, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if c.Confirm(read, previous) != transaction.LocalAborted {
		return transaction.Reservation{}, transaction.ErrStaleExecution
	}
	// The SDK atomically rechecks exact prior fence/epoch and every Aborted
	// witness, retaining old records. A concurrent caller cannot win twice.
	return c.store.Retry(read, transaction.RetryAuthorization{Previous: previous}, options)
}
