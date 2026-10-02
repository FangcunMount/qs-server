//go:build reliable_messaging

package transaction

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestStandardMongoAppenderOriginalRunner(t *testing.T) {
	uri := os.Getenv("RM_QS_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test replica-set URI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	db := client.Database("rm_qs_standard_runner")
	defer db.Drop(context.Background())
	require.NoError(t, db.CreateCollection(ctx, "rm_business"))
	require.NoError(t, db.CreateCollection(ctx, "rm_outbox"))
	outboxColl := db.Collection("rm_outbox")
	_, err = outboxColl.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	limiter := &transactionLimiterSpy{}
	runner := NewMongoRunner(db, MongoRunnerOptions{Boundary: "rm-standard-proof", Limiter: limiter})
	due := time.Now().Add(10 * time.Minute).Truncate(time.Millisecond)
	occurredAt := time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano)
	makeMessage := func(payload string) message.Message {
		t.Helper()
		m, err := message.New(message.Input{
			Producer: "qs-server", ID: "m4-standard-mongo-one", Destination: "qs.evaluation.lifecycle",
			EventType: "answersheet.submitted", SchemaVersion: "v1", Scope: "org:7",
			ContentType: "application/json", OccurredAt: occurredAt, Payload: []byte(payload),
		})
		require.NoError(t, err)
		return m
	}
	stage := func(factID string, intent message.Message, finalErr error) error {
		return runner.WithinTransaction(ctx, func(txCtx context.Context) error {
			sc, ok := txCtx.(mongo.SessionContext)
			if !ok {
				return errors.New("QS runner lost Mongo SessionContext")
			}
			if _, err := db.Collection("rm_business").InsertOne(sc, bson.M{"_id": factID}); err != nil {
				return err
			}
			appender, err := sdkmongo.Bind(sc, outboxColl)
			if err != nil {
				return err
			}
			if err := appender.Append(intent, due); err != nil {
				return err
			}
			return finalErr
		})
	}
	first := makeMessage(`{"id":"m4-standard-mongo-one"}`)
	require.NoError(t, stage("committed", first, nil))
	abort := errors.New("abort original QS Mongo transaction")
	require.ErrorIs(t, stage("rolled-back", makeMessage(`{"id":"m4-standard-mongo-one"}`), abort), abort)
	require.ErrorIs(t, stage("conflict", makeMessage(`{"id":"m4-standard-mongo-one","changed":true}`), nil), outbox.ErrConflict)
	for _, name := range []string{"rm_business", "rm_outbox"} {
		n, err := db.Collection(name).CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		require.EqualValues(t, 1, n, name)
	}
	require.Equal(t, 3, limiter.acquired)
	require.Equal(t, 3, limiter.released)
	var stored struct {
		Payload []byte    `bson:"payload"`
		Due     time.Time `bson:"next_attempt_at"`
	}
	require.NoError(t, outboxColl.FindOne(ctx, bson.M{"message_id": first.Input().ID}).Decode(&stored))
	require.Equal(t, first.Input().Payload, stored.Payload)
	require.True(t, stored.Due.Equal(due), "standard Mongo due instant changed")
	store, err := sdkmongo.New(outboxColl)
	require.NoError(t, err)
	claims, err := store.ClaimDue(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Empty(t, claims, "delayed standard Mongo message claimed early")
}
