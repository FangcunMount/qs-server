//go:build integration

package migration

import (
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"testing"
)

func TestMongoHistoricalReferenceMigration38InstallsOnlyAdditiveIndexes(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	for _, name := range []string{"answersheets", "report_generations", "domain_event_outbox"} {
		if _, err := db.Collection(name).InsertOne(ctx, bson.M{"sentinel": name}); err != nil {
			t.Fatal(err)
		}
	}
	up, err := migrations.ReadFile("migrations/mongodb/000038_add_historical_event_references.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []bson.D
	if err := bson.UnmarshalExtJSON(append([]byte(`{"commands":`), append(up, []byte(`}`)...)...), false, &struct {
		Commands *[]bson.D `bson:"commands"`
	}{&commands}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatal("unexpected additive migration command count")
	}
	for range 2 {
		for _, command := range commands {
			if len(command) == 0 || command[0].Key != "createIndexes" {
				t.Fatal("non-additive command")
			}
			if err := db.RunCommand(ctx, command).Err(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, target := range []struct{ name, key string }{{"answersheets", "legacy_submission_evidence.entries.event_id"}, {"report_generations", "historical_generated_evidence.entries.event_id"}} {
		cursor, err := db.Collection(target.name).Indexes().List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var rows []bson.M
		if err := cursor.All(ctx, &rows); err != nil {
			t.Fatal(err)
		}
		unique := false
		for _, row := range rows {
			if row["unique"] == true {
				keys, ok := row["key"].(bson.M)
				if ok && keys[target.key] != nil {
					unique = true
				}
			}
		}
		if !unique {
			t.Fatal("missing unique history index", target.name)
		}
		slot := "legacy_submission_evidence"
		if target.name == "report_generations" {
			slot = "historical_generated_evidence"
		}
		value := bson.M{slot: bson.M{"entries": bson.A{bson.M{"event_id": "index-contract-source-id"}}}}
		if _, err := db.Collection(target.name).InsertOne(ctx, value); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Collection(target.name).InsertOne(ctx, value); !mongo.IsDuplicateKeyError(err) {
			t.Fatalf("unique multikey collision was not enforced: %s %v", target.name, err)
		}
	}
	for _, name := range []string{"answersheets", "report_generations", "domain_event_outbox"} {
		if n, err := db.Collection(name).CountDocuments(ctx, bson.M{"sentinel": name}); err != nil || n != 1 {
			t.Fatalf("migration changed business data: %s %d %v", name, n, err)
		}
	}
}
