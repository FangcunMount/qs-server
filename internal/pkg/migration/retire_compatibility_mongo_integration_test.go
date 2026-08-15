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

func TestRuntimeLedgerRetirementPreconditionRejectsAnySourceDocument(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	driver := NewMongoDriver(client)

	if err := driver.ensureMongoCollections(t.Context(), db.Name(), runtimeLedgerRetirementCollections); err != nil {
		t.Fatalf("prepare runtime ledger retirement collections: %v", err)
	}
	if err := driver.verifyEmptyMongoCollections(t.Context(), db.Name(), runtimeLedgerRetirementCollections, "runtime ledger"); err != nil {
		t.Fatalf("empty runtime ledger precondition: %v", err)
	}
	for _, name := range runtimeLedgerRetirementCollections {
		name := name
		t.Run(name, func(t *testing.T) {
			if _, err := db.Collection(name).InsertOne(t.Context(), bson.M{"proof": true}); err != nil {
				t.Fatal(err)
			}
			err := driver.verifyEmptyMongoCollections(t.Context(), db.Name(), runtimeLedgerRetirementCollections, "runtime ledger")
			if err == nil || !strings.Contains(err.Error(), name+" contains 1 documents") {
				t.Fatalf("runtime ledger precondition error = %v", err)
			}
			if _, err := db.Collection(name).DeleteMany(t.Context(), bson.M{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeLedgerRetirementFromVersion22WithSourcesAlreadyDropped(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	config := ensureConfigDefaults(&Config{Enabled: true, Database: db.Name()})
	driver := NewMongoDriver(client)
	instance, err := driver.CreateInstance(migrations, config)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := driver.PrepareRun(t.Context(), config, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Migrate(compatibilityRetirementVersion); err != nil {
		t.Fatalf("migrate MongoDB 0 -> %d: %v", compatibilityRetirementVersion, err)
	}
	if err := cleanup(t.Context()); err != nil {
		t.Fatalf("cleanup temporary migration index: %v", err)
	}
	for _, name := range runtimeLedgerRetirementCollections {
		if err := db.Collection(name).Drop(t.Context()); err != nil {
			t.Fatalf("simulate bounded cutover drop %s: %v", name, err)
		}
	}

	version, changed, err := NewMongoMigrator(client, config).Run()
	if err != nil || !changed || version != runtimeLedgerRetirementVersion {
		t.Fatalf("migrate MongoDB %d -> %d: version=%d changed=%v err=%v", compatibilityRetirementVersion, runtimeLedgerRetirementVersion, version, changed, err)
	}
	collectionNames, err := db.ListCollectionNames(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range runtimeLedgerRetirementCollections {
		for _, name := range collectionNames {
			if name == retired {
				t.Fatalf("retired runtime ledger %s was recreated", retired)
			}
		}
	}
}
