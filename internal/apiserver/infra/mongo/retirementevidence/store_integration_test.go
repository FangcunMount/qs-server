//go:build integration

package retirementevidence

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type nilRetirementSession struct{ mongo.SessionContext }

func historyProof(id, binding string) *evidence.EventEvidenceV1 {
	return &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: id, Digest: evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("original selected source")), BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "original-source-and-terminal-business", Version: "v1", OperationID: "test-retirement", VerifiedAt: time.Date(2026, 10, 8, 1, 2, 3, 456789123, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
}

func TestBorrowedRetirementCASRequiresRealActiveOwnedSession(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	coll := db.Collection("facts")
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: "1"}, {Key: "value", Value: "business"}}); err != nil {
		t.Fatal(err)
	}
	baseline, err := Read(ctx, coll, bson.M{"_id": "1"}, "evidence")
	if err != nil {
		t.Fatal(err)
	}
	proof := historyProof("event:1", evidence.BindingDigest("test", evidence.String("business")))
	if err := baseline.Apply(ctx, coll, proof); !errors.Is(err, sdkmongo.ErrTransactionRequired) {
		t.Fatalf("no session: %v", err)
	}
	var typedNil *nilRetirementSession
	if err := baseline.Apply(typedNil, coll, proof); !errors.Is(err, sdkmongo.ErrTransactionRequired) {
		t.Fatalf("typed nil session: %v", err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if err := baseline.Apply(mongo.NewSessionContext(ctx, session), coll, proof); !errors.Is(err, sdkmongo.ErrTransactionRequired) {
		t.Fatalf("inactive session: %v", err)
	}
	other, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("QS_SERVER_TEST_MONGO_URI")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := other.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	}()
	if err := session.StartTransaction(); err != nil {
		t.Fatal(err)
	}
	tx := mongo.NewSessionContext(ctx, session)
	if err := baseline.Apply(tx, other.Database(db.Name()).Collection("facts"), proof); !errors.Is(err, sdkmongo.ErrTransactionRequired) {
		t.Fatalf("foreign client session: %v", err)
	}
	if err := baseline.Apply(tx, client.Database(db.Name()+"_wrong").Collection("facts"), proof); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong database: %v", err)
	}
	if err := session.AbortTransaction(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRetirementCASRollbackIdentityAndExactBaseline(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	coll := db.Collection("facts")
	clock := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: "rollback"}, {Key: "clock", Value: clock}, {Key: "marker", Value: bson.D{{Key: "event_id", Value: "event:1"}, {Key: "request_id", Value: "original-request"}}}}); err != nil {
		t.Fatal(err)
	}
	baseline, err := Read(ctx, coll, bson.M{"_id": "rollback"}, "marker.evidence")
	if err != nil {
		t.Fatal(err)
	}
	proof := historyProof("event:1", evidence.BindingDigest("test", evidence.String("business")))
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	abort := errors.New("host chooses rollback")
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		if err := baseline.Apply(tx, coll, proof); err != nil {
			return nil, err
		}
		var inside bson.Raw
		if err := coll.FindOne(tx, bson.M{"_id": "rollback"}).Decode(&inside); err != nil {
			return nil, err
		}
		if inside.Lookup("marker", "evidence").Type == 0 {
			t.Fatal("caller cannot read its write")
		}
		return nil, abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var outside bson.Raw
	if err := coll.FindOne(ctx, bson.M{"_id": "rollback"}).Decode(&outside); err != nil {
		t.Fatal(err)
	}
	if outside.Lookup("marker", "evidence").Type != 0 {
		t.Fatal("helper committed borrowed transaction")
	}
	apply := func(p *evidence.EventEvidenceV1) error {
		_, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) { return nil, baseline.Apply(tx, coll, p) })
		return err
	}
	if err := apply(proof); err != nil {
		t.Fatal(err)
	}
	if err := apply(proof.Clone()); err != nil {
		t.Fatalf("identical proof not idempotent at BSON milliseconds: %v", err)
	}
	if err := coll.FindOne(ctx, bson.M{"_id": "rollback"}).Decode(&outside); err != nil {
		t.Fatal(err)
	}
	if outside.Lookup("marker", "evidence", "_retirement_cas_fence").Type != 0 {
		t.Fatal("idempotence persisted a transaction-local fence")
	}
	different := proof.Clone()
	different.Verification.OperationID = "different-operation"
	if err := apply(different); !errors.Is(err, ErrConflict) {
		t.Fatalf("different conclusion overwrote proof: %v", err)
	}
	if _, err := coll.UpdateOne(ctx, bson.M{"_id": "rollback"}, bson.M{"$set": bson.M{"added_business_field": "changed"}}); err != nil {
		t.Fatal(err)
	}
	if err := apply(proof); !errors.Is(err, ErrConflict) {
		t.Fatalf("identical proof ignored added business field: %v", err)
	}
	for _, change := range []bson.M{{"$set": bson.M{"optional": nil}}, {"$set": bson.M{"clock": clock.Add(time.Millisecond)}}, {"$set": bson.M{"marker.request_id": "new-request"}}, {"$set": bson.M{"marker.event_id": "new-event"}}} {
		id := primitiveCaseID(change)
		if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "clock", Value: clock}, {Key: "marker", Value: bson.D{{Key: "event_id", Value: "event:1"}, {Key: "request_id", Value: "original-request"}}}}); err != nil {
			t.Fatal(err)
		}
		before, err := Read(ctx, coll, bson.M{"_id": id}, "marker.evidence")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coll.UpdateOne(ctx, bson.M{"_id": id}, change); err != nil {
			t.Fatal(err)
		}
		_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) { return nil, before.Apply(tx, coll, proof) })
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("changed baseline accepted: %v", err)
		}
	}
}

