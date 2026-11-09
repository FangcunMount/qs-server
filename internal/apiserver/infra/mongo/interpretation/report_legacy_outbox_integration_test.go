//go:build integration

package interpretation

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestLegacyReportFutureWriteRetirementOnMongo(t *testing.T) {
	for _, state := range []transaction.State{transaction.Prepared, transaction.Aborted, transaction.Committed} {
		t.Run(string(state), func(t *testing.T) {
			ctx := t.Context()
			client, db := mongodbtest.ReplicaSetDatabase(t)
			require.NoError(t, InstallLifecycleSchema(ctx, db))
			require.NoError(t, txmongo.InstallSchema(ctx, db))
			require.NoError(t, db.CreateCollection(ctx, "rm_outbox"))
			binding := transaction.Binding{Owner: "qs.mongo", StoreID: "legacy-report-fixture", WriterEpoch: "m3-1"}
			_, err := db.Collection("rm_tx_binding").InsertOne(ctx, bson.M{"_id": binding.Owner, "store_id": binding.StoreID, "writer_epoch": binding.WriterEpoch, "schema_version": 1})
			require.NoError(t, err)
			store, err := txmongo.New(db, binding)
			require.NoError(t, err)
			cfg := legacyReportReadConfig(t, binding)
			owner, err := guarded.New(db, cfg, []string{"qs.evaluation.lifecycle"})
			require.NoError(t, err)
			q := reportTransactionRequest(t)
			require.NoError(t, NewGenerationRepositoryWithoutSchema(db).Create(ctx, q.Generation))
			require.NoError(t, NewRunRepositoryWithoutSchema(db).Create(ctx, q.Run))
			current, err := freezeReportFailure(app.CommitFailureRequest{Generation: q.Generation, Run: q.Run,
				OutcomeID: q.Generation.Key().OutcomeID, Association: q.InterpretReport.Association(), FailedAt: q.CompletedAt,
				Failure: run.Failure{Kind: run.FailureKindBuild, Code: "controlled", SafeMessage: "fixture", Retryable: true}}, reportRoutes{})
			require.NoError(t, err)
			legacy := current
			frozen, err := readReportPlan(legacy.Artifact)
			require.NoError(t, err)
			frozen.Version = 1
			legacy.Artifact, err = json.Marshal(frozen)
			require.NoError(t, err)
			plan, err := shared.FreezeWithRoutes(legacy, reportRoutes{}, binding.Owner, "m3-1", owner.Runtime().Routes())
			require.NoError(t, err)
			original, err := store.Reserve(ctx, transaction.RequestFor(plan), plan, transaction.ReserveOptions{Lease: time.Minute})
			require.NoError(t, err)
			require.True(t, original.Acquired)
			newPlan, err := shared.FreezeWithRoutes(current, reportRoutes{}, binding.Owner, "m3-1", owner.Runtime().Routes())
			require.NoError(t, err)
			duplicate, err := store.Reserve(ctx, transaction.RequestFor(newPlan), newPlan, transaction.ReserveOptions{Lease: time.Minute})
			require.NoError(t, err)
			require.False(t, duplicate.Acquired)
			require.Equal(t, plan.Digest(), duplicate.Execution.Plan.Digest())

			if state != transaction.Prepared {
				seedLegacyReportRoot(t, db, store, original.Execution, frozen, state)
			}
			counts, err := InventoryLegacyFutures(ctx, db)
			require.NoError(t, err)
			expected := LegacyFutureInventory{Executions: 1, ReportTerminals: 1}
			switch state {
			case transaction.Prepared:
				expected.Prepared = 1
			case transaction.Committed:
				expected.Committed = 1
			case transaction.Aborted:
				expected.Aborted = 1
			}
			require.Equal(t, expected, counts)

			admin := client.Database("admin")
			require.NoError(t, admin.RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: db.Name() + ".rm_outbox"},
				{Key: "to", Value: db.Name() + ".qs_rm_outbox_history"},
			}).Err())
			history := db.Collection("qs_rm_outbox_history")
			var retained bson.Raw
			if state == transaction.Committed {
				require.NoError(t, history.FindOne(ctx, bson.M{"message_id": frozen.Future.ID}).Decode(&retained))
			} else {
				count, err := history.CountDocuments(ctx, bson.D{})
				require.NoError(t, err)
				require.Zero(t, count)
			}
			protocol := readLegacyReportBytes(t, db, original.Execution)
			fresh, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("QS_SERVER_TEST_MONGO_URI")))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, fresh.Disconnect(context.Background())) })
			freshDB := fresh.Database(db.Name())
			freshOwner, err := guarded.New(freshDB, cfg, []string{"qs.evaluation.lifecycle"})
			require.NoError(t, err)
			committer, err := NewTransactionCommitter(freshDB, freshOwner, apptransaction.RunnerFunc(func(context.Context, func(context.Context) error) error {
				t.Error("read-only historical recovery entered a business root")
				return port.ErrUnresolved
			}), reportRoutes{})
			require.NoError(t, err)
			result, handled, err := committer.RecoverPending(ctx, q.Generation.Key(), 7)
			if state == transaction.Committed {
				require.NoError(t, err)
				require.False(t, handled, "original committed failure resumes its normal retry authority")
				require.Nil(t, result)
				var after bson.Raw
				require.NoError(t, history.FindOne(ctx, bson.M{"message_id": frozen.Future.ID}).Decode(&after))
				require.Equal(t, retained, after)
			} else {
				require.ErrorIs(t, err, port.ErrUnresolved)
				require.True(t, handled)
				require.Nil(t, result)
				saved, err := NewGenerationRepositoryWithoutSchema(freshDB).FindByID(ctx, q.Generation.ID())
				require.NoError(t, err)
				require.Equal(t, generation.StatusGenerating, saved.Status())
				savedRun, err := NewRunRepositoryWithoutSchema(freshDB).FindByID(ctx, q.Run.ID())
				require.NoError(t, err)
				require.Equal(t, run.StatusRunning, savedRun.Status())
			}
			require.Equal(t, protocol, readLegacyReportBytes(t, db, original.Execution))
			collections, err := db.ListCollectionNames(ctx, bson.M{"name": "rm_outbox"})
			require.NoError(t, err)
			require.Empty(t, collections)
			// Reject a missing request even though the execution and its
			// witness otherwise still decode.
			requestKey := transaction.RequestFor(plan).KeyDigest()
			_, err = db.Collection("rm_tx_request").DeleteOne(ctx, bson.M{"_id": requestKey[:]})
			require.NoError(t, err)
			counts, err = InventoryLegacyFutures(ctx, db)
			require.Error(t, err)
			require.Empty(t, counts)

		})
	}
	if !t.Failed() {
		t.Log("F06_LEGACY_REPORT_ROOT_PROOF prepared_refused=true rollback_preserved=true committed_restart_read_only=true frozen_witness_bytes_unchanged=true retired_collection_not_recreated=true")
		t.Log("F06_FROZEN_REPORT_INVENTORY_PROOF prepared_committed_aborted_counted=true missing_request_refused=true")
	}
}

