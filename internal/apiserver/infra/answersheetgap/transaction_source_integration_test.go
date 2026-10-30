//go:build integration

package answersheetgap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func currentGapNativeFixture(t *testing.T) (context.Context, *mongo.Database, *gorm.DB, *Scanner, sheetRow) {
	t.Helper()
	uri, dsn := os.Getenv("QS_GAP_TEST_MONGO_URI"), os.Getenv("QS_GAP_TEST_MYSQL_DSN")
	require.True(t, strings.HasPrefix(uri, "mongodb://127.0.0.1:") && strings.Contains(uri, "replicaSet="), "explicit owned Mongo replica set required")
	cfg, err := drivermysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, cfg.Net == "tcp" && strings.HasPrefix(cfg.Addr, "127.0.0.1:"), "explicit owned MySQL service required")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	name := "qs_gap_tx_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	require.NoError(t, admin.WithContext(ctx).Exec("CREATE DATABASE `"+name+"`").Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE `"+name+"`").Error) })
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.WithContext(ctx).Exec("CREATE TABLE assessment (id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT UNSIGNED NOT NULL, answer_sheet_id BIGINT UNSIGNED NOT NULL UNIQUE, deleted_at DATETIME(3) NULL)").Error)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })
	mdb := client.Database(name)
	t.Cleanup(func() { require.NoError(t, mdb.Drop(context.Background())) })
	require.NoError(t, txmongo.InstallSchema(ctx, mdb))
	_, err = mdb.Collection("rm_tx_binding").InsertOne(ctx, bson.M{"_id": "qs.mongo", "store_id": "gap-fixture", "writer_epoch": "fixture", "schema_version": 1})
	require.NoError(t, err)
	_, err = mdb.Collection("answersheets").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "durable_acceptance.schema_version", Value: 1}, {Key: "deleted_at", Value: 1}, {Key: "domain_id", Value: 1}}, Options: options.Index().SetName("idx_answersheet_durable_audit")})
	require.NoError(t, err)
	store, err := txmongo.New(mdb, transaction.Binding{Owner: "qs.mongo", StoreID: "gap-fixture", WriterEpoch: "fixture"})
	require.NoError(t, err)
	raw, sheet, _, plan := currentGapFixture(t)
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
		if _, err = mdb.Collection("answersheets").InsertOne(sc, raw); err != nil {
			return nil, err
		}
		return nil, guard.CommitWitness(sc)
	})
	require.NoError(t, err)
	scanner, err := New(mdb, db)
	require.NoError(t, err)
	return ctx, mdb, db, scanner, sheet
}

func TestCurrentGapScanAndRecoveryWithoutOutbox(t *testing.T) {
	ctx, mdb, db, scanner, sheet := currentGapNativeFixture(t)
	cutoff, now := time.Now().Add(-10*time.Second), time.Now()
	page, err := scanner.ScanPage(ctx, 0, sheet.DomainID, cutoff, 10)
	require.NoError(t, err)
	require.Len(t, page.Findings, 1)
	require.Equal(t, CommittedGap, page.Findings[0].Disposition)
	review, err := scanner.CaptureOriginalRecovery(ctx, sheet.DomainID, sheet.OrgID, cutoff, now)
	require.NoError(t, err)
	require.Equal(t, sheet.DurableAcceptance.EventID, review.Message().Input().ID)
	names, err := mdb.ListCollectionNames(ctx, bson.M{})
	require.NoError(t, err)
	require.NotContains(t, names, "rm_outbox")
	// Broker observation may advance without changing committed source authority.
	_, err = mdb.Collection("rm_tx_execution").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"witnesses.0.final_outcome": "accepted", "witnesses.0.broker_message_id": "observed-later"}})
	require.NoError(t, err)
	again, err := scanner.CaptureOriginalRecovery(ctx, sheet.DomainID, sheet.OrgID, cutoff, time.Now())
	require.NoError(t, err)
	require.Equal(t, review.SourceFingerprint, again.SourceFingerprint)
	require.NoError(t, db.WithContext(ctx).Exec("INSERT INTO assessment(id,org_id,answer_sheet_id) VALUES(?,?,?)", 801, sheet.OrgID, sheet.DomainID).Error)
	page, err = scanner.ScanPage(ctx, 0, sheet.DomainID, cutoff, 10)
	require.NoError(t, err)
	require.Equal(t, Present, page.Findings[0].Disposition)
	_, err = scanner.CaptureOriginalRecovery(ctx, sheet.DomainID, sheet.OrgID, cutoff, time.Now())
	require.ErrorIs(t, err, ErrUnsafeRecoverySource)
	require.NoError(t, db.WithContext(ctx).Exec("UPDATE assessment SET deleted_at=? WHERE id=801", time.Now()).Error)
	page, err = scanner.ScanPage(ctx, 0, sheet.DomainID, cutoff, 10)
	require.NoError(t, err)
	require.Equal(t, ManualRequired, page.Findings[0].Disposition)
	require.Equal(t, "assessment_previously_accepted", page.Findings[0].Reason)
	_, err = scanner.CaptureOriginalRecovery(ctx, sheet.DomainID, sheet.OrgID, cutoff, time.Now())
	require.ErrorIs(t, err, ErrUnsafeRecoverySource)
}

