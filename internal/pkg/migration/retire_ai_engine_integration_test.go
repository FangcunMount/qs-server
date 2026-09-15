//go:build integration

package migration

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestAIEngineRetirementRejectsEveryNonemptyCollectionWithoutDeleting(t *testing.T) {
	for _, name := range aiEngineRetirementCollections {
		t.Run(name, func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			if _, err := db.Collection(name).InsertOne(t.Context(), bson.M{"proof": true}); err != nil {
				t.Fatal(err)
			}
			_, err := NewMongoDriver(client).PrepareRun(t.Context(), ensureConfigDefaults(&Config{Database: db.Name()}), 34)
			if err == nil || !strings.Contains(err.Error(), name+" contains 1 documents") {
				t.Fatalf("expected nonempty refusal: %v", err)
			}
			count, err := db.Collection(name).CountDocuments(t.Context(), bson.M{})
			if err != nil || count != 1 {
				t.Fatalf("source changed: %d %v", count, err)
			}
		})
	}
}

func TestAIEngineRetirementFreshAndCleanedUpgradeAndRestart(t *testing.T) {
	for _, stage := range []string{"fresh", "cleaned", "partly_absent"} {
		t.Run(stage, func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			config := ensureConfigDefaults(&Config{Enabled: true, Database: db.Name()})
			driver := NewMongoDriver(client)
			if stage != "fresh" {
				instance, err := driver.CreateInstance(migrations, config)
				if err != nil {
					t.Fatal(err)
				}
				cleanup, err := driver.PrepareRun(t.Context(), config, 0)
				if err != nil {
					t.Fatal(err)
				}
				if err := instance.Migrate(34); err != nil {
					t.Fatal(err)
				}
				if err := cleanup(t.Context()); err != nil {
					t.Fatal(err)
				}
				for i, name := range aiEngineRetirementCollections {
					if stage == "partly_absent" && i%2 == 0 {
						continue
					}
					if err := db.Collection(name).Drop(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
			}
			// A shared business collection must survive both upgrade and restart.
			if _, err := db.Collection("m5_test_protected").InsertOne(t.Context(), bson.M{"keep": true}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				version, changed, err := NewMongoMigrator(client, config).Run()
				if err != nil || version != aiEngineRetirementVersion || changed != (i == 0) {
					t.Fatalf("upgrade/restart: %d %v %v", version, changed, err)
				}
				names, err := db.ListCollectionNames(t.Context(), bson.M{})
				if err != nil {
					t.Fatal(err)
				}
				for _, name := range names {
					for _, retired := range aiEngineRetirementCollections {
						if name == retired {
							t.Fatalf("retired collection recreated: %s", name)
						}
					}
				}
				count, err := db.Collection("m5_test_protected").CountDocuments(t.Context(), bson.M{})
				if err != nil || count != 1 {
					t.Fatalf("protected data changed: %d %v", count, err)
				}
			}
		})
	}
}
