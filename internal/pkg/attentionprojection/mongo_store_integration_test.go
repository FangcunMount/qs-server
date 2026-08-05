package attentionprojection

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoStoreSyncClient struct {
	calls int
}

func (c *mongoStoreSyncClient) SyncAssessmentAttention(context.Context, uint64, string, bool) error {
	c.calls++
	return nil
}

func TestMongoStoreProjectorUpsertAndDuplicateEvent(t *testing.T) {
	db := openAttentionProjectionMongo(t)
	store, err := NewMongoStore(db)
	if err != nil {
		t.Fatalf("NewMongoStore: %v", err)
	}
	client := &mongoStoreSyncClient{}
	projector := NewProjector(store, client, DefaultMaxAttempts, nil)
	input := PendingInput{
		EventID:      "evt-mongo-upsert",
		ReportID:     "report-1",
		AssessmentID: "assessment-1",
		TesteeID:     99,
		RiskLevel:    "high",
		MarkKeyFocus: true,
	}

	if err := projector.Project(t.Context(), input); err != nil {
		t.Fatalf("first Project: %v", err)
	}
	if err := projector.Project(t.Context(), input); err != nil {
		t.Fatalf("duplicate Project: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("attention sync calls = %d, want 1", client.calls)
	}

	record, err := store.GetByEventID(t.Context(), input.EventID)
	if err != nil {
		t.Fatalf("GetByEventID: %v", err)
	}
	if record.Status != StatusSucceeded {
		t.Fatalf("status = %q, want %q", record.Status, StatusSucceeded)
	}
	if record.ReportID != input.ReportID || record.AssessmentID != input.AssessmentID || record.TesteeID != input.TesteeID {
		t.Fatalf("projection identity = %#v, want input %#v", record, input)
	}
	if record.RiskLevel != input.RiskLevel || record.MarkKeyFocus != input.MarkKeyFocus {
		t.Fatalf("projection attention = (%q, %t), want (%q, %t)", record.RiskLevel, record.MarkKeyFocus, input.RiskLevel, input.MarkKeyFocus)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		t.Fatalf("projection timestamps must be persisted: %#v", record)
	}

	count, err := db.Collection(CollectionName).CountDocuments(t.Context(), bson.M{"event_id": input.EventID})
	if err != nil {
		t.Fatalf("CountDocuments: %v", err)
	}
	if count != 1 {
		t.Fatalf("projection documents = %d, want 1", count)
	}
}

func openAttentionProjectionMongo(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("QS_SERVER_TEST_MONGO_URI is not set; attention projection Mongo integration test skipped")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect Mongo: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("ping Mongo: %v", err)
	}
	base := os.Getenv("QS_SERVER_TEST_MONGO_DB")
	if base == "" {
		base = "qs_server_contract_test"
	}
	db := client.Database(fmt.Sprintf("%s_attention_projection_%d", base, time.Now().UnixNano()))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = db.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})
	return db
}
