//go:build integration

package migration

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestCompatibilityRetirementPreconditionRejectsLiveDataAndArchiveReferences(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	driver := NewMongoDriver(client)

	if err := driver.ensureCompatibilityRetirementCollections(t.Context(), db.Name()); err != nil {
		t.Fatalf("prepare empty retirement collections: %v", err)
	}
	for _, name := range compatibilityRetirementCollections {
		if err := db.Collection(name).FindOne(t.Context(), bson.M{}).Err(); err != mongo.ErrNoDocuments {
			t.Fatalf("prepared collection %s lookup error = %v, want ErrNoDocuments", name, err)
		}
	}
	if err := driver.verifyCompatibilityRetirement(t.Context(), db.Name()); err != nil {
		t.Fatalf("empty retirement precondition: %v", err)
	}
	if _, err := db.Collection("answersheet_submit_idempotency").InsertOne(t.Context(), bson.M{"proof": true}); err != nil {
		t.Fatal(err)
	}
	if err := driver.verifyCompatibilityRetirement(t.Context(), db.Name()); err == nil || !strings.Contains(err.Error(), "answersheet_submit_idempotency contains 1 documents") {
		t.Fatalf("answersheet precondition error = %v", err)
	}
	if _, err := db.Collection("answersheet_submit_idempotency").DeleteMany(t.Context(), bson.M{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("archived_reports").InsertOne(t.Context(), bson.M{"proof": true}); err != nil {
		t.Fatal(err)
	}
	if err := driver.verifyCompatibilityRetirement(t.Context(), db.Name()); err == nil || !strings.Contains(err.Error(), "archived_reports contains 1 documents") {
		t.Fatalf("archive precondition error = %v", err)
	}
	if _, err := db.Collection("archived_reports").DeleteMany(t.Context(), bson.M{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("report_query_catalog").InsertOne(t.Context(), bson.M{"source_kind": "archive"}); err != nil {
		t.Fatal(err)
	}
	if err := driver.verifyCompatibilityRetirement(t.Context(), db.Name()); err == nil || !strings.Contains(err.Error(), "report_query_catalog contains 1 archive references") {
		t.Fatalf("catalog precondition error = %v", err)
	}
}
