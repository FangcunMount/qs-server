//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package standardoutbox

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestStandardMongoScheduledRetryUsesOriginalTransactionAndDueTime(t *testing.T) {
	uri := os.Getenv("RM_M5_SCHEDULED_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://127.0.0.1:") ||
		!strings.Contains(uri, "replicaSet=rm-test") ||
		!strings.Contains(uri, "directConnection=true") {
		t.Skip("disposable local rm-test replica-set URI required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database("rm_m5_scheduled_test")
	defer db.Drop(context.Background())
	if err := db.CreateCollection(ctx, "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	collection := db.Collection("rm_outbox")
	wire, err := eventcatalog.Load("../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := NewStager(collection, eventcatalog.NewCatalog(wire), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	dueAt := now.Add(15 * time.Minute)
	newRetry := func(generation string) domaininterpretation.InterpretationRetryRequestedEvent {
		return domaininterpretation.NewInterpretationRetryRequestedEvent(domaininterpretation.RetryRequestedEventInput{
			OrgID: 501, GenerationID: generation, RunID: "701", AssessmentID: "801", OutcomeID: "901",
			TesteeID: 1001, ExpectedAttempt: 2, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: now,
		})
	}
	retry := newRetry("601")
	if err := stager.StageAt(ctx, dueAt, retry); !errors.Is(err, sdkmongo.ErrTransactionRequired) {
		t.Fatalf("scheduled write outside transaction error=%v", err)
	}
	if err := stager.StageAt(ctx, time.Time{}, retry); err == nil {
		t.Fatal("scheduled write accepted a zero due time")
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		return nil, stager.StageAt(txCtx, dueAt, retry)
	}); err != nil {
		t.Fatal(err)
	}
	var row struct {
		State         string    `bson:"state"`
		EventType     string    `bson:"event_type"`
		NextAttemptAt time.Time `bson:"next_attempt_at"`
	}
	if err := collection.FindOne(ctx, bson.M{"message_id": retry.EventID()}).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.State != "pending" || row.EventType != eventcatalog.InterpretationRetryRequested || !row.NextAttemptAt.Equal(dueAt.UTC()) {
		t.Fatalf("scheduled row state=%s event=%s due=%s; want pending/%s/%s", row.State, row.EventType, row.NextAttemptAt, eventcatalog.InterpretationRetryRequested, dueAt.UTC())
	}
	store, err := sdkmongo.New(collection)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDue(ctx, 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("future retry was claimed before due time: %d", len(claimed))
	}
	abort := errors.New("abort original transaction")
	abortedRetry := newRetry("602")
	_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		if err := stager.StageAt(txCtx, dueAt, abortedRetry); err != nil {
			return nil, err
		}
		return nil, abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("aborted transaction error=%v", err)
	}
	count, err := collection.CountDocuments(ctx, bson.M{"message_id": abortedRetry.EventID()})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("aborted transaction retained %d retry intents", count)
	}
}
