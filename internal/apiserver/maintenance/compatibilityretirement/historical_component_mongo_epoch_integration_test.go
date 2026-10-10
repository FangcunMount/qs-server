//go:build integration

package retirement

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// The component graph here is the private graph fixture. Its raw baselines,
// metadata, indexes, original/new transactions and rereads are real native
// reads. It does not claim a full-source/SQL/AI-qualified production component.
func mongoComponentNativeFixture(t *testing.T) (*mongo.Client, *mongo.Database, *MongoSnapshotInputEpoch, *HistoricalCASComponent) {
	t.Helper()
	client, db, cfg := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	mongoCASNativeHistoryIndexes(t, db)
	for i := uint64(1); i <= 3; i++ {
		mongoCycleNativeSheet(t, db, 30_000+i, "", "")
	}
	session := snapshotInputNativeSession(t, client)
	input, err := PrepareMongoSnapshotInputEpoch(mongo.NewSessionContext(t.Context(), session), db, cfg, MongoSnapshotInputLimits{Scan: mongoCycleTestLimits(), MaxDuration: time.Minute}, snapshotInputNativeFile(t))
	if err != nil {
		t.Fatal(err)
	}
	session.EndSession(context.Background())
	index, frames := componentFixture(t, 1)
	f := frames[0]
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		global, err := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
		if err != nil {
			return err
		}
		var raw bson.Raw
		if err = db.Collection("answersheets").FindOne(ctx, bson.D{{Key: "domain_id", Value: int64(30_001)}}).Decode(&raw); err != nil {
			return err
		}
		f.rows = []historicalCASRowInput{{historicalCASRowKey{"mongodb", "answersheets", 30_001}, historicalSpoolSHA(raw), uint64(len(raw)), false}}
		f.mongoRead = &mongoHistoricalComponentReadRecipe{config: cfg, metadata: global.metadata.hash, original: global.txn, selection: mongoBatchSelection{sheets: map[uint64]bool{30_001: true}, outcomes: map[uint64]bool{42: true}}, limits: DefaultMongoHistoricalOwnerBatchLimits(), hints: map[string]string{"answersheets:domain_id": "native_domain_unique", "report_generations:outcome_id": "native_outcome", "interpret_report_artifacts:outcome_id": "native_outcome"}}
		f.seal = f.digest()
		c := &HistoricalCASComponent{inputs: frames}
		if _, err := PrepareMongoHistoricalComponentObservation(ctx, db, c, input, time.Second); !errors.Is(err, ErrMongoHistoricalComponentEpoch) {
			t.Fatal("original transaction masqueraded as fresh", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	components, err := PrepareHistoricalCASComponents(t.Context(), index, frames, DefaultHistoricalCASComponentLimits())
	if err != nil {
		t.Fatal(err)
	}
	return client, db, input, components.Components()[0]
}

func TestMongoHistoricalComponentNativeFreshTransactionsAndServerReread(t *testing.T) {
	client, db, input, component := mongoComponentNativeFixture(t)
	var first *MongoHistoricalComponentObservation
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		var err error
		first, err = PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if err != nil {
			return err
		}
		r := first.Summary()
		if r.Rows != 1 || !r.ActualTransactionRead || !r.FrozenBaselinesMatched || r.CASAuthorized || r.HostCommitVerified || r.DropReady || !r.FullSourcesRequired || !r.SQLQualificationRequired || !r.AIClosureRequired {
			t.Fatal("physical subrange claimed write/completion authority", r)
		}
		if first.VerifyIndependentRead(ctx, first) == nil {
			t.Fatal("same transaction accepted as independent reread")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		fresh, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if err != nil {
			return err
		}
		if err = first.VerifyIndependentRead(ctx, fresh); err != nil {
			return err
		}
		if fresh.Summary().NativeTransactionSHA256 == first.Summary().NativeTransactionSHA256 {
			t.Fatal("native transaction identity did not change")
		}
		fresh.started = time.Now().Add(-time.Minute)
		if first.VerifyIndependentRead(ctx, fresh) == nil {
			t.Fatal("expired fresh observation was renewed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMongoHistoricalComponentNativeRejectsNewNegativeRangeDescendant(t *testing.T) {
	client, db, input, component := mongoComponentNativeFixture(t)
	if _, err := db.Collection("report_generations").InsertOne(t.Context(), bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(900)}, {Key: "outcome_id", Value: int64(42)}}); err != nil {
		t.Fatal(err)
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		observed, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if observed != nil || !errors.Is(err, ErrMongoBatchConflict) {
			t.Fatal("new row in originally empty range became zero dependencies")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMongoHistoricalComponentNativeRejectsChangedRowSnapshotModeAndBudget(t *testing.T) {
	client, db, input, component := mongoComponentNativeFixture(t)
	snapshot := snapshotInputNativeSession(t, client)
	if _, err := PrepareMongoHistoricalComponentObservation(mongo.NewSessionContext(t.Context(), snapshot), db, component, input, time.Second); err == nil {
		t.Fatal("input-only snapshot session became transaction qualification")
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		if observed, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 21*time.Second); observed != nil || err == nil {
			t.Fatal("unbounded component budget admitted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("answersheets").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(30_001)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "filler_id", Value: int64(987)}}}}); err != nil {
		t.Fatal(err)
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		observed, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if observed != nil || !errors.Is(err, ErrMongoBatchConflict) {
			t.Fatal("changed native business baseline accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
