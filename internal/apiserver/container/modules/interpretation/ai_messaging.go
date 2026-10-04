package interpretation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	client "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

type messagingLifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// Configure selects one host-owned transport before admitting commands. There is
// no MQ failure fallback to gRPC and no connection or scheduler created here.
func (m *Module) configureAIMessaging(o opts.AIWorkflowMessagingOptions, db *sql.DB, c *client.GovernanceClients) error {
	if m == nil || m.aiBridge == nil || db == nil || c == nil || c.Payloads == nil {
		return errors.New("AI MQ host dependencies unavailable")
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
	receiver := &store.MessagingEventReceiver{DB: db, Store: s, Keys: keys.Ring, Bodies: c.Payloads, Seal: seal}
	runtime, err := client.NewMessagingRuntime(o, db, s, receiver)
	if err != nil {
		return err
	}
	m.aiBridge.Store = commands
	m.aiEligibility = c.Commands
	m.aiMessagingRuntime = runtime
	m.aiMessagingReader = &store.MessagingReader{DB: db, Store: s}
	m.aiOperations = &app.OperationAdministration{Store: commands}
	if m.aiManagement != nil {
		m.aiManagement.Messages = commands
	}
	if m.aiParticipants != nil {
		m.aiParticipants.Messages = commands
	}
	return nil
}
