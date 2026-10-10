//go:build integration

package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// This regression borrows one genuine fixture client and its real snapshot.
// It neither constructs an Archive/Window/Pair nor grants writer/DROP power.
func TestMongoMetadataSnapshotNative(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	id, migrationID := primitive.NewObjectID(), primitive.NewObjectID()
	if _, e := db.Collection(targetNames[3]).InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "fact", Value: "original"}}); e != nil {
		t.Fatal("owned original fixture insert failed")
	}
	if _, e := db.Collection("schema_migrations").InsertOne(ctx, bson.D{{Key: "_id", Value: migrationID}, {Key: "version", Value: int64(38)}, {Key: "dirty", Value: false}}); e != nil {
		t.Fatal("owned schema-head insert failed")
	}
	collections, defs, e := mongoCatalog(ctx, db)
	if e != nil {
		t.Fatal("owned original metadata read failed")
	}
	var hello, config bson.Raw
	if db.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || db.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&config) != nil {
		t.Fatal("owned actual topology read failed")
	}
	subtype, uuid, valid := collections["schema_migrations"].Lookup("info", "uuid").BinaryOK()
	set, setOK := hello.Lookup("setName").StringValueOK()
	replicaID, replicaOK := config.Lookup("config", "settings", "replicaSetId").ObjectIDOK()
	if !valid || subtype != 4 || len(uuid) != 16 || !setOK || !replicaOK || replicaID.IsZero() {
		t.Fatal("owned actual database identity unavailable")
	}
	stable := bson.D{}
	for _, key := range []string{"setName", "hosts", "me"} {
		value := hello.Lookup(key)
		if value.Type != 0 {
			var decoded any
			if value.Unmarshal(&decoded) != nil {
				t.Fatal("owned actual identity value invalid")
			}
			stable = append(stable, bson.E{Key: key, Value: decoded})
		}
	}
	stableJSON, e := json.Marshal(stable)
	if e != nil {
		t.Fatal("owned actual identity encoding failed")
	}
	binding := Binding{IdentityHash: parts("mongodb_database_identity_v1", string(stableJSON), db.Name(), hex.EncodeToString(uuid)), GenerationHash: parts("mongodb_migration_generation_v1", hex.EncodeToString(uuid)), AnchorHash: parts("mongodb_database_anchor_v1", replicaID.Hex(), set, db.Name()), Version: 38}
	ordered, e := ReadOrderedMongoSchema(ctx, db)
	if e != nil {
		t.Fatal("owned actual ordered schema read failed")
	}
	session, e := client.StartSession()
	if e != nil {
		t.Fatal("owned snapshot session start failed")
	}
	defer session.EndSession(context.Background())
	if session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())) != nil {
		t.Fatal("owned snapshot start failed")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if session.AbortTransaction(cleanup) != nil {
			t.Error("owned snapshot cleanup failed")
		}
	}()
	type ownerKey struct{}
	parent := context.WithValue(ctx, ownerKey{}, "original-snapshot-owner")
	body := mongo.NewSessionContext(parent, session)
	first, e := nonTargetMongoData(body, db, targetNames[3], 38)
	if e != nil || first.Rows != 1 {
		t.Fatal("actual snapshot body read failed")
	}
	x, ok := session.(mongo.XSession) //nolint:staticcheck // pinned driver actual transaction state
	if !ok || x.ClientSession() == nil || !x.ClientSession().TransactionInProgress() || x.ClientSession().CurrentRc == nil || x.ClientSession().CurrentRc.Level != "snapshot" {
		t.Fatal("server body read did not start the real snapshot")
	}
	number := x.ClientSession().TxnNumber
	metadata := mongoMetadataContext(body)
	deadline, hasDeadline := body.Deadline()
	metadataDeadline, hasMetadataDeadline := metadata.Deadline()
	if mongo.SessionFromContext(metadata) != nil || mongo.SessionFromContext(body) != session || metadata.Done() != body.Done() || !hasDeadline || !hasMetadataDeadline || !deadline.Equal(metadataDeadline) || metadata.Value(ownerKey{}) != parent.Value(ownerKey{}) {
		t.Fatal("metadata context changed original snapshot lifetime/binding")
	}
	fresh, freshDefs, e := mongoCatalog(body, db)
	if e != nil || jsonSHA(freshDefs) != jsonSHA(defs) {
		t.Fatal("actual catalog in live snapshot caller failed")
	}
	actualOrdered, e := ReadOrderedMongoSchema(body, db)
	if e != nil || actualOrdered.SHA256() != ordered.SHA256() {
		t.Fatal("actual ordered target indexes in snapshot caller failed")
	}
	if _, e = nonTargetOrderedMongo(body, db, "schema_migrations", fresh["schema_migrations"]); e != nil {
		t.Fatal("actual ordered non-target index metadata failed")
	}
	// Ordinary writes use this exact same borrowed client outside the body
	// session. They establish a real newer fact which snapshot reads must hide.
	if _, e = db.Collection(targetNames[3]).UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: bson.D{{Key: "fact", Value: "newer"}}}}); e != nil {
		t.Fatal("owned external body update failed")
	}
	if _, e = db.Collection("schema_migrations").UpdateOne(ctx, bson.D{{Key: "_id", Value: migrationID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "version", Value: int64(39)}}}}); e != nil {
		t.Fatal("owned external schema-head update failed")
	}
	actual, e := nonTargetMongoData(ctx, db, targetNames[3], 38)
	if e != nil || actual.DataSHA256 == first.DataSHA256 {
		t.Fatal("owned actual external update was not observed")
	}
	after, e := nonTargetMongoData(body, db, targetNames[3], 38)
	if e != nil || !reflect.DeepEqual(first, after) {
		t.Fatal("metadata read removed the original body snapshot")
	}
	if _, e = mongoState(body, db, binding, fresh); e != nil {
		t.Fatal("metadata/head split failed to keep exact original snapshot head")
	}
	if _, e = nonTargetMongoData(body, db, "schema_migrations", 38); e != nil {
		t.Fatal("actual migration head escaped original snapshot")
	}
	if _, e = nonTargetMongoData(ctx, db, "schema_migrations", 39); e != nil {
		t.Fatal("owned actual newer migration head was not observed")
	}
	if !x.ClientSession().TransactionInProgress() || x.ClientSession().TxnNumber != number || mongo.SessionFromContext(body) != session {
		t.Fatal("metadata helpers ended or replaced original snapshot")
	}
	cancel()
	if !errors.Is(metadata.Err(), context.Canceled) || !errors.Is(body.Err(), context.Canceled) {
		t.Fatal("metadata context survived owner cancellation")
	}
	if _, _, e = mongoCatalog(body, db); e == nil {
		t.Fatal("canceled metadata caller was accepted")
	}
}