func seedLegacyReportRoot(t *testing.T, db *mongo.Database, store *txmongo.Store, execution transaction.Execution, f frozenReportCommit, state transaction.State) {
	t.Helper()
	ctx := t.Context()
	session, err := db.Client().StartSession()
	require.NoError(t, err)
	defer session.EndSession(ctx)
	rolledBack := errors.New("controlled old root rollback")
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) {
		guard, err := store.Bind(sc, execution)
		if err != nil {
			return nil, err
		}
		result, terminal, err := materializeReportCommit(f, execution)
		if err != nil {
			return nil, err
		}
		// Use the current terminal CAS and the historical appender in the
		// fixture's SDK-guarded root. Recovery uses a new production owner.
		written, err := db.Collection("interpretation_runs").UpdateOne(sc, terminal.filter, terminal.update)
		if err != nil {
			return nil, err
		}
		if written.MatchedCount != 1 {
			return nil, generation.ErrVersionConflict
		}
		if err := NewGenerationRepositoryWithoutSchema(db).Save(sc, result.Generation, result.Generation.Version()-1); err != nil {
			return nil, err
		}
		future, err := message.New(*f.Future)
		if err != nil {
			return nil, err
		}
		appender, err := sdkmongo.Bind(sc, db.Collection("rm_outbox"))
		if err != nil {
			return nil, err
		}
		if err := appender.Append(future, f.Due); err != nil {
			return nil, err
		}
		if err := guard.CommitWitness(sc); err != nil {
			return nil, err
		}
		if state == transaction.Aborted {
			return nil, rolledBack
		}
		return nil, nil
	})
	if state == transaction.Aborted {
		require.ErrorIs(t, err, rolledBack)
		aborted, err := store.Abort(ctx, execution)
		require.NoError(t, err)
		require.True(t, aborted)
		return
	}
	require.NoError(t, err)
}

func readLegacyReportBytes(t *testing.T, db *mongo.Database, execution transaction.Execution) [2]bson.Raw {
	t.Helper()
	key := transaction.ExecutionKey(execution.Plan.Input().Owner, execution.Plan.Input().OperationID, execution.WriterEpoch, execution.Fence)
	requestKey := transaction.RequestFor(execution.Plan).KeyDigest()
	var stored [2]bson.Raw
	require.NoError(t, db.Collection("rm_tx_execution").FindOne(t.Context(), bson.M{"_id": key[:]}).Decode(&stored[0]))
	require.NoError(t, db.Collection("rm_tx_request").FindOne(t.Context(), bson.M{"_id": requestKey[:]}).Decode(&stored[1]))
	return stored
}

func legacyReportReadConfig(t *testing.T, binding transaction.Binding) *qsoptions.MessagingOptions {
	t.Helper()
	dir := t.TempDir()
	server := httptest.NewTLSServer(nil)
	certificate := server.Certificate().Raw
	server.Close()
	write := func(name string, content []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, content, 0600))
		return path
	}
	const topic = "qs.evaluation.lifecycle"
	physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, binding.Owner, "m3-1")
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]any{"version": 2, "routes": []map[string]string{
		{"topic": topic, "physical_topic": physical, "message_type": "TRANSACTION", "owner": binding.Owner, "route_revision": "m3-1"},
	}})
	require.NoError(t, err)
	disabled := false
	cfg := qsoptions.NewMessagingOptions()
	cfg.Provider = "rocketmq"
	cfg.RocketMQ = &qsoptions.RocketMQOptions{
		Endpoint: "127.0.0.1:1", PublishEnabled: &disabled,
		CredentialsFile: write("credentials.json", []byte(`{"username":"fixture","password":"fixture-only"}`)),
		CAFile:          write("ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})),
		RoutesFile:      write("routes.json", raw),
		Transactions: map[string]qsoptions.TransactionOptions{
			binding.Owner: {StoreID: binding.StoreID, WriterEpoch: binding.WriterEpoch, RouteRevision: "m3-1"},
		},
	}
	return cfg
}