func TestCurrentGapRejectsMissingOrConflictingSource(t *testing.T) {
	for _, name := range []string{"missing_execution", "prepared", "missing_witness", "wrong_owner", "wrong_digest", "missing_proof", "downgraded_version", "unknown_version", "missing_version", "missing_acceptance_clock", "changed_admission"} {
		t.Run(name, func(t *testing.T) {
			ctx, mdb, _, scanner, sheet := currentGapNativeFixture(t)
			var err error
			switch name {
			case "missing_execution":
				_, err = mdb.Collection("rm_tx_execution").DeleteMany(ctx, bson.M{})
			case "prepared":
				_, err = mdb.Collection("rm_tx_execution").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"state": transaction.Prepared}})
			case "missing_witness":
				_, err = mdb.Collection("rm_tx_execution").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"witnesses": bson.A{}}})
			case "wrong_owner":
				_, err = mdb.Collection("rm_tx_execution").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"owner": "qs.mysql"}})
			case "wrong_digest":
				_, err = mdb.Collection("rm_tx_execution").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"plan_digest": make([]byte, 32)}})
			case "missing_proof":
				_, err = mdb.Collection("answersheets").UpdateOne(ctx, bson.M{}, bson.M{"$unset": bson.M{"durable_acceptance.transaction_evidence": ""}})
			case "downgraded_version":
				_, err = mdb.Collection("answersheets").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"durable_acceptance.schema_version": 1}})
			case "unknown_version":
				_, err = mdb.Collection("answersheets").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"durable_acceptance.schema_version": 99}})
			case "missing_version":
				_, err = mdb.Collection("answersheets").UpdateOne(ctx, bson.M{}, bson.M{"$unset": bson.M{"durable_acceptance.schema_version": ""}})
			case "missing_acceptance_clock":
				_, err = mdb.Collection("answersheets").UpdateOne(ctx, bson.M{}, bson.M{"$unset": bson.M{"durable_acceptance.accepted_at": ""}})
			case "changed_admission":
				_, err = mdb.Collection("answersheets").UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"admission.model_version": "changed"}})
			}
			require.NoError(t, err)
			cutoff := time.Now().Add(-10 * time.Second)
			page, err := scanner.ScanPage(ctx, 0, sheet.DomainID, cutoff, 10)
			require.NoError(t, err)
			require.Len(t, page.Findings, 1, "unknown source must not disappear as a clean empty scan")
			require.Equal(t, ManualRequired, page.Findings[0].Disposition)
			_, err = scanner.CaptureOriginalRecovery(ctx, sheet.DomainID, sheet.OrgID, cutoff, time.Now())
			require.Error(t, err)
		})
	}
}

func TestCurrentGapMixedHistoricalAndCommittedSources(t *testing.T) {
	ctx, mdb, _, scanner, current := currentGapNativeFixture(t)
	_, legacy, intent, confirmation, _ := recoveryFixture(t)
	legacy.DomainID = 43
	// Rebuild the historical wire for this legacy sheet identity.
	intent.Payload = makeWire(t, legacy, legacy.Admission.ModelCode)
	intent.Fingerprint = fingerprint(t, intent)
	raw, err := bson.Marshal(legacy)
	require.NoError(t, err)
	_, err = mdb.Collection("answersheets").InsertOne(ctx, raw)
	require.NoError(t, err)
	raw, err = bson.Marshal(intent)
	require.NoError(t, err)
	var old bson.M
	require.NoError(t, bson.Unmarshal(raw, &old))
	old["_id"] = bson.D{{Key: "producer", Value: producer}, {Key: "message_id", Value: intent.MessageID}, {Key: "destination", Value: destination}}
	old["version"], old["attempt_count"], old["transport_confirmed_at"] = confirmation.Version, confirmation.Attempts, confirmation.TransportConfirmedAt
	_, err = mdb.Collection("rm_outbox").InsertOne(ctx, old)
	require.NoError(t, err)
	page, err := scanner.ScanPage(ctx, 0, legacy.DomainID, time.Now().Add(-10*time.Second), 10)
	require.NoError(t, err)
	require.Len(t, page.Findings, 2)
	require.Equal(t, current.DomainID, page.Findings[0].AnswerSheetID)
	require.Equal(t, CommittedGap, page.Findings[0].Disposition)
	require.Equal(t, Missing, page.Findings[1].Disposition)
}
