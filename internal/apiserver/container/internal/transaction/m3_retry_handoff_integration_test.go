//go:build integration && m3_retry_store

package transaction_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongorepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	mongoreplay "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/outboxhistory"
	mysqlstore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	mysqlreplay "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func handoffCases() map[string]bool {
	return map[string]bool{"legacy_transfer_root_rollback_atomic": true, "legacy_transfer_exact_first_wire_and_due": true, "legacy_transfer_repeat_keeps_single_owner": true, "legacy_transfer_attempted_or_due_refused": true, "legacy_replay_cannot_reauthorize_transferred": true, "legacy_old_relay_cannot_claim_transferred": true, "legacy_transferred_status_not_pending": true}
}
func handoffReplay(f model.Frozen) request.ReplayRequest {
	return request.ReplayRequest{OrgID: f.Authority().OrgID, RequestID: "native-handoff-replay", Store: "native-handoff", Reason: "verify retired responsibility is refused", Targets: []request.ReplayTarget{{EventID: f.Message().Input().ID, ExpectedFailureCount: 1}}}
}
func waitHandoffDue(t *testing.T, f model.Frozen) {
	t.Helper()
	timer := time.NewTimer(time.Until(f.DueAt().Add(2 * time.Millisecond)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal("handoff deadline wait cancelled")
	}
}
func verifyLegacyHandoffMySQL(t *testing.T, db *gorm.DB, store *mysqlstore.Store, f model.Frozen, binding transaction.Binding) map[string]bool {
	pool, err := db.DB()
	require.NoError(t, err)
	root := modtx.NewMySQLRunner(db)
	authority := checkpoint.NewRepository(db)
	// Convert only this disposable test's unexecuted schedule into a legacy-only
	// starting fixture. There are no SDK executions or production rows here.
	key := f.Key()
	removed, err := pool.ExecContext(t.Context(), "DELETE FROM qs_retry_schedule WHERE schedule_key=? AND state='ready' AND revision=1", key[:])
	require.NoError(t, err)
	count, err := removed.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	_, err = pool.ExecContext(t.Context(), string(retryMigration(t, "mysql", "000084_standard_reliable_outbox.up.sql")))
	require.NoError(t, err)
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		tx, err := hostmysql.RequireTx(ctx)
		if err != nil {
			return err
		}
		a, err := sdkmysql.BindGORM(tx)
		if err != nil {
			return err
		}
		return a.Append(ctx, f.Message(), f.DueAt())
	}))
	abort := errors.New("controlled handoff root rollback")
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		already, err := store.TransferLegacy(ctx, f, authority)
		if err != nil {
			return err
		}
		require.False(t, already)
		return abort
	})
	require.ErrorIs(t, err, abort)
	_, err = store.Find(t.Context(), f)
	require.ErrorIs(t, err, transaction.ErrNotFound)
	var state, code string
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT state,last_error_code FROM rm_outbox WHERE message_id=?", f.Message().Input().ID).Scan(&state, &code))
	require.Equal(t, "pending", state)
	require.Empty(t, code)
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		tx, err := hostmysql.RequireTx(ctx)
		if err != nil {
			return err
		}
		if err = tx.Exec("UPDATE rm_outbox SET attempt_count=1 WHERE message_id=?", f.Message().Input().ID).Error; err != nil {
			return err
		}
		_, err = store.TransferLegacy(ctx, f, authority)
		return err
	})
	require.ErrorIs(t, err, model.ErrLegacyOwnership)
	cliCases := map[string]bool{}
	if handoffCLIRequired(t) {
		var database string
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&database))
		var tablesBefore, tablesAfter int
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", database).Scan(&tablesBefore))
		cfg, err := driver.ParseDSN(os.Getenv("QS_M3_RETRY_DSN"))
		require.NoError(t, err)
		cfg.DBName = database
		cliCases = nativeHandoffCLI(t, binding, f, database, "QS_RETRY_HANDOFF_MYSQL_DSN", cfg.FormatDSN())
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", database).Scan(&tablesAfter))
		require.Equal(t, tablesBefore, tablesAfter)
		var requests, executions int
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_request").Scan(&requests))
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_execution").Scan(&executions))
		require.Zero(t, requests)
		require.Zero(t, executions)
		cliCases["offline_cli_no_new_tables_or_collections"] = true
		cliCases["offline_cli_no_sdk_transaction_rows"] = true
		func() {
			_, err := pool.ExecContext(t.Context(), "RENAME TABLE rm_outbox TO rm_outbox_native_hidden")
			require.NoError(t, err)
			defer func() {
				_, err := pool.ExecContext(context.Background(), "RENAME TABLE rm_outbox_native_hidden TO rm_outbox")
				require.NoError(t, err)
			}()
			nativeMissingHandoffLedger(t, binding, database, "QS_RETRY_HANDOFF_MYSQL_DSN", cfg.FormatDSN())
		}()
		cliCases["offline_inventory_missing_ledger_not_empty"] = true
	} else {
		require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
			already, err := store.TransferLegacy(ctx, f, authority)
			require.False(t, already)
			return err
		}))
	}
	first, err := store.Find(t.Context(), f)
	require.NoError(t, err)
	require.Equal(t, f.Bytes(), first.Plan.Bytes())
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		already, err := store.TransferLegacy(ctx, f, authority)
		require.True(t, already)
		return err
	}))
	var raw []byte
	var version uint64
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT state,last_error_code,payload,version FROM rm_outbox WHERE message_id=?", f.Message().Input().ID).Scan(&state, &code, &raw, &version))
	require.Equal(t, "held", state)
	require.Equal(t, model.HandoffMarker(f), code)
	require.Equal(t, f.Message().Input().Payload, raw)
	require.EqualValues(t, 1, version)
	replay, err := mysqlreplay.NewReplayLedger(pool, "native-handoff")
	require.NoError(t, err)
	results, err := replay.Authorize(t.Context(), handoffReplay(f))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Authorized)
	require.Equal(t, "future_schedule_owned", results[0].Reason)
	statusReader, err := mysqlreplay.NewStatusReader(pool)
	require.NoError(t, err)
	snapshot, err := statusReader.OutboxStatusSnapshot(t.Context(), time.Now())
	require.NoError(t, err)
	require.Len(t, snapshot.Buckets, 4)
	for _, bucket := range snapshot.Buckets {
		require.Zero(t, bucket.Count)
		require.Nil(t, bucket.OldestCreatedAt)
		require.Zero(t, bucket.OldestAgeSeconds)
	}
	_, err = pool.ExecContext(t.Context(), "UPDATE rm_outbox SET last_error_code='invalid-transfer' WHERE message_id=?", f.Message().Input().ID)
	require.NoError(t, err)
	_, err = statusReader.OutboxStatusSnapshot(t.Context(), time.Now())
	require.Error(t, err, "malformed held state cannot be hidden")
	_, err = pool.ExecContext(t.Context(), "UPDATE rm_outbox SET last_error_code=? WHERE message_id=?", model.HandoffMarker(f), f.Message().Input().ID)
	require.NoError(t, err)
	verifyMySQLHandoffHistory(t, pool, store, f, statusReader)
	waitHandoffDue(t, f)
	old, err := sdkmysql.New(pool)
	require.NoError(t, err)
	claims, err := old.ClaimDue(t.Context(), 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, claims)
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		tx, err := hostmysql.RequireTx(ctx)
		if err != nil {
			return err
		}
		if err = tx.Exec("UPDATE rm_outbox SET state='pending',last_error_code='' WHERE message_id=?", f.Message().Input().ID).Error; err != nil {
			return err
		}
		_, err = store.TransferLegacy(ctx, f, authority)
		return err
	})
	require.ErrorIs(t, err, model.ErrLegacyOwnership)
	cases := handoffCases()
	for key, passed := range cliCases {
		cases[key] = passed
	}
	return cases
}
func verifyMySQLHandoffHistory(t *testing.T, pool *sql.DB, store *mysqlstore.Store, f model.Frozen, status *mysqlreplay.StatusReader) {
	t.Helper()
	ctx := t.Context()
	plans := []model.Frozen{f}
	expected, err := store.InspectLegacy(ctx, plans)
	require.NoError(t, err)
	require.True(t, expected.ResponsibilitiesAccounted)
	_, err = store.InspectHistory(ctx, plans)
	require.Error(t, err, "an explicit archive audit cannot substitute the live table")
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := pool.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}
	exec("CREATE TABLE isolated_outbox_history_rows LIKE rm_outbox")
	exec("INSERT INTO isolated_outbox_history_rows SELECT * FROM rm_outbox")
	exec("CREATE SQL SECURITY INVOKER VIEW qs_rm_outbox_history AS SELECT * FROM isolated_outbox_history_rows")
	exec(string(retryMigration(t, "mysql", "000090_qs_gap_recovery_request.up.sql")))
	org := f.Authority().OrgID
	eventID := f.Message().Input().ID
	exec(`INSERT INTO qs_rm_gap_recovery_request
		(org_id,request_id,actor_id,assessment_id,event_id,expected_version,reason,submitted_before,input_hash,result_code,authorized,outbox_version_after)
		VALUES(?,'retained-decision',1,?, ?,1,'isolated archive audit','2026-11-10T00:00:00Z',UNHEX(REPEAT('aa',32)),'authorized',1,1)`, org, f.Authority().AssessmentID, eventID)
	gaps, err := mysqlreplay.NewGapRecoveryLedger(pool)
	require.NoError(t, err)

	// Live governance must observe current work even while a valid archive exists.
	exec("UPDATE rm_outbox SET state='retry_wait' WHERE message_id=?", eventID)
	live, err := status.ReadOutboxGovernance(ctx, org)
	require.NoError(t, err)
	require.EqualValues(t, 1, live.Automatic)
	candidates, err := status.ListOutboxCandidates(ctx, org, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, eventID, candidates[0].ResourceID)
	exec("UPDATE rm_outbox SET state='held' WHERE message_id=?", eventID)
	exec("RENAME TABLE rm_outbox TO isolated_original_rm_outbox")
	_, err = store.InspectLegacy(ctx, plans)
	require.Error(t, err, "live inventory cannot substitute archived evidence")
	history, err := store.InspectHistory(ctx, plans)
	require.NoError(t, err)
	require.Equal(t, expected, history)
	unselected, err := store.InspectHistory(ctx, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, unselected.UnverifiedHeld)
	require.False(t, unselected.ResponsibilitiesAccounted)
	archived, err := status.ReadOutboxGovernance(ctx, org)
	require.NoError(t, err)
	require.Zero(t, archived.Automatic)
	candidates, err = status.ListOutboxCandidates(ctx, org, 10)
	require.NoError(t, err)
	require.Empty(t, candidates)
	snapshot, err := status.OutboxStatusSnapshot(ctx, time.Now())
	require.NoError(t, err)
	for _, bucket := range snapshot.Buckets {
		require.Zero(t, bucket.Count)
	}
	summary, err := gaps.ReadSummary(ctx, org)
	require.NoError(t, err)
	require.EqualValues(t, 1, summary.Authorized)
	require.Zero(t, summary.WaitingRelay)

	if handoffCLIRequired(t) {
		nativeArchiveHandoffLedger(t, pool, store.Binding(), f)
	}

	exec("UPDATE isolated_outbox_history_rows SET last_error_code=CONCAT('m3_future_owner:',REPEAT('a',64)) WHERE message_id=?", eventID)
	_, err = status.OutboxStatusSnapshot(ctx, time.Now())
	require.ErrorIs(t, err, outboxhistory.ErrUnresolvedArchive)
	exec("UPDATE isolated_outbox_history_rows SET last_error_code=? WHERE message_id=?", model.HandoffMarker(f), eventID)
	exec("UPDATE isolated_outbox_history_rows SET state='retry_wait' WHERE message_id=?", eventID)
	_, err = status.OutboxStatusSnapshot(ctx, time.Now())
	require.ErrorIs(t, err, outboxhistory.ErrUnresolvedArchive)
	_, err = status.ReadOutboxGovernance(ctx, org)
	require.ErrorIs(t, err, outboxhistory.ErrUnresolvedArchive)
	_, err = status.ListOutboxCandidates(ctx, org, 10)
	require.ErrorIs(t, err, outboxhistory.ErrUnresolvedArchive)
	_, err = gaps.ReadSummary(ctx, org)
	require.ErrorIs(t, err, outboxhistory.ErrUnresolvedArchive)
	_, err = store.InspectHistory(ctx, plans)
	require.Error(t, err, "selected source no longer matches the retained handoff")
	exec("RENAME TABLE isolated_original_rm_outbox TO rm_outbox")
	exec("DROP VIEW qs_rm_outbox_history")
	restored, err := store.InspectLegacy(ctx, plans)
	require.NoError(t, err)
	require.Equal(t, expected, restored)
	t.Log("F06_MYSQL_HISTORY_PROOF live_preferred=true archive_handoff_exact=true archived_pending_refused=true live_inventory_never_substitutes=true")
}

