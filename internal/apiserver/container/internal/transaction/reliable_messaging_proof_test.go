//go:build reliable_messaging

package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Uses the actual qs-server transaction Runner and historical Outbox Store.
// Business collection is a test record, not AnswerSheet application acceptance.
func TestReliableMessagingOriginalMongoRunner(t *testing.T) {
	uri := os.Getenv("RM_QS_MONGO_URI")
	if uri == "" {
		t.Fatal("isolated replica-set URI required; must not skip")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	db := client.Database("rm_qs_original_runner")
	defer db.Drop(context.Background())
	require.NoError(t, db.CreateCollection(ctx, "rm_business"))
	require.NoError(t, db.CreateCollection(ctx, "rm_outbox"))
	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  proof:
    name: qs.rm.proof
events:
  rm.proof.created:
    topic: proof
    delivery: durable_outbox
    aggregate: Proof
    domain: proof
    handler: proof
`))
	require.NoError(t, err)
	old, err := eventoutbox.NewStoreWithTopicResolver(db, eventcatalog.NewCatalog(config))
	require.NoError(t, err)
	limiter := &transactionLimiterSpy{}
	runner := NewMongoRunner(db, MongoRunnerOptions{Boundary: "rm-proof", Limiter: limiter})
	hostAbort := errors.New("host abort")
	var committed message.Message
	for _, mode := range []string{"commit", "host-abort", "sdk-conflict"} {
		evt := event.New("rm.proof.created", "Proof", mode, map[string]any{"org_id": int64(7), "case": mode})
		attempts := 0
		err = runner.WithinTransaction(ctx, func(txCtx context.Context) error {
			attempts++
			sc, ok := txCtx.(mongo.SessionContext)
			if !ok {
				return errors.New("original Runner lost SessionContext")
			}
			if _, e := db.Collection("rm_business").InsertOne(sc, bson.M{"_id": mode}); e != nil {
				return e
			}
			if e := old.Stage(txCtx, evt); e != nil {
				return e
			}
			var row eventoutbox.OutboxPO
			if e := db.Collection("domain_event_outbox").FindOne(sc, bson.M{"event_id": evt.EventID()}).Decode(&row); e != nil {
				return e
			}
			// Simulate a historical producer's unknown extension without re-encoding
			// known fields. Both the old row and SDK proof must preserve these bytes.
			row.PayloadJSON = row.PayloadJSON[:len(row.PayloadJSON)-1] + `,"future_extension":{"preserve":true}}`
			if _, e := db.Collection("domain_event_outbox").UpdateOne(sc, bson.M{"event_id": row.EventID}, bson.M{"$set": bson.M{"payload_json": row.PayloadJSON}}); e != nil {
				return e
			}
			var envelope struct {
				ID         string `json:"id"`
				EventType  string `json:"eventType"`
				OccurredAt string `json:"occurredAt"`
			}
			if e := json.Unmarshal([]byte(row.PayloadJSON), &envelope); e != nil {
				return e
			}
			if envelope.ID != row.EventID || envelope.EventType != row.EventType {
				return errors.New("stored/envelope identity conflict")
			}
			intent, e := message.New(message.Input{Producer: "qs-server", ID: row.EventID, Destination: row.TopicName, EventType: row.EventType, SchemaVersion: "v1", Scope: "7", ContentType: "application/json", OccurredAt: envelope.OccurredAt, Payload: []byte(row.PayloadJSON)})
			if e != nil {
				return e
			}
			appender, e := sdkmongo.Bind(sc, db.Collection("rm_outbox"))
			if e != nil {
				return e
			}
			if e = appender.Append(intent, row.NextAttemptAt); e != nil {
				return e
			}
			if mode == "host-abort" {
				return hostAbort
			}
			if mode == "sdk-conflict" {
				changed := intent.Input()
				changed.Payload = []byte(`{"changed":true}`)
				conflict, e := message.New(changed)
				if e != nil {
					return e
				}
				return appender.Append(conflict, row.NextAttemptAt)
			}
			if attempts == 1 {
				return mongo.CommandError{Code: 112, Message: "controlled callback reentry", Labels: []string{"TransientTransactionError"}}
			}
			committed = intent
			return nil
		})
		switch mode {
		case "commit":
			require.NoError(t, err)
			require.Equal(t, 2, attempts)
		case "host-abort":
			require.ErrorIs(t, err, hostAbort)
		case "sdk-conflict":
			require.ErrorIs(t, err, outbox.ErrConflict)
		}
	}
	require.Equal(t, 3, limiter.acquired)
	require.Equal(t, 3, limiter.released)
	for _, name := range []string{"rm_business", "domain_event_outbox", "rm_outbox"} {
		n, e := db.Collection(name).CountDocuments(ctx, bson.M{})
		require.NoError(t, e)
		require.EqualValues(t, 1, n, name)
	}
	var oldRow eventoutbox.OutboxPO
	require.NoError(t, db.Collection("domain_event_outbox").FindOne(ctx, bson.M{}).Decode(&oldRow))
	require.Equal(t, oldRow.EventID, committed.Input().ID)
	require.Equal(t, oldRow.PayloadJSON, string(committed.Input().Payload))
	require.Contains(t, oldRow.PayloadJSON, "future_extension")
	store, err := sdkmongo.New(db.Collection("rm_outbox"))
	require.NoError(t, err)
	claims, err := store.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, committed.Fingerprint(), claims[0].Message.Fingerprint())
	require.NoError(t, store.Confirm(ctx, claims[0]))
}
