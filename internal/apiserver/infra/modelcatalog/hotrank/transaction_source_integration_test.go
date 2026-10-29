//go:build integration

package hotrank

import (
	"context"
	"os"
	"testing"
	"time"

	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This verifies real original-root commits and source reconstruction. It does
// not claim Broker fault coverage; that belongs to the RocketMQ native suite.
func rankMongoFixture(t *testing.T) (context.Context, *mongo.Database, *MongoDaySource, sourceSheet) {
	t.Helper()
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	require.NotEmpty(t, uri, "required disposable Mongo replica-set URI")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })
	db := client.Database("qs_hotrank_tx_test_" + uuid.NewString())
	t.Cleanup(func() { require.NoError(t, db.Drop(context.Background())) })
	require.NoError(t, txmongo.InstallSchema(ctx, db))
	_, err = db.Collection("rm_tx_binding").InsertOne(ctx, bson.M{"_id": "qs.mongo", "store_id": "hotrank-fixture", "writer_epoch": "fixture", "schema_version": 1})
	require.NoError(t, err)
	_, err = db.Collection("answersheets").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "filled_at", Value: 1}, {Key: "domain_id", Value: 1}}, Options: options.Index().SetName(DayRebuildIndex)})
	require.NoError(t, err)
	store, err := txmongo.New(db, transaction.Binding{Owner: "qs.mongo", StoreID: "hotrank-fixture", WriterEpoch: "fixture"})
	require.NoError(t, err)
	sheet, _, plan := rankTransactionFixture(t)
	reservation, err := store.Reserve(ctx, transaction.RequestFor(plan), plan, transaction.ReserveOptions{Lease: time.Minute})
	require.NoError(t, err)
	session, err := client.StartSession()
	require.NoError(t, err)
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) {
		guard, err := store.Bind(sc, reservation.Execution)
		if err != nil {
			return nil, err
		}
		if _, err = db.Collection("answersheets").InsertOne(sc, sheet); err != nil {
			return nil, err
		}
		return nil, guard.CommitWitness(sc)
	})
	require.NoError(t, err)
	source, err := NewMongoDaySource(db)
	require.NoError(t, err)
	return ctx, db, source, sheet
}

func TestHotRankRebuildFromCurrentTransactionFactsWithoutOutbox(t *testing.T) {
	ctx, db, source, sheet := rankMongoFixture(t)
	day := sheet.FilledAt.In(hotRankTimezone).Format("20060102")
	first, err := source.Capture(ctx, day)
	require.NoError(t, err)
	require.Len(t, first.Facts, 1)
	one, _, err := first.Manifest()
	require.NoError(t, err)
	second, err := source.Capture(ctx, day)
	require.NoError(t, err)
	two, _, err := second.Manifest()
	require.NoError(t, err)
	require.Equal(t, one, two)
	names, err := db.ListCollectionNames(ctx, bson.M{})
	require.NoError(t, err)
	require.NotContains(t, names, "rm_outbox")
	projection, _, _ := rebuildFixture(t)
	projection.now = func() time.Time { return time.Now() }
	lease, err := projection.AcquireRebuild(ctx, day, uuid.NewString())
	require.NoError(t, err)
	first, err = source.Capture(ctx, day)
	require.NoError(t, err)
	second, err = source.Capture(ctx, day)
	require.NoError(t, err)
	receipt, err := projection.ApplyDayRebuild(ctx, lease, first, "integration")
	require.NoError(t, err)
	repeated, err := projection.ApplyDayRebuild(ctx, lease, second, "integration")
	require.NoError(t, err)
	require.Equal(t, receipt.Fingerprint, repeated.Fingerprint)
	// Missing original evidence refuses the entire day, rather than lowering rank.
	_, err = db.Collection("rm_tx_execution").DeleteMany(ctx, bson.M{})
	require.NoError(t, err)
	_, err = source.Capture(ctx, day)
	require.Error(t, err)
}

