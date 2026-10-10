//go:build integration

package retirement

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Authenticate and persist two real SQL and two real Mongo originals through
// the existing two-epoch spool. No test-populated qualification or Q is used.
// Each mutation is confined to this fixture's random namespace and restored
// before the next independent read epoch; current standard facts stay intact.
func TestFinalHistoricalEOFNativeMongoPersistedMissingConflictAndSourceDrift(t *testing.T) {
	sqlDB, _, db, config, session := historicalSpoolNativeFixture(t)
	spool, copies := historicalSpoolNativePrepared(t, sqlDB, db, config, session)
	bounded, cancel, err := spool.InheritedContext(t.Context())
	if err != nil {
		t.Fatal("original spool deadline unavailable", err)
	}
	defer cancel()
	if applied, err := historicalSpoolNativeApply(t, bounded, sqlDB, db, session, spool, false); err != nil || applied == nil {
		t.Fatal("actual fixture-only SQL and Mongo commit failed", err)
	}
	historicalSpoolNativeCounts(t, sqlDB, db, true)

	verify := func(t *testing.T, wantSuccess bool) {
		t.Helper()
		var observed *FinalHistoricalObservation
		err := persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, _ *MongoResponsibilitySnapshot, _ *gorm.DB) error {
			q, end := context.WithDeadline(ctx, deadlineFromSpoolNative(t, bounded))
			defer end()
			paired := mongo.NewSessionContext(q, mongo.SessionFromContext(ctx))
			var err error
			observed, err = PrepareFinalHistoricalEOF(paired, FinalHistoricalInput{Binding: coordinatorBinding(), SQLIdentity: current.Report().DatabaseIdentitySHA256, SQLHead: 99, MongoDatabase: db, MongoConfig: config, Copies: copies.inputs()})
			if err == nil {
				r := observed.Summary()
				if r.EventReferences != 4 || r.SourceReferences != 4 || r.AICommands != 0 || r.UnverifiableReferences > 4 || r.DropReady || r.CASAuthority || r.WholeWriterFence || !evidenceHash(r.PersistedReferencesSHA256) {
					return errors.New("final readback changed scope or granted authority")
				}
				return observed.ValidateBorrowedSnapshot(paired)
			}
			return err
		})
		if wantSuccess {
			if err != nil || observed == nil {
				t.Fatal("nonempty four-source persisted readback failed", err)
			}
			if observed.ValidateBorrowedSnapshot(t.Context()) == nil {
				t.Fatal("ended native read epochs remained valid")
			}
		} else if err == nil || observed != nil {
			t.Fatal("missing, conflicting or changed Mongo original completed")
		}
	}
	t.Run("persisted", func(t *testing.T) { verify(t, true) })

	for _, target := range []struct{ collection, slot string }{
		{"answersheets", "legacy_submission_evidence"},
		{"report_generations", "historical_generated_evidence"},
	} {
		for _, mode := range []string{"missing", "conflict"} {
			t.Run(target.collection+"/"+mode, func(t *testing.T) {
				collection := db.Collection(target.collection)
				if n, err := collection.CountDocuments(t.Context(), bson.D{}); err != nil || n != 1 {
					t.Fatal("exact owned business document unavailable", err)
				}
				var original bson.Raw
				if err := collection.FindOne(t.Context(), bson.D{}).Decode(&original); err != nil {
					t.Fatal(err)
				}
				id := original.Lookup("_id")
				filter := bson.D{{Key: "_id", Value: id}}
				defer func() {
					if result, err := collection.ReplaceOne(t.Context(), filter, original); err != nil || result == nil || result.MatchedCount != 1 {
						t.Error("exact fixture business row restoration failed", err)
					}
				}()
				change := bson.D{{Key: "$unset", Value: bson.D{{Key: target.slot, Value: ""}}}}
				if mode == "conflict" {
					change = bson.D{{Key: "$set", Value: bson.D{{Key: target.slot + ".entries.0.proof.business_binding_sha256", Value: sourceSHA([]byte("different-owned-business-binding"))}}}}
				}
				if result, err := collection.UpdateOne(t.Context(), filter, change); err != nil || result == nil || result.MatchedCount != 1 || result.ModifiedCount != 1 {
					t.Fatal("owned evidence mutation ineffective", err)
				}
				verify(t, false)
			})
		}
	}
	t.Run("source_delete", func(t *testing.T) {
		collection := db.Collection("domain_event_outbox")
		var original bson.Raw
		if err := collection.FindOne(t.Context(), bson.D{}).Decode(&original); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := collection.InsertOne(t.Context(), original); err != nil {
				t.Error("exact owned original restoration failed", err)
			}
		}()
		if result, err := collection.DeleteOne(t.Context(), bson.D{{Key: "_id", Value: original.Lookup("_id")}}); err != nil || result == nil || result.DeletedCount != 1 {
			t.Fatal("owned source deletion ineffective", err)
		}
		verify(t, false)
	})
	t.Run("restored_originals", func(t *testing.T) { verify(t, true) })
	historicalSpoolNativeCounts(t, sqlDB, db, true)
}