func verifyLegacyHandoffMongo(t *testing.T, db *mongo.Database, store *mongostore.Store, f model.Frozen, binding transaction.Binding) map[string]bool {
	root := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{Boundary: "interpretation_commit", Limiter: backpressure.NewLimiter(2, time.Second)})
	authority, err := mongorepo.NewRetryDispatchAuthority(db)
	require.NoError(t, err)
	key := f.Key()
	removed, err := db.Collection("qs_retry_schedule").DeleteOne(t.Context(), bson.M{"_id": key[:], "state": model.Ready, "revision": int64(1)})
	require.NoError(t, err)
	require.EqualValues(t, 1, removed.DeletedCount)
	require.NoError(t, db.CreateCollection(t.Context(), "rm_outbox"))
	require.NoError(t, db.CreateCollection(t.Context(), "qs_rm_replay_requests"))
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		sc, ok := ctx.(mongo.SessionContext)
		if !ok {
			return transaction.ErrTransactionRequired
		}
		a, err := sdkmongo.Bind(sc, db.Collection("rm_outbox"))
		if err != nil {
			return err
		}
		return a.Append(f.Message(), f.DueAt())
	}))
	abort := errors.New("controlled handoff root rollback")
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		already, err := store.TransferLegacy(ctx, f, authority)
		if err != nil {
			return err
		}
		require.False(t, already)
		return abort
	})
	require.ErrorIs(t, err, abort)
	_, err = store.Find(t.Context(), f)
	require.ErrorIs(t, err, transaction.ErrNotFound)
	filter := bson.M{"message_id": f.Message().Input().ID}
	var source struct {
		State   string `bson:"state"`
		Code    string `bson:"last_error_code"`
		Payload []byte `bson:"payload"`
		Version int64  `bson:"version"`
	}
	require.NoError(t, db.Collection("rm_outbox").FindOne(t.Context(), filter).Decode(&source))
	require.Equal(t, "pending", source.State)
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := db.Collection("rm_outbox").UpdateOne(ctx, filter, bson.M{"$set": bson.M{"attempt_count": int64(1)}}); err != nil {
			return err
		}
		_, err := store.TransferLegacy(ctx, f, authority)
		return err
	})
	require.ErrorIs(t, err, model.ErrLegacyOwnership)
	cliCases := map[string]bool{}
	if handoffCLIRequired(t) {
		before, err := db.ListCollectionNames(t.Context(), bson.D{})
		require.NoError(t, err)
		require.Contains(t, before, "rm_tx_execution")
		cliCases = nativeHandoffCLI(t, binding, f, db.Name(), "QS_RETRY_HANDOFF_MONGO_URI", os.Getenv("QS_M3_MONGO_URI"))
		after, err := db.ListCollectionNames(t.Context(), bson.D{})
		require.NoError(t, err)
		require.ElementsMatch(t, before, after)
		for _, name := range []string{"rm_tx_request", "rm_tx_execution"} {
			count, err := db.Collection(name).CountDocuments(t.Context(), bson.D{})
			require.NoError(t, err)
			require.Zero(t, count)
		}
		cliCases["offline_cli_no_new_tables_or_collections"] = true
		cliCases["offline_cli_no_sdk_transaction_rows"] = true
		nativeMongoHistoryHandoffCLI(t, binding, f, db.Name(), true, 1)
		legacy := db.Collection("rm_outbox")
		archive := db.Collection("qs_rm_outbox_history")
		var original bson.Raw
		require.NoError(t, legacy.FindOne(t.Context(), filter).Decode(&original))
		_, err = archive.InsertOne(t.Context(), original)
		require.NoError(t, err)
		nativeMongoHistoryHandoffCLI(t, binding, f, db.Name(), false, 2)
		nativeMongoHistoryHandoffCLI(t, binding, f, db.Name(), true, 0)
		func() {
			admin := db.Client().Database("admin")
			require.NoError(t, admin.RunCommand(t.Context(), bson.D{{Key: "renameCollection", Value: db.Name() + ".rm_outbox"}, {Key: "to", Value: db.Name() + ".rm_outbox_native_hidden"}}).Err())
			defer func() {
				require.NoError(t, admin.RunCommand(context.Background(), bson.D{{Key: "renameCollection", Value: db.Name() + ".rm_outbox_native_hidden"}, {Key: "to", Value: db.Name() + ".rm_outbox"}}).Err())
			}()
			nativeMissingHandoffLedger(t, binding, db.Name(), "QS_RETRY_HANDOFF_MONGO_URI", os.Getenv("QS_M3_MONGO_URI"))
			nativeMongoHistoryHandoffCLI(t, binding, f, db.Name(), true, 0)
			var retained bson.Raw
			require.NoError(t, archive.FindOne(t.Context(), filter).Decode(&retained))
			require.Equal(t, original, retained)
			_, err := archive.UpdateOne(t.Context(), filter, bson.M{"$set": bson.M{"last_error_code": "m3_future_owner:" + strings.Repeat("0", 64)}})
			require.NoError(t, err)
			nativeMongoHistoryHandoffCLI(t, binding, f, db.Name(), true, 1)
			_, err = archive.DeleteOne(t.Context(), filter)
			require.NoError(t, err)
			var changed bson.D
			require.NoError(t, bson.Unmarshal(original, &changed))
			require.Equal(t, "_id", changed[0].Key)
			changed[0].Value = "substituted-bson-identity"
			_, err = archive.InsertOne(t.Context(), changed)
			require.NoError(t, err)
			nativeMongoHistoryHandoffCLI(t, binding, f, db.Name(), true, 1)
		}()
		var retained bson.Raw
		require.NoError(t, legacy.FindOne(t.Context(), filter).Decode(&retained))
		require.Equal(t, original, retained)
		cliCases["offline_inventory_missing_ledger_not_empty"] = true
		cliCases["offline_mongo_history_requires_explicit_archive"] = true
		cliCases["offline_mongo_history_verifies_only_reviewed_held"] = true
		cliCases["offline_mongo_history_preserves_original_and_rejects_false_marker"] = true
		t.Log("F06_MONGO_SCHEDULE_PROOF live_missing_refused=true explicit_archive_required=true unselected_held_blocked=true exact_pair=true false_marker_refused=true substituted_bson_id_refused=true original_unchanged=true")
	} else {
		require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
			already, err := store.TransferLegacy(ctx, f, authority)
			require.False(t, already)
			return err
		}))
	}
	first, err := store.Find(t.Context(), f)
	require.NoError(t, err)
	require.Equal(t, f.Bytes(), first.Plan.Bytes())
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		already, err := store.TransferLegacy(ctx, f, authority)
		require.True(t, already)
		return err
	}))
	require.NoError(t, db.Collection("rm_outbox").FindOne(t.Context(), filter).Decode(&source))
	require.Equal(t, "held", source.State)
	require.Equal(t, model.HandoffMarker(f), source.Code)
	require.Equal(t, f.Message().Input().Payload, source.Payload)
	require.EqualValues(t, 1, source.Version)
	replay, err := mongoreplay.NewReplayLedger(db, "native-handoff")
	require.NoError(t, err)
	results, err := replay.Authorize(t.Context(), handoffReplay(f))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Authorized)
	require.Equal(t, "future_schedule_owned", results[0].Reason)
	statusReader, err := mongoreplay.NewStatusReader(db.Collection("rm_outbox"))
	require.NoError(t, err)
	snapshot, err := statusReader.OutboxStatusSnapshot(t.Context(), time.Now())
	require.NoError(t, err)
	require.Len(t, snapshot.Buckets, 4)
	for _, bucket := range snapshot.Buckets {
		require.Zero(t, bucket.Count)
		require.Nil(t, bucket.OldestCreatedAt)
		require.Zero(t, bucket.OldestAgeSeconds)
	}
	buckets, err := statusReader.OutboxStatusByEventType(t.Context(), time.Now())
	require.NoError(t, err)
	require.Empty(t, buckets)
	_, err = db.Collection("rm_outbox").UpdateOne(t.Context(), filter, bson.M{"$set": bson.M{"last_error_code": "invalid-transfer"}})
	require.NoError(t, err)
	_, err = statusReader.OutboxStatusSnapshot(t.Context(), time.Now())
	require.Error(t, err, "malformed held state cannot be hidden")
	_, err = statusReader.OutboxStatusByEventType(t.Context(), time.Now())
	require.Error(t, err)
	_, err = db.Collection("rm_outbox").UpdateOne(t.Context(), filter, bson.M{"$set": bson.M{"last_error_code": model.HandoffMarker(f)}})
	require.NoError(t, err)
	waitHandoffDue(t, f)
	old, err := sdkmongo.New(db.Collection("rm_outbox"))
	require.NoError(t, err)
	claims, err := old.ClaimDue(t.Context(), 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, claims)
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := db.Collection("rm_outbox").UpdateOne(ctx, filter, bson.M{"$set": bson.M{"state": "pending", "last_error_code": ""}}); err != nil {
			return err
		}
		_, err := store.TransferLegacy(ctx, f, authority)
		return err
	})
	require.ErrorIs(t, err, model.ErrLegacyOwnership)
	cases := handoffCases()
	for key, passed := range cliCases {
		cases[key] = passed
	}
	return cases
}