func primitiveCaseID(value bson.M) string {
	for _, field := range value {
		for name := range field.(bson.M) {
			return name
		}
	}
	return "invalid"
}

func TestRetirementCASConcurrentAndStaleIdempotentSnapshotsDoNotSucceed(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	coll := db.Collection("facts")
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: "1"}, {Key: "value", Value: "business"}}); err != nil {
		t.Fatal(err)
	}
	baseline, err := Read(ctx, coll, bson.M{"_id": "1"}, "evidence")
	if err != nil {
		t.Fatal(err)
	}
	proof := historyProof("event:1", evidence.BindingDigest("test", evidence.String("business")))
	first, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer first.EndSession(ctx)
	second, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer second.EndSession(ctx)
	if err := first.StartTransaction(); err != nil {
		t.Fatal(err)
	}
	if err := second.StartTransaction(); err != nil {
		t.Fatal(err)
	}
	firstCtx, secondCtx := mongo.NewSessionContext(ctx, first), mongo.NewSessionContext(ctx, second)
	var read bson.Raw
	if err := coll.FindOne(secondCtx, bson.M{"_id": "1"}).Decode(&read); err != nil {
		t.Fatal(err)
	}
	if err := baseline.Apply(firstCtx, coll, proof); err != nil {
		t.Fatal(err)
	}
	if err := first.CommitTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if err := baseline.Apply(secondCtx, coll, proof); !hasTransientLabel(err) {
		t.Fatalf("stale second writer claimed success: %v", err)
	}
	if err := second.AbortTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	// A transaction that read an already identical proof still must not claim
	// idempotent success after a competing business write outside that snapshot.
	if err := second.StartTransaction(); err != nil {
		t.Fatal(err)
	}
	secondCtx = mongo.NewSessionContext(ctx, second)
	if err := coll.FindOne(secondCtx, bson.M{"_id": "1"}).Decode(&read); err != nil {
		t.Fatal(err)
	}
	if _, err := coll.UpdateOne(ctx, bson.M{"_id": "1"}, bson.M{"$set": bson.M{"value": "concurrently changed"}}); err != nil {
		t.Fatal(err)
	}
	if err := baseline.Apply(secondCtx, coll, proof); !hasTransientLabel(err) {
		t.Fatalf("stale idempotent reader claimed current success: %v", err)
	}
	if err := second.AbortTransaction(ctx); err != nil {
		t.Fatal(err)
	}
}

func hasTransientLabel(err error) bool {
	var serverError mongo.ServerError
	return errors.As(err, &serverError) && serverError.HasErrorLabel("TransientTransactionError")
}
