package aibridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	driver "github.com/nsqio/go-nsq"
)

func factsOnlyAIRuntime(t *testing.T, owner *runtimefacts.Owner) (*MessagingRuntime, opts.AIWorkflowMessagingOptions) {
	t.Helper()
	// Inert host objects are sufficient for New: no database, key or broker work
	// is allowed before the real host Start path. The canceled Start test below
	// also stops before preflight and never touches this borrowed database.
	db, s := &sql.DB{}, store.NewMessagingStore()
	receiver := &store.MessagingEventReceiver{DB: db, Store: s, Bodies: &MessagingPayloadClient{}, Keys: protected.Keyring{Decrypt: map[string]jose.JSONWebKey{"private-kid": {}}, Signers: map[string]protected.TrustedSigner{"private-signer": {}}}}
	receiver.Seal = func(pb.MessagingKind, string, string, string, string, *pb.MessagingBody) (*app.PreparedMessaging, error) {
		t.Fatal("unexpected business work")
		return nil, nil
	}
	o := opts.AIWorkflowMessagingOptions{Enabled: true, NSQD: map[string]string{"resolved-nsqd.internal:14450": "https://resolved-nsqd.internal:14451"}, SigningKeyFile: "private-signing-path", AIRecipientKeyFile: "private-recipient-path", DecryptKeyFiles: map[string]string{"private-kid": "private-decrypt-path"}, AISignerFiles: map[string]string{"private-signer": "private-trust-path"}}
	r, err := NewMessagingRuntime(o, db, s, receiver)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.BindRuntimeFacts(owner); err != nil {
		t.Fatal(err)
	}
	return r, o
}

func TestAIRuntimeFactsCaptureOwnedResolvedMapWithoutSecrets(t *testing.T) {
	owner, err := runtimefacts.New("apiserver", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	r, original := factsOnlyAIRuntime(t, owner)
	original.NSQD["resolved-nsqd.internal:14450"] = "https://mutated.invalid:1"
	cfg := driver.NewConfig()
	r.declareConsumerFacts(cfg)
	snapshot := owner.Snapshot()
	if len(snapshot.Transports) != 1 {
		t.Fatal("actual driver declaration missing")
	}
	fact := snapshot.Transports[0]
	if cfg.ClientID != owner.ClientID(aiConsumerFactsID) || fact.ClientID != cfg.ClientID || fact.Hostname != cfg.Hostname || !reflect.DeepEqual(fact.NSQDPairs, []runtimefacts.NSQDPair{{TCPAddress: "resolved-nsqd.internal:14450", HTTPAddress: "https://resolved-nsqd.internal:14451"}}) || fact.State != "declared" || len(fact.Subscriptions) != 0 || snapshot.BrokerConnectionsVerified || snapshot.ObservationComplete {
		t.Fatal("loaded map changed, inferred, or treated as actual connection")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-signing-path", "private-recipient-path", "private-decrypt-path", "private-trust-path", "private-kid", "private-signer", "auth_secret", "password", "dsn"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("runtime facts exposed non-allowlisted configuration")
		}
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if owner.Snapshot().Transports[0].State != "stopped" || r.DB != r.Receiver.DB {
		t.Fatal("stop did not invalidate observation or changed borrowed pool")
	}
	if err := owner.MarkStarted(aiConsumerFactsID); err == nil {
		t.Fatal("stopped facts restarted without a new process owner")
	}
}

func TestAIRuntimeCanceledStartCannotQualifyFacts(t *testing.T) {
	owner, err := runtimefacts.New("apiserver", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	r, _ := factsOnlyAIRuntime(t, owner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("original canceled-start behavior changed: %v", err)
	}
	snapshot := owner.Snapshot()
	if snapshot.ObservationComplete || len(snapshot.IncompleteReasons) == 0 || len(snapshot.Transports) != 0 || r.business != nil || r.failure != nil || r.publisher != nil || r.relayDone != nil {
		t.Fatal("failed Start created work or qualified observation")
	}
	other, err := runtimefacts.New("apiserver", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if err := r.BindRuntimeFacts(other); err == nil {
		t.Fatal("runtime rebound to another owner")
	}
}
