//go:build integration

// Package mongodbtest provides the fail-closed Mongo Replica Set harness used
// by integration contracts. It never falls back to an application database.
package mongodbtest

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const testURIEnv = "QS_SERVER_TEST_MONGO_URI"

var databaseToken = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// ReplicaSetDatabase connects to the explicitly configured test URI, proves
// the server is a Replica Set, and allocates a unique disposable database.
func ReplicaSetDatabase(t testing.TB) (*mongo.Client, *mongo.Database) {
	t.Helper()
	uri := strings.TrimSpace(os.Getenv(testURIEnv))
	if uri == "" {
		t.Fatalf("%s is required for integration tests; SKIP is not allowed", testURIEnv)
	}
	prefix := strings.TrimSpace(os.Getenv("QS_SERVER_TEST_MONGO_DB_PREFIX"))
	if prefix == "" {
		prefix = "qs_modelcatalog_contract"
	}
	lowerPrefix := strings.ToLower(prefix)
	if !strings.Contains(lowerPrefix, "test") && !strings.Contains(lowerPrefix, "contract") {
		t.Fatalf("QS_SERVER_TEST_MONGO_DB_PREFIX %q must contain test or contract", prefix)
	}
	prefix = databaseToken.ReplaceAllString(prefix, "_")
	if len(prefix) > 36 {
		prefix = prefix[:36]
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect test Mongo: %v", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("ping primary test Mongo: %v", err)
	}
	var hello struct {
		SetName string `bson:"setName"`
	}
	if err := client.Database("admin").RunCommand(ctx, map[string]any{"hello": 1}).Decode(&hello); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("Mongo hello: %v", err)
	}
	if strings.TrimSpace(hello.SetName) == "" {
		_ = client.Disconnect(context.Background())
		t.Fatal("integration Mongo is not a Replica Set (hello.setName is empty)")
	}

	databaseName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	db := client.Database(databaseName)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := db.Drop(cleanupCtx); err != nil {
			t.Errorf("drop integration database %s: %v", databaseName, err)
		}
		if err := client.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect integration Mongo: %v", err)
		}
	})
	return client, db
}
