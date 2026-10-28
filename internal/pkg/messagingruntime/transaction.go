package messagingruntime

import (
	"errors"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// NewTransactionalSender borrows exactly one original primary store. It reads
// configuration, without opening connections, starting work or applying DDL.
func NewTransactionalSender(o *options.MessagingOptions, owner string, store transaction.PreparedStore, topics []string, native NativeLimitReader) (*TransactionRuntime, error) {
	if o == nil || o.Provider != "rocketmq" || o.RocketMQ == nil {
		return nil, errors.New("explicit RocketMQ transaction configuration required")
	}
	binding, ok := o.RocketMQ.Transactions[owner]
	if !ok || store == nil || store.Binding() != (transaction.Binding{Owner: owner, StoreID: binding.StoreID, WriterEpoch: binding.WriterEpoch}) {
		return nil, errors.New("transaction sender requires the configured host primary binding")
	}
	config, err := o.RocketMQ.TransactionConfig(owner, topics)
	if err != nil {
		return nil, err
	}
	sender, err := rocketmq.NewTransactionalSender(config, store)
	if err != nil {
		return nil, err
	}
	routes, err := transactionRouteResolver(config)
	if err != nil {
		return nil, err
	}
	runtime := &TransactionRuntime{sender: sender, routes: routes, admission: o.RocketMQ.PublishingEnabled()}
	if err = attachProtocolMonitor(runtime, store, config.Limits, native); err != nil {
		return nil, err
	}
	return runtime, nil
}

// Routes exposes only immutable trusted route identities, never credentials.
// The resolver is assembled from the exact configuration used by this owner.
func (r *TransactionRuntime) Routes() *transport.RouteResolver {
	if r == nil {
		return nil
	}
	return r.routes
}
func transactionRouteResolver(config rocketmq.TransactionConfig) (*transport.RouteResolver, error) {
	convert := func(input []rocketmq.TransactionRoute) []transport.RouteBinding {
		rows := make([]transport.RouteBinding, 0, len(input))
		for _, row := range input {
			rows = append(rows, transport.RouteBinding{Topic: row.LogicalTopic, PhysicalTopic: row.PhysicalTopic, MessageType: transport.TransactionMessageType, Owner: config.Owner, RouteRevision: row.Revision, NamingScheme: row.NamingScheme})
		}
		return rows
	}
	return transport.NewRouteResolver(convert(config.Routes), convert(config.HistoricalRoutes))
}
