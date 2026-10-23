//go:build integration && m3_answersheet_integration && m3_host_faults

package transaction_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	answerrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	tx "github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// The infra parent owns a finite packet-loss window for matching original
// session COMMITs, including native driver retries, then restores the real path.
// No client hook substitutes for a server commit or repeats the SDK executor.
func TestM3AnswerOriginalCommitPacketFault(t *testing.T) {
	boundary := os.Getenv("QS_M3_FAULT_BOUNDARY")
	require.Contains(t, []string{"before", "after"}, boundary)
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	uri, err := url.Parse(os.Getenv("QS_M3_MONGO_URI"))
	require.NoError(t, err)
	assertLoopback := func(address string) {
		host, _, e := net.SplitHostPort(address)
		require.NoError(t, e)
		require.True(t, net.ParseIP(host).IsLoopback())
	}
	assertLoopback(uri.Host)
	require.Equal(t, "sdk-proof", uri.Query().Get("replicaSet"))
	require.Equal(t, "true", uri.Query().Get("directConnection"))
	primary, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri.String()))
	require.NoError(t, err)
	db := primary.Database(fmt.Sprintf("qs_m3_packet_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, db.Drop(ctx))
		require.NoError(t, primary.Disconnect(ctx))
	})
	proxy := os.Getenv("QS_M3_FAULT_MONGO_PROXY")
	assertLoopback(proxy)
	uri.Host = proxy
	original, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri.String()))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, original.Disconnect(ctx))
	})
	originalDB := original.Database(db.Name())
	load := func(key string) qsoptions.MessagingOptions {
		var value qsoptions.MessagingOptions
		raw, e := os.ReadFile(os.Getenv(key))
		require.NoError(t, e)
		require.NoError(t, json.Unmarshal(raw, &value))
		require.NotNil(t, value.RocketMQ)
		assertLoopback(value.RocketMQ.Endpoint)
		return value
	}
	producer, observer := load("QS_M3_PRODUCER_CONFIG"), load("QS_M3_OBSERVER_CONFIG")
	cfg, err := eventcatalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	catalog := eventcatalog.NewCatalog(cfg)
	topic, found := catalog.GetTopicForEvent(domain.EventTypeSubmitted)
	require.True(t, found)
	// Explicit installation on the owned primary, before any runtime is started.
	require.NoError(t, answerrepo.NewRepositoryWithoutSchema(db).InstallSubmissionIndexes(t.Context()))
	require.NoError(t, txmongo.InstallSchema(t.Context(), db))
	config := producer.RocketMQ.Transactions["qs.mongo"]
	binding := tx.Binding{Owner: "qs.mongo", StoreID: config.StoreID, WriterEpoch: config.WriterEpoch}
	_, err = db.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": binding.Owner, "store_id": binding.StoreID, "writer_epoch": binding.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	authority, err := txmongo.New(db, binding)
	require.NoError(t, err)
	root := modtx.NewMongoRunner(originalDB, modtx.MongoRunnerOptions{Boundary: "answersheet_submit", Limiter: backpressure.NewLimiter(2, time.Second)})
	var calls, callbacks atomic.Int32
	var sessionID bson.Raw
	var sessionMu sync.Mutex
	rootFinished := make(chan struct{})
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		calls.Add(1)
		defer close(rootFinished)
		return root.WithinTransaction(ctx, func(active context.Context) error {
			callbacks.Add(1)
			sessionMu.Lock()
			sessionID = append(bson.Raw(nil), mongo.SessionFromContext(active).ID()...)
			sessionMu.Unlock()
			return fn(active)
		})
	})
	store, err := answerrepo.NewRocketMQSubmissionStore(answerrepo.NewRepositoryWithoutSchema(originalDB), runner, &producer, catalog)
	require.NoError(t, err)
	stop := func(value *answerrepo.RocketMQSubmissionStore) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, value.Runtime().Close(ctx))
	}
	t.Cleanup(func() { stop(store) })
	require.NoError(t, store.Runtime().Start(t.Context()))
	transportConfig, err := observer.RocketMQ.Load([]string{topic}, 1, 5*time.Second)
	require.NoError(t, err)
	consumer, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: transportConfig, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
	require.NoError(t, err)
	closeConsumer := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, consumer.Close(ctx))
	}
	t.Cleanup(closeConsumer)
	received := make(chan transport.Received, 4)
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-answer-observer", func(_ context.Context, d transport.Delivery) error {
		if e := d.Ack(); e != nil {
			return e
		}
		select {
		case received <- d.Message():
			return nil
		default:
			return errors.New("bounded answer fault observer full")
		}
	}, func(context.Context, transport.Failure) error {
		return errors.New("answer fault observer refused failure")
	}))
	require.Eventually(t, func() bool { return consumer.Ready(10 * time.Second) }, 30*time.Second, 50*time.Millisecond)
	sheet, meta := m3AnswerSheet(t, 901, "original-packet-input")
	// The caller deadline is not proof that the driver returned: its commit loop
	// uses a background context. Keep real capacity until path recovery and join.
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	result, _, applicationError := store.CreateDurably(ctx, sheet, meta)
	cancel()
	require.Equal(t, int32(1), calls.Load())
	require.GreaterOrEqual(t, callbacks.Load(), int32(1))
	sessionMu.Lock()
	exactSession := append(bson.Raw(nil), sessionID...)
	sessionMu.Unlock()
	require.NotEmpty(t, exactSession)
	if boundary == "before" {
		// Dispose the exact still-open server session after the caller deadline.
		// The driver's real COMMIT loop can outlive that deadline; this alone is
		// not accepted as an SDK rollback decision or a released execution slot.
		require.NoError(t, primary.Database("admin").RunCommand(t.Context(), bson.D{{Key: "killSessions", Value: bson.A{exactSession}}}).Err())
	}
	control := os.Getenv("QS_M3_FAULT_PATH_RESTORED")
	require.True(t, filepath.IsAbs(control))
	require.True(t, strings.HasPrefix(filepath.Base(filepath.Dir(control)), "rocketmq-m2-"))
	// The parent first captures real proxy events/counters, then restores the
	// path for this original session. It does not synthesize a COMMIT response.
	fmt.Println("M3_QS_MONGO_FAULT_STATE {\"event\":\"fault_window_ready\"}")
	require.Eventually(t, func() bool {
		raw, e := os.ReadFile(control)
		return e == nil && string(raw) == "original-path-restored"
	}, 15*time.Second, 50*time.Millisecond)
	select {
	case <-rootFinished:
	case <-time.After(35 * time.Second):
		t.Fatal("original Mongo driver did not return after the real network path recovered")
	}
	require.LessOrEqual(t, callbacks.Load(), int32(16))
	require.Eventually(t, func() bool { return store.Runtime().Stats().ActiveSends == 0 }, 15*time.Second, 50*time.Millisecond)
	var execution struct {
		Plan []byte `bson:"plan"`
	}
	require.NoError(t, db.Collection("rm_tx_execution").FindOne(t.Context(), bson.M{"owner": binding.Owner}).Decode(&execution))
	plan, err := tx.DecodePlan(execution.Plan)
	require.NoError(t, err)
	reservation, err := authority.Lookup(t.Context(), tx.RequestFor(plan))
	require.NoError(t, err)
	reference := reservation.Execution.References()[0]
	evidence, err := authority.Evidence(t.Context(), reference)
	require.NoError(t, err)
	decision := tx.Resolve(reference, evidence)
	facts, err := db.Collection("answersheets").CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err)
	if boundary == "before" {
		require.Error(t, applicationError)
		require.Nil(t, result)
		require.Zero(t, facts)
		require.NotEqual(t, tx.Commit, decision)
	} else {
		require.NoError(t, applicationError)
		require.NotNil(t, result)
		require.Equal(t, sheet.ID(), result.ID())
		require.Equal(t, int64(1), facts)
		require.Equal(t, tx.Commit, decision)
	}
	duplicate, existed, duplicateError := store.CreateDurably(t.Context(), sheet, meta)
	if boundary == "after" {
		require.NoError(t, duplicateError)
		require.True(t, existed)
		require.Equal(t, result.ID(), duplicate.ID())
	} else {
		require.Error(t, duplicateError)
		require.Nil(t, duplicate)
	}
	require.Equal(t, int32(1), calls.Load())
	winner, err := authority.Lookup(t.Context(), tx.RequestFor(plan))
	require.NoError(t, err)
	require.Equal(t, plan.Digest(), winner.Execution.Plan.Digest())
	require.Equal(t, int64(1), store.Runtime().Stats().HalfPreparedResults)
	stop(store)
	paused := producer
	cloned := *producer.RocketMQ
	publish := false
	cloned.PublishEnabled = &publish
	paused.RocketMQ = &cloned
	newCalls := atomic.Int32{}
	newRunner := apptransaction.RunnerFunc(func(context.Context, func(context.Context) error) error {
		newCalls.Add(1)
		return errors.New("paused replacement cannot execute original business")
	})
	next, err := answerrepo.NewRocketMQSubmissionStore(answerrepo.NewRepositoryWithoutSchema(db), newRunner, &paused, catalog)
	require.NoError(t, err)
	t.Cleanup(func() { stop(next) })
	require.NoError(t, next.Runtime().Start(t.Context()))
	expected := tx.Commit
	if boundary == "before" {
		expected = tx.Rollback
	}
	require.Eventually(t, func() bool {
		read, done := context.WithTimeout(t.Context(), 2*time.Second)
		defer done()
		ev, e := authority.Evidence(read, reference)
		stats := next.Runtime().Stats()
		checks := stats.CheckCommit
		if boundary == "before" {
			checks = stats.CheckRollback
		}
		return e == nil && tx.Resolve(reference, ev) == expected && (boundary == "after" || checks > 0)
	}, 100*time.Second, 100*time.Millisecond)
	if boundary == "after" {
		select {
		case got := <-received:
			branch := plan.Input().Branches[0]
			require.Equal(t, branch.Message.Input().ID, got.ID)
			require.True(t, bytes.Equal(branch.Wire, got.Wire))
		case <-time.After(45 * time.Second):
			t.Fatal("committed original answer event missing")
		}
	} else {
		select {
		case <-received:
			t.Fatal("uncommitted original answer became visible")
		case <-time.After(2 * time.Second):
		}
	}
	require.Zero(t, newCalls.Load())
	require.Zero(t, next.Runtime().Stats().HalfPreparedResults)
	for _, table := range []string{"rm_outbox", "domain_event_outbox"} {
		count, e := db.Collection(table).CountDocuments(t.Context(), bson.M{})
		require.NoError(t, e)
		require.Zero(t, count)
	}
	stats := next.Runtime().Stats()
	stop(next)
	closeConsumer()
	require.NoError(t, primary.Ping(t.Context(), readpref.Primary()))
	require.NoError(t, original.Ping(t.Context(), readpref.Primary()))
	source, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(source)), "scope": "actual QS AnswerSheet original Mongo root with wire-level commitTransaction faults; component only", "passed": 6, "failed": 0, "skipped": 0, "cases": map[string]bool{"actual_answer_root_once": true, "business_witness_match_boundary": true, "duplicate_keeps_original_plan_and_result": true, "broker_visibility_or_new_paused_checker": true, "published_outboxes_empty": true, "owned_owner_and_transport_drained_primary_retained": true}, "boundary": boundary, "local_calls": calls.Load(), "mongo_callbacks": callbacks.Load(), "half_prepared": store.Runtime().Stats().HalfPreparedResults, "new_half_prepared": stats.HalfPreparedResults, "new_check_commit": stats.CheckCommit, "new_check_rollback": stats.CheckRollback, "business_rows": facts, "wire_unchanged": true, "new_local_calls": newCalls.Load(), "full_api_process_provider_system_not_claimed": true})
	require.NoError(t, err)
	fmt.Println("M3_QS_ANSWER_FAULT_RECEIPT " + string(raw))
}
