//go:build integration

package migration

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestSchemaPairNativeM5RetainedFullBaseline(t *testing.T) {
	n := openNativeSchemaPairWithPrefix(t, "qs_m5_baseline_test")
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	n.approve(t, r)
	n.runBoth(t, n.preflight(t, r))
	_, err = n.db.ExecContext(t.Context(), "CREATE TABLE m5_retained_fixture(id BIGINT PRIMARY KEY, original_bytes BLOB NULL, original_time DATETIME(6), exact_number DECIMAL(20,6)); INSERT INTO m5_retained_fixture VALUES(1,NULL,'2026-10-31 01:02:03.123456',123.456789),(2,X'0001FF','2026-10-31 01:02:03.123457',0.000001)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.mongoDB.Collection("m5_retained_fixture").InsertOne(t.Context(), bson.D{{Key: "_id", Value: "original"}, {Key: "typed", Value: primitive.Binary{Subtype: 0x80, Data: []byte{0, 1, 255}}}, {Key: "date", Value: time.Date(2026, 10, 31, 1, 2, 3, 0, time.UTC)}, {Key: "nested", Value: bson.D{{Key: "a", Value: int64(1)}, {Key: "b", Value: nil}}}})
	if err != nil {
		t.Fatal(err)
	}
	read := func() RetainedBaseline {
		t.Helper()
		out, err := CaptureRetainedBaseline(t.Context(), n.db, n.client, n.cfg, 1000000)
		if err != nil || !out.Complete || !out.ReadOnly || out.Mutation {
			t.Fatal("full owned baseline unavailable", out.Phase, err)
		}
		return out
	}
	before := read()
	repeat := read()
	if !reflect.DeepEqual(before.Objects, repeat.Objects) {
		t.Fatal("unchanged complete objects produced different baseline")
	}
	sqlObject := before.Objects["qs.mysql.m5_retained_fixture"]
	mongoObject := before.Objects["qs.mongo.m5_retained_fixture"]
	if sqlObject.Rows == nil || *sqlObject.Rows != 2 || mongoObject.Rows == nil || *mongoObject.Rows != 1 {
		t.Fatal("complete typed data not counted")
	}
	_, err = n.db.ExecContext(t.Context(), "UPDATE m5_retained_fixture SET original_bytes=X'' WHERE id=1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.mongoDB.Collection("m5_retained_fixture").UpdateOne(t.Context(), bson.M{"_id": "original"}, bson.M{"$set": bson.M{"nested.a": int64(2)}})
	if err != nil {
		t.Fatal(err)
	}
	changed := read()
	if changed.Objects["qs.mysql.m5_retained_fixture"].ContentSHA256 == sqlObject.ContentSHA256 || changed.Objects["qs.mongo.m5_retained_fixture"].ContentSHA256 == mongoObject.ContentSHA256 {
		t.Fatal("NULL/empty or BSON change hidden")
	}
	_, err = n.db.ExecContext(t.Context(), "UPDATE m5_retained_fixture SET original_bytes=NULL WHERE id=1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.mongoDB.Collection("m5_retained_fixture").UpdateOne(t.Context(), bson.M{"_id": "original"}, bson.M{"$set": bson.M{"nested.a": int64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Objects, read().Objects) {
		t.Fatal("baseline reader modified original data or metadata")
	}
	partial, err := CaptureRetainedBaseline(t.Context(), n.db, n.client, n.cfg, 1)
	if err == nil || partial.Complete {
		t.Fatal("bounded prefix was accepted as full content")
	}
	_, err = n.mongoDB.Collection("m5_retained_ttl_fixture").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "expires", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(3600)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.mongoDB.Collection("m5_retained_ttl_fixture").InsertOne(t.Context(), bson.M{"_id": "retained", "expires": time.Now().UTC().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	unsafe, err := CaptureRetainedBaseline(t.Context(), n.db, n.client, n.cfg, 1000000)
	if err == nil || unsafe.Complete {
		t.Fatal("nonempty autonomous expiry was silently qualified")
	}
}
