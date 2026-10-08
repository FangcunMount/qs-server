//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	mongomigrate "github.com/golang-migrate/migrate/v4/database/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoStableAnchorSurvivesActualMigrationDriverGenerationNative(t *testing.T) {
	uri := os.Getenv("QS_RETIREMENT_ANCHOR_MONGO_URI")
	if uri == "" {
		if os.Getenv("QS_RETIREMENT_ANCHOR_NATIVE_REQUIRED") == "1" {
			t.Fatal("native fixture missing")
		}
		t.Skip("owned Mongo replica fixture not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal("fixture connect failed")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(closeCtx)
	}()
	db := client.Database("qs_retirement_anchor_" + primitive.NewObjectID().Hex())
	defer func() {
		if err := db.Drop(context.Background()); err != nil {
			t.Error("owned database cleanup failed")
		}
	}()
	var hello bson.Raw
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatal("hello read failed")
	}
	original, err := mongoDatabaseAnchor(ctx, db, hello)
	if err != nil {
		t.Fatal("stable anchor read failed")
	}
	if _, err := db.Collection("kept_business_fact").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "sentinel", Value: "retained"}}); err != nil {
		t.Fatal(err)
	}
	driver, err := mongomigrate.WithInstance(client, &mongomigrate.Config{DatabaseName: db.Name(), MigrationsCollection: "schema_migrations"})
	if err != nil {
		t.Fatal("actual driver initialize failed")
	}
	if err := driver.SetVersion(37, false); err != nil {
		t.Fatal("initial schema stamp failed")
	}
	catalog, defs, err := mongoSchemas(ctx, db)
	if err != nil {
		t.Fatal("initial catalog read failed")
	}
	firstGeneration, err := mongoMigrationGeneration(catalog["schema_migrations"])
	if err != nil {
		t.Fatal("initial generation read failed")
	}
	if err := driver.SetVersion(38, false); err != nil {
		t.Fatal("actual schema advancement failed")
	}
	secondCatalog, secondDefs, err := mongoSchemas(ctx, db)
	if err != nil {
		t.Fatal("new catalog read failed")
	}
	secondGeneration, err := mongoMigrationGeneration(secondCatalog["schema_migrations"])
	if err != nil || firstGeneration == secondGeneration {
		t.Fatal("actual migration Drop/Insert did not change recorded generation")
	}
	if digest(defs) == digest(secondDefs) {
		t.Fatal("full catalog must observe migration collection replacement")
	}
	current, err := mongoDatabaseAnchor(ctx, db, hello)
	if err != nil || current != original {
		t.Fatal("normal schema advancement changed stable anchor")
	}
	var kept bson.M
	if err := db.Collection("kept_business_fact").FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&kept); err != nil || kept["sentinel"] != "retained" {
		t.Fatal("non-target business fact changed")
	}
	if err := db.Collection("kept_business_fact").Drop(ctx); err != nil {
		t.Fatal("owned replacement setup failed")
	}
	if _, err := db.Collection("kept_business_fact").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "sentinel", Value: "retained"}}); err != nil {
		t.Fatal("owned replacement setup failed")
	}
	_, replaced, err := mongoSchemas(ctx, db)
	if err != nil || digest(secondDefs) == digest(replaced) {
		t.Fatal("stable anchor must not replace full catalog integrity checks")
	}
}
