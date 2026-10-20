package interpretation

import (
	"context"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	client "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"gorm.io/gorm"
)

type messagingLifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// Configure selects one host-owned transport before admitting commands. There is
// no MQ failure fallback to gRPC and no connection or scheduler created here.
func (m *Module) configureAIMessaging(o opts.AIWorkflowMessagingOptions, primary *gorm.DB, c *client.GovernanceClients, shared *mysqlEventTx.Committer) error {
	if m == nil || m.aiBridge == nil || primary == nil || c == nil || c.Payloads == nil {
		return errors.New("AI MQ host dependencies unavailable")
	}
	db, err := primary.DB()
	if err != nil {
		return err
	}
	if shared != nil && (o.SelectedProvider() != "rocketmq" || !shared.UsesPrimary(primary) || !shared.HasTopic(app.CommandsTopic) || !shared.HasTopic(app.AcksTopic)) {
		return errors.New("AI command transaction owner must share original MySQL and RocketMQ route")
	}
	keys, err := client.LoadMessagingKeys(o)
	if err != nil {
		return err
	}
	seal := func(k pb.MessagingKind, id, agg, org, original string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		if original == "" {
			original = time.Now().In(time.FixedZone("UTC+8", 8*60*60)).Format(time.RFC3339Nano)
		}
		return app.ProtectMessaging(k, id, agg, "", org, original, b, keys.Signing, keys.Recipient)
	}
	s := store.NewMessagingStore()
	c.Payloads.Observe = func(ctx context.Context, kind string) error {
		return store.RecordPayloadObservation(ctx, db, kind)
	}
	commands := &store.MessagingCommandStore{Store: &store.Store{DB: db}, Messaging: s, Seal: seal}
	if shared != nil {
		commands.Transactions, err = store.NewTransactionCommands(primary, shared, seal)
		if err != nil {
			return err
		}
	}
	receiver := &store.MessagingEventReceiver{DB: db, Store: s, Keys: keys.Ring, Bodies: c.Payloads, Seal: seal}
	if shared != nil {
		if o.RocketMQ == nil {
			return errors.New("AI event transaction configuration unavailable")
		}
		receiver.Transactions, err = store.NewTransactionEvents(primary, shared, s, seal, o.RocketMQ.Transactions["qs.mysql"])
		if err != nil {
			return err
		}
		s.Archive = commands.Transactions.Archive()
	}
	runtime, err := client.NewMessagingRuntime(o, db, s, receiver)
	if err != nil {
		return err
	}
	runtime.CommandTransactions = commands.Transactions
	m.aiBridge.Store = commands
	m.aiEligibility = c.Commands
	m.aiMessagingRuntime = runtime
	m.aiMessagingReader = &store.MessagingReader{DB: db, Store: s}
	if commands.Transactions != nil {
		m.aiMessagingReader.Archive = commands.Transactions.Archive()
	}
	m.aiOperations = &app.OperationAdministration{Store: commands}
	if m.aiManagement != nil {
		m.aiManagement.Messages = commands
	}
	if m.aiParticipants != nil {
		m.aiParticipants.Messages = commands
	}
	return nil
}
