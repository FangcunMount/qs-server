package transactionplan

import (
	"errors"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
)

// ErrHistoricalExecution keeps a readable historical plan from acquiring new
// fences or being silently rebuilt by a consumer or due-dispatch retry. Reading
// exact committed evidence remains valid; crossing routes needs explicit Replan.
var ErrHistoricalExecution = errors.New("historical message route requires explicit host replan")

func RequireActiveRoutes(plan transaction.FrozenPlan, routes *transport.RouteResolver) error {
	if !plan.Valid() {
		return transaction.ErrInvalidPlan
	}
	if routes == nil {
		return transaction.ErrBinding
	}
	in := plan.Input()
	for _, branch := range in.Branches {
		key := transport.RouteKey{Topic: branch.Message.Input().Destination, MessageType: transport.TransactionMessageType, Owner: in.Owner, RouteRevision: branch.RouteRevision}
		if _, err := routes.RecordedRoute(key, branch.PhysicalTopic); err != nil {
			return transaction.ErrBinding
		}
		active, err := routes.PublishRoute(key)
		if err != nil {
			return transaction.ErrBinding
		}
		if active.PhysicalTopic != branch.PhysicalTopic {
			return ErrHistoricalExecution
		}
	}
	return nil
}
