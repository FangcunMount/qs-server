//go:build integration

package interpretation

import (
	"encoding/json"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"os"
	"testing"
)

func TestOriginalEventEvidenceMigrationIndexesAreUniqueOnlyForNonemptyStrings(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	raw, err := os.ReadFile("../../../../pkg/migration/migrations/mongodb/000037_original_event_evidence.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []json.RawMessage
	if err := json.Unmarshal(raw, &commands); err != nil {
		t.Fatal(err)
	}
	for _, rawCommand := range commands {
		var command bson.D
		if err := bson.UnmarshalExtJSON(rawCommand, false, &command); err != nil {
			t.Fatal(err)
		}
		if err := db.RunCommand(ctx, command).Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, target := range []struct{ collection, field string }{{"report_generations", "generated_event_id"}, {"interpretation_runs", "retry_event_id"}} {
		t.Run(target.collection, func(t *testing.T) {
			coll := db.Collection(target.collection)
			if _, err := coll.InsertMany(ctx, []any{bson.M{}, bson.M{}, bson.M{target.field: ""}, bson.M{target.field: ""}}); err != nil {
				t.Fatalf("legacy missing/empty refs were uniquely indexed: %v", err)
			}
			if _, err := coll.InsertOne(ctx, bson.M{target.field: "same-original-event"}); err != nil {
				t.Fatal(err)
			}
			if _, err := coll.InsertOne(ctx, bson.M{target.field: "same-original-event"}); !mongo.IsDuplicateKeyError(err) {
				t.Fatalf("duplicate original ref was accepted: %v", err)
			}
		})
	}
}
