//go:build integration

package migration

import (
	"testing"

	mongo_indexes "github.com/FangcunMount/qs-server/internal/pkg/mongodb"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestUnifiedModelCatalogSchemaMigrationFreshInstall(t *testing.T) {
	ctx := t.Context()
	_, db := mongodbtest.ReplicaSetDatabase(t)

	for _, collection := range []string{"assessment_models", "questionnaires"} {
		if err := db.CreateCollection(ctx, collection); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate pre-000013 legacy unique indexes from 000001 / 000010.
	if _, err := db.Collection("assessment_models").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetName("idx_assessment_models_code").SetUnique(true).SetPartialFilterExpression(bson.M{"deleted_at": nil}),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("questionnaires").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}, {Key: "version", Value: 1}},
		Options: options.Index().SetName("idx_code_version").SetUnique(true),
	}); err != nil {
		t.Fatal(err)
	}

	execMongoMigration(t, db, "000013_unified_modelcatalog_schema.up.json")

	mgr := mongo_indexes.NewIndexManager(db)
	// JSON up only createIndexes; legacy drop is Go-side (idempotent IndexNotFound).
	if err := mgr.ReconcileUnifiedModelCatalogIndexes(ctx); err != nil {
		t.Fatalf("ReconcileUnifiedModelCatalogIndexes: %v", err)
	}
	if err := mgr.ReconcileUnifiedModelCatalogIndexes(ctx); err != nil {
		t.Fatalf("idempotent ReconcileUnifiedModelCatalogIndexes: %v", err)
	}

	assertMongoIndex(t, db.Collection("assessment_models"), "idx_assessment_models_code", false)
	assertMongoIndex(t, db.Collection("questionnaires"), "idx_code_version", false)
	for collection, names := range mongo_indexes.RequiredUnifiedIndexNames() {
		for _, name := range names {
			assertMongoIndex(t, db.Collection(collection), name, true)
		}
	}

	if err := mgr.VerifyUnifiedModelCatalogIndexes(ctx); err != nil {
		t.Fatalf("VerifyUnifiedModelCatalogIndexes: %v", err)
	}

	// Head + snapshot with the same code must coexist after dropping legacy unique(code).
	if _, err := db.Collection("assessment_models").InsertMany(ctx, []any{
		bson.M{"code": "M1", "record_role": "head", "deleted_at": nil},
		bson.M{"code": "M1", "record_role": "published_snapshot", "release_status": "active", "release_version": "1.0.0", "kind": "scale", "deleted_at": nil},
	}); err != nil {
		t.Fatalf("insert head+snapshot same code: %v", err)
	}
	// Duplicate active snapshot for same code must be rejected.
	_, err := db.Collection("assessment_models").InsertOne(ctx, bson.M{
		"code": "M1", "record_role": "published_snapshot", "release_status": "active", "release_version": "1.0.1", "kind": "scale", "deleted_at": nil,
	})
	if err == nil {
		t.Fatal("expected duplicate active code to violate idx_assessment_models_active_code")
	}

	if _, err := db.Collection("assessment_norms").InsertOne(ctx, bson.M{"table_version": "norm-v1", "deleted_at": nil}); err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("assessment_norms").InsertOne(ctx, bson.M{"table_version": "norm-v1", "deleted_at": nil})
	if err == nil {
		t.Fatal("expected duplicate table_version to violate idx_assessment_norms_table_version")
	}

	execMongoMigration(t, db, "000013_unified_modelcatalog_schema.down.json")
	assertMongoIndex(t, db.Collection("assessment_models"), "idx_assessment_models_head_code", false)
	assertMongoIndex(t, db.Collection("assessment_models"), "idx_assessment_models_code", true)
	assertMongoIndex(t, db.Collection("questionnaires"), "idx_code_version", true)
}

func TestUnifiedModelCatalogSchemaDirty13FailsClosed(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	if _, err := db.Collection("schema_migrations").InsertOne(t.Context(), bson.M{"version": int64(13), "dirty": true}); err != nil {
		t.Fatal(err)
	}
	migrator := NewMongoMigrator(client, &Config{Enabled: true, Database: db.Name()})
	version, changed, err := migrator.Run()
	if err == nil || version != 13 || changed {
		t.Fatalf("Run() = version %d, changed %t, err %v; want dirty@13 diagnostic", version, changed, err)
	}
}
