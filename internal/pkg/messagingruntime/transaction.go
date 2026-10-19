package messagingruntime

import (
	"errors"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// NewTransactionalSender borrows exactly one original primary store. It reads
// configuration, without opening connections, starting work or applying DDL.
func NewTransactionalSender(o *options.MessagingOptions, owner string, store transaction.PreparedStore, topics []string) (*TransactionRuntime, error) {
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
	return &TransactionRuntime{sender: sender, admission: o.RocketMQ.PublishingEnabled()}, nil
}
