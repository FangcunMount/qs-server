//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package standardoutbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestStandardMongoStatusFailsClosedOnIncompleteRows(t *testing.T) {
	uri := os.Getenv("RM_QS_STATUS_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test replica-set Mongo URI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := driver.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database("m4_qs_status")
	defer db.Drop(context.Background())
	collection := db.Collection("rm_outbox")
	created := time.Date(2026, 9, 23, 2, 0, 0, 0, time.UTC)
	for _, state := range []string{"pending", "retry_wait", "publishing", "quarantined", "published"} {
		if _, err := collection.InsertOne(ctx, bson.M{"_id": state, "state": state, "created_at": created}); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := NewStatusReader(collection)
	if err != nil {
		t.Fatal(err)
	}
	observed := created.Add(2 * time.Hour).In(time.FixedZone("UTC+8", 8*3600))
	snapshot, err := reader.OutboxStatusSnapshot(ctx, observed)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Buckets) != 4 {
		t.Fatalf("unexpected buckets: %+v", snapshot)
	}
	for _, bucket := range snapshot.Buckets {
		if bucket.Count != 1 || bucket.OldestCreatedAt == nil || !bucket.OldestCreatedAt.Equal(created) || bucket.OldestAgeSeconds != 7200 {
			t.Fatalf("wrong state count or UTC age: %+v", bucket)
		}
	}
	if _, err := collection.InsertOne(ctx, bson.M{"_id": "missing", "state": "pending"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.OutboxStatusSnapshot(ctx, observed); err == nil {
		t.Fatal("missing creation time was hidden")
	}
	if _, err := collection.DeleteOne(ctx, bson.M{"_id": "missing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := collection.InsertOne(ctx, bson.M{"_id": "unknown", "state": "unexpected", "created_at": created}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.OutboxStatusSnapshot(ctx, observed); err == nil {
		t.Fatal("unknown unfinished state was hidden")
	}
}