func TestHotRankRejectsIncompleteCurrentTransactionDay(t *testing.T) {
	for _, name := range []string{"missing_execution", "prepared_execution", "aborted_witness", "missing_witness", "wrong_execution_owner", "changed_plan_digest", "missing_proof", "mixed_proof", "unknown_version", "changed_business_identity", "duplicate_event", "missing_index", "partial_index", "truncated_day"} {
		t.Run(name, func(t *testing.T) {
			ctx, db, source, sheet := rankMongoFixture(t)
			executions := db.Collection("rm_tx_execution")
			sheets := db.Collection("answersheets")
			var err error
			switch name {
			case "missing_execution":
				_, err = executions.DeleteMany(ctx, bson.M{})
			case "prepared_execution":
				_, err = executions.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"state": transaction.Prepared}})
			case "aborted_witness":
				_, err = executions.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"witnesses.0.state": transaction.Aborted}})
			case "missing_witness":
				_, err = executions.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"witnesses": bson.A{}}})
			case "wrong_execution_owner":
				_, err = executions.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"owner": "qs.mysql"}})
			case "changed_plan_digest":
				_, err = executions.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"plan_digest": make([]byte, 32)}})
			case "missing_proof":
				_, err = sheets.UpdateOne(ctx, bson.M{}, bson.M{"$unset": bson.M{"durable_acceptance.transaction_evidence": ""}})
			case "mixed_proof":
				_, err = sheets.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"durable_acceptance.event_evidence": bson.M{"version": 1}}})
			case "unknown_version":
				_, err = sheets.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"durable_acceptance.schema_version": 99}})
			case "changed_business_identity":
				_, err = sheets.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"questionnaire_code": "another-questionnaire"}})
			case "duplicate_event":
				_, err = sheets.InsertOne(ctx, sheet)
			case "missing_index", "partial_index":
				_, err = sheets.Indexes().DropOne(ctx, DayRebuildIndex)
				if name == "partial_index" && err == nil {
					_, err = sheets.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "filled_at", Value: 1}, {Key: "domain_id", Value: 1}}, Options: options.Index().SetName(DayRebuildIndex).SetPartialFilterExpression(bson.M{"domain_id": bson.M{"$gt": 0}})})
				}
			case "truncated_day":
				rows := make([]any, MaxRebuildFacts)
				for i := range rows {
					rows[i] = sheet
				}
				_, err = sheets.InsertMany(ctx, rows)
			}
			require.NoError(t, err)
			snapshot, err := source.Capture(ctx, sheet.FilledAt.In(hotRankTimezone).Format("20060102"))
			require.Error(t, err)
			require.Empty(t, snapshot.Facts, "no partial repair may escape a conflicting day")
		})
	}
}

func TestHotRankMixedHistoricalAndCurrentDay(t *testing.T) {
	ctx, db, source, current := rankMongoFixture(t)
	legacy, row, _ := rankTransactionFixtureAt(t, 101, "Q-B", current.FilledAt)
	intent, err := transactionIntent(legacy, row)
	require.NoError(t, err)
	legacy.DurableAcceptance.SchemaVersion = 1
	legacy.DurableAcceptance.TransactionEvidence = nil
	_, err = db.Collection("answersheets").InsertOne(ctx, legacy)
	require.NoError(t, err)
	raw, err := bson.Marshal(intent)
	require.NoError(t, err)
	var old bson.M
	require.NoError(t, bson.Unmarshal(raw, &old))
	old["_id"] = bson.D{{Key: "producer", Value: intent.Producer}, {Key: "message_id", Value: intent.ID}, {Key: "destination", Value: intent.Destination}}
	_, err = db.Collection("rm_outbox").InsertOne(ctx, old)
	require.NoError(t, err)
	// Soft-deleted submissions remain accepted facts in the complete day.
	_, err = db.Collection("answersheets").UpdateOne(ctx, bson.M{"domain_id": current.ID}, bson.M{"$set": bson.M{"deleted_at": time.Now()}})
	require.NoError(t, err)
	// An unmarked row in the next UTC+8 business day must not enter this snapshot.
	start, err := rebuildDay(current.FilledAt.In(hotRankTimezone).Format("20060102"), time.Now())
	require.NoError(t, err)
	_, err = db.Collection("answersheets").InsertOne(ctx, bson.M{"domain_id": 102, "filled_at": start.AddDate(0, 0, 1).UTC()})
	require.NoError(t, err)
	snapshot, err := source.Capture(ctx, start.Format("20060102"))
	require.NoError(t, err)
	require.Len(t, snapshot.Facts, 2)
	_, counts, err := snapshot.Manifest()
	require.NoError(t, err)
	require.Equal(t, map[string]int{"Q-A": 1, "Q-B": 1}, counts)
	// Historical responsibility remains mandatory even in an otherwise valid v2 day.
	_, err = db.Collection("rm_outbox").DeleteMany(ctx, bson.M{})
	require.NoError(t, err)
	snapshot, err = source.Capture(ctx, start.Format("20060102"))
	require.Error(t, err)
	require.Empty(t, snapshot.Facts)
}
