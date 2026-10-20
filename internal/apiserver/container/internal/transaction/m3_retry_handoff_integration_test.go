//go:build integration && m3_retry_store

package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongorepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	mongoreplay "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	mysqlstore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	mysqlreplay "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
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
func verifyLegacyHandoffMySQL(t *testing.T, db *gorm.DB, store *mysqlstore.Store, f model.Frozen) map[string]bool {
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
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		already, err := store.TransferLegacy(ctx, f, authority)
		require.False(t, already)
		return err
	}))
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
	require.Empty(t, snapshot.Buckets)
	_, err = pool.ExecContext(t.Context(), "UPDATE rm_outbox SET last_error_code='invalid-transfer' WHERE message_id=?", f.Message().Input().ID)
	require.NoError(t, err)
	_, err = statusReader.OutboxStatusSnapshot(t.Context(), time.Now())
	require.Error(t, err, "malformed held state cannot be hidden")
	_, err = pool.ExecContext(t.Context(), "UPDATE rm_outbox SET last_error_code=? WHERE message_id=?", model.HandoffMarker(f), f.Message().Input().ID)
	require.NoError(t, err)
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
	return handoffCases()
}
func verifyLegacyHandoffMongo(t *testing.T, db *mongo.Database, store *mongostore.Store, f model.Frozen) map[string]bool {
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
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		already, err := store.TransferLegacy(ctx, f, authority)
		require.False(t, already)
		return err
	}))
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
	require.Empty(t, snapshot.Buckets)
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
	return handoffCases()
}
