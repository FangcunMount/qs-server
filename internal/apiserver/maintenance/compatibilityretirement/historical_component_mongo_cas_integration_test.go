//go:build integration

package retirement

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// This PRIVATE effect fixture supplies synthetic graph/reference inputs. Its
// reads, physical CAS, rollback, commit and independent server reads are real.
// It grants no full-source/SQL/AI authority and exercises no production caller.
func mongoComponentPrivateCASFixture(t *testing.T) (*mongo.Client, *mongo.Database, *MongoSnapshotInputEpoch, *HistoricalCASComponent) {
	t.Helper()
	client, db, input, original := mongoComponentNativeFixture(t)
	var raw bson.Raw
	if err := db.Collection("answersheets").FindOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(30_001)}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	index, frames := componentFixture(t, 2)
	for i, f := range frames {
		f.rows = []historicalCASRowInput{{historicalCASRowKey{"mongodb", "answersheets", 30_001}, historicalSpoolSHA(raw), uint64(len(raw)), true}}
		f.mongoRead = original.inputs[0].mongoRead
		entry, _, _, _ := persistenceTestReference()
		entry.EventID, entry.EventType = "component-private-"+strconv.Itoa(i+1), "answersheet.submitted"
		entry.Source.Database, entry.Source.PrimaryKeyKind = "mongodb", "mongodb_objectid"
		entry.Source.PrimaryKeySHA256 = strings.Repeat(strconv.Itoa(i+1), 64)
		entry.Source.Digest = evidence.SourceDigest(MongoRowDigestKind, []byte("synthetic frozen source "+strconv.Itoa(i+1)))
		entry.Proof.EventID, entry.Proof.Digest = entry.EventID, entry.Source.Digest
		if err := entry.Validate(); err != nil {
			t.Fatal(err)
		}
		set, err := (*evidence.HistoricalReferenceSetV1)(nil).Append(entry)
		if err != nil {
			t.Fatal(err)
		}
		f.mongoCAS = &mongoHistoricalComponentCASRecipe{identity: input.metadata.identity, sqlIdentity: strings.Repeat("3", 64), sqlRows: strings.Repeat("4", 64), groups: []mongoCASGroup{{collection: "answersheets", slot: "legacy_submission_evidence", id: 30_001, pk: raw.Lookup("_id"), set: set, entries: []evidence.HistoricalReferenceEntryV1{entry}}}, attachments: []mongoCASAttachmentFacts{{collection: "answersheets", slot: "legacy_submission_evidence", id: 30_001, entry: entry, content: entry.Source.Digest}}}
		f.seal = f.digest()
	}
	components, err := PrepareHistoricalCASComponents(t.Context(), index, frames, DefaultHistoricalCASComponentLimits())
	if err != nil || len(components.Components()) != 1 || len(components.Components()[0].inputs) != 2 {
		t.Fatal("overlapping writable owners escaped a single complete component", err)
	}
	return client, db, input, components.Components()[0]
}

func TestMongoHistoricalComponentPrivateCASCommitAndExpectedServerReadback(t *testing.T) {
	client, db, input, component := mongoComponentPrivateCASFixture(t)
	var statement *MongoHistoricalComponentStatement
	if err := mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error {
		fresh, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if err != nil {
			return err
		}
		statement, err = applyMongoHistoricalComponent(ctx, fresh)
		if err != nil {
			return err
		}
		if statement.VerifyIndependentPersisted(ctx, db, time.Second) == nil {
			t.Fatal("same uncommitted transaction accepted as independent persisted read")
		}
		if retry, err := applyMongoHistoricalComponent(ctx, fresh); retry != nil || err == nil {
			t.Fatal("physical effect instance reused")
		}
		r := statement.Summary()
		if !r.StatementApplied || r.HostCommitVerified || r.BusinessClosureVerified || r.DropReady || !r.FullSourcesRequired || !r.SQLQualificationRequired || !r.AIClosureRequired || !r.HostCommitRequired || !r.IndependentReadbackRequired {
			t.Fatal("private physical CAS claimed business/commit authority", r)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != 2 {
		t.Fatal("coalesced cross-page original IDs lost")
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		return statement.VerifyIndependentPersisted(ctx, db, 10*time.Second)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("answersheets").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(30_001)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "filler_id", Value: int64(456)}}}}); err != nil {
		t.Fatal(err)
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		if err := statement.VerifyIndependentPersisted(ctx, db, 10*time.Second); !errors.Is(err, ErrMongoBatchConflict) {
			t.Fatal("changed actual expected image accepted", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMongoHistoricalComponentPrivateCASRollbackCannotBecomeCommitted(t *testing.T) {
	client, db, input, component := mongoComponentPrivateCASFixture(t)
	var statement *MongoHistoricalComponentStatement
	if err := mongoCASNativeHostWrite(t, client, false, func(ctx mongo.SessionContext) error {
		fresh, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if err != nil {
			return err
		}
		statement, err = applyMongoHistoricalComponent(ctx, fresh)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != 0 {
		t.Fatal("aborted private physical CAS leaked evidence")
	}
	if err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		if err := statement.VerifyIndependentPersisted(ctx, db, 10*time.Second); !errors.Is(err, ErrMongoBatchConflict) {
			t.Fatal("aborted transaction inferred committed persistence", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The host controls its transaction/session and no hidden cleanup/commit ran.
	if statement.Summary().HostCommitVerified {
		t.Fatal("rollback gained host commit proof")
	}
}

func TestMongoHistoricalComponentPrivateCASTransportFailurePoisonsAttempt(t *testing.T) {
	client, db, input, component := mongoComponentPrivateCASFixture(t)
	if err := mongoCASNativeHostWrite(t, client, false, func(ctx mongo.SessionContext) error {
		fresh, err := PrepareMongoHistoricalComponentObservation(ctx, db, component, input, 10*time.Second)
		if err != nil {
			return err
		}
		command := bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: bson.D{{Key: "times", Value: 1}}}, {Key: "data", Value: bson.D{{Key: "failCommands", Value: bson.A{"update"}}, {Key: "closeConnection", Value: true}}}}
		if err := client.Database("admin").RunCommand(t.Context(), command).Err(); err != nil {
			return err
		}
		defer func() {
			if err := client.Database("admin").RunCommand(context.Background(), bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: "off"}}).Err(); err != nil {
				t.Error(err)
			}
		}()
		statement, err := applyMongoHistoricalComponent(ctx, fresh)
		if statement != nil || err == nil || !mongo.IsNetworkError(err) {
			t.Fatal("real transport failure was converted into a success or hidden retry", err)
		}
		if retried, err := applyMongoHistoricalComponent(ctx, fresh); retried != nil || err == nil {
			t.Fatal("transport-unknown physical effect retried with the same authority")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != 0 {
		t.Fatal("host abort after transport failure left evidence")
	}
}
