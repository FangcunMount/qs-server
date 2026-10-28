//go:build integration && m3_answersheet_integration

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
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	questionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	answerrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	submit "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
)

func m3AnswerSheet(t *testing.T, id uint64, value string) (*domain.AnswerSheet, submit.DurableSubmitMeta) {
	t.Helper()
	ref, err := domain.NewQuestionnaireRef("M3-NATIVE", "1.0.0", "M3 native fixture")
	require.NoError(t, err)
	attribution, err := domain.NewAttributionSnapshot(domain.OriginRef{Type: domain.OriginTypeSelfService}, "", "", "", "", "", time.Now())
	require.NoError(t, err)
	submission, err := domain.NewSubmissionContextWithAttribution(actor.NewFillerRef(301, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(401)), meta.FromUint64(501), "", attribution)
	require.NoError(t, err)
	answer, err := domain.NewAnswer(meta.NewCode("Q1"), questionnaire.TypeText, domain.NewStringValue(value), 0)
	require.NoError(t, err)
	sheet, err := domain.Submit(meta.FromUint64(id), ref, submission, []domain.Answer{answer}, time.Now())
	require.NoError(t, err)
	fingerprint, err := submit.Fingerprint(sheet)
	require.NoError(t, err)
	return sheet, submit.DurableSubmitMeta{WriterID: 301, Fingerprint: fingerprint, IdempotencyKey: fmt.Sprintf("m3-native-%d", id), RequestID: "original-request"}
}

func TestM3AnswerSubmissionBrokerHost(t *testing.T) {
	// No skip path: only the owned infra project may supply this integration.
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	uri, err := url.Parse(os.Getenv("QS_M3_MONGO_URI"))
	require.NoError(t, err)
	host, _, err := net.SplitHostPort(uri.Host)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	require.Equal(t, "sdk-proof", uri.Query().Get("replicaSet"))
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri.String()).SetAppName("qs-m3-answer-fixture"))
	require.NoError(t, err)
	db := client.Database(fmt.Sprintf("qs_m3_answers_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, db.Drop(ctx))
		require.NoError(t, client.Disconnect(ctx))
	})
	load := func(key string) qsoptions.MessagingOptions {
		var o qsoptions.MessagingOptions
		raw, err := os.ReadFile(os.Getenv(key))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &o))
		require.NotNil(t, o.RocketMQ)
		host, _, err := net.SplitHostPort(o.RocketMQ.Endpoint)
		require.NoError(t, err)
		require.True(t, net.ParseIP(host).IsLoopback())
		return o
	}
	producerOptions, observerOptions := load("QS_M3_PRODUCER_CONFIG"), load("QS_M3_OBSERVER_CONFIG")
	config, err := eventcatalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	catalog := eventcatalog.NewCatalog(config)
	topic, found := catalog.GetTopicForEvent(domain.EventTypeSubmitted)
	require.True(t, found)
	repo := answerrepo.NewRepositoryWithoutSchema(db)
	before, err := db.ListCollectionNames(t.Context(), bson.M{})
	require.NoError(t, err)
	require.Empty(t, before, "constructor performed hidden DDL")
	require.NoError(t, repo.InstallSubmissionIndexes(t.Context()))
	require.NoError(t, txmongo.InstallSchema(t.Context(), db))
	// The current host also validates native-limit governance before admission.
	// Install the existing production schema explicitly in this owned database;
	// a missing schema must remain a protection failure, not be bypassed.
	nativeSchema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mongodb", "000043_native_transaction_limit_notices.up.json"))
	require.NoError(t, err)
	var commands []json.RawMessage
	require.NoError(t, json.Unmarshal(nativeSchema, &commands))
	for _, raw := range commands {
		var command bson.D
		require.NoError(t, bson.UnmarshalExtJSON(raw, true, &command))
		require.NoError(t, db.RunCommand(t.Context(), command).Err())
	}
	bindingConfig := producerOptions.RocketMQ.Transactions["qs.mongo"]
	binding := tx.Binding{Owner: "qs.mongo", StoreID: bindingConfig.StoreID, WriterEpoch: bindingConfig.WriterEpoch}
	_, err = db.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": binding.Owner, "store_id": binding.StoreID, "writer_epoch": binding.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	authority, err := txmongo.New(db, binding)
	require.NoError(t, err)
	native, err := messagingruntime.NewMongoNativeLimits(db, binding, "m3-1")
	require.NoError(t, err)
	blocked, err := native.ReadNativeLimit(t.Context())
	require.NoError(t, err, "explicit current native-limit schema is required")
	require.False(t, blocked)
	limiter := backpressure.NewLimiter(2, time.Second)
	rootRunner := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{Boundary: "answersheet_submit", Limiter: limiter})
	var runs, callbackAttempts atomic.Int32
	type hookKey struct{}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		runs.Add(1)
		if hook, ok := ctx.Value(hookKey{}).(func(string) error); ok {
			if err := hook("before_root"); err != nil {
				return err
			}
		}
		err := rootRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
			callbackAttempts.Add(1)
			if err := fn(txCtx); err != nil {
				return err
			}
			if hook, ok := ctx.Value(hookKey{}).(func(string) error); ok {
				return hook("before_commit")
			}
			return nil
		})
		if err == nil {
			if hook, ok := ctx.Value(hookKey{}).(func(string) error); ok {
				return hook("after_commit")
			}
		}
		return err
	})
	store, err := answerrepo.NewRocketMQSubmissionStore(repo, runner, &producerOptions, catalog)
	require.NoError(t, err)
	var durable appanswersheet.SubmissionDurableStore = store
	var reader appanswersheet.SubmissionIdempotencyReader = store
	collectionReadback := newM3CollectionReadback(t, db, repo, store, limiter)
	require.NoError(t, store.Runtime().Start(t.Context()))
	require.True(t, store.Runtime().Stats().Ready, "owned fixture protocol admission not ready")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, store.Runtime().Close(ctx))
	})
	consumerConfig, err := observerOptions.RocketMQ.Load([]string{topic}, 2, 5*time.Second)
	require.NoError(t, err)
	consumer, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: consumerConfig, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, consumer.Close(ctx))
	})
	received := make(chan transport.Received, 16)
	var mu sync.Mutex
	seen := map[string]bool{}
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-answer-observer", func(_ context.Context, delivery transport.Delivery) error {
		msg := delivery.Message()
		if err := delivery.Ack(); err != nil {
			return err
		}
		mu.Lock()
		duplicate := seen[msg.ID]
		seen[msg.ID] = true
		mu.Unlock()
		if !duplicate {
			select {
			case received <- msg:
			default:
				return errors.New("fixture receipt queue full")
			}
		}
		return nil
	}, func(context.Context, transport.Failure) error { return errors.New("fixture delivery failed") }))
	lookup := func(meta submit.DurableSubmitMeta) tx.Reservation {
		var doc struct {
			Plan  []byte `bson:"plan"`
			Fence int64  `bson:"fence"`
			Epoch string `bson:"writer_epoch"`
		}
		// Fixture-only bounded enumeration; no tokens or body are printed.
		cur, err := db.Collection("rm_tx_execution").Find(t.Context(), bson.M{"owner": "qs.mongo"})
		require.NoError(t, err)
		defer cur.Close(t.Context())
		for cur.Next(t.Context()) {
			require.NoError(t, cur.Decode(&doc))
			plan, err := tx.DecodePlan(doc.Plan)
			require.NoError(t, err)
			if strings.HasSuffix(plan.Input().IdempotencyKey, ":"+meta.IdempotencyKey) {
				r, err := authority.Lookup(t.Context(), tx.RequestFor(plan))
				require.NoError(t, err)
				return r
			}
		}
		t.Fatal("exact original reservation missing")
		return tx.Reservation{}
	}
	waitMessage := func(meta submit.DurableSubmitMeta) {
		r := lookup(meta)
		select {
		case message := <-received:
			branch := r.Execution.Plan.Input().Branches[0]
			require.Equal(t, branch.Message.Input().ID, message.ID)
			require.True(t, bytes.Equal(branch.Wire, message.Wire), "first wire changed")
			e, err := authority.Evidence(t.Context(), r.Execution.References()[0])
			require.NoError(t, err)
			require.Equal(t, tx.Commit, tx.Resolve(r.Execution.References()[0], e))
		case <-time.After(45 * time.Second):
			t.Fatal("committed answer event missing")
		}
	}
	cases := map[string]bool{}
	// Half prepared, but neither callback nor fact nor consumer acceptance yet.
	sheet, metaInfo := m3AnswerSheet(t, 101, "accepted")
	entered, release := make(chan struct{}), make(chan struct{})
	ctx := context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_root" {
			close(entered)
			select {
			case <-release:
			case <-t.Context().Done():
				return t.Context().Err()
			}
		}
		return nil
	})
	finished := make(chan error, 1)
	go func() { _, _, err := durable.CreateDurably(ctx, sheet, metaInfo); finished <- err }()
	select {
	case <-entered:
	case err := <-finished:
		require.NoError(t, err, "submission returned before original root")
		t.Fatal("submission completed without original root")
	case <-time.After(30 * time.Second):
		t.Fatal("half preparation did not reach host")
	}
	require.Equal(t, int32(0), callbackAttempts.Load())
	count, err := db.Collection("answersheets").CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err)
	require.Zero(t, count)
	select {
	case <-received:
		t.Fatal("half visible before local root")
	case <-time.After(2 * time.Second):
	}
	cases["half_does_not_accept_business"] = true
	close(release)
	require.NoError(t, <-finished)
	waitMessage(metaInfo)
	completed, err := reader.FindCompleted(t.Context(), metaInfo)
	require.NoError(t, err)
	require.NotNil(t, completed)
	require.Equal(t, sheet.ID(), completed.Sheet.ID())
	cases["original_root_fact_and_witness_and_wire"] = true
	// Same business input with freshly generated IDs must reuse the first fact.
	duplicate, _ := m3AnswerSheet(t, 102, "accepted")
	priorRuns := runs.Load()
	result, existed, err := durable.CreateDurably(t.Context(), duplicate, metaInfo)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, sheet.ID(), result.ID())
	require.Equal(t, priorRuns, runs.Load())
	cases["duplicate_does_not_execute"] = true
	require.Equal(t, 202, collectionReadback(metaInfo.IdempotencyKey, "accepted"))
	cases["collection_202_exact_root"] = true
	changed, changedMeta := m3AnswerSheet(t, 103, "changed")
	changedMeta.IdempotencyKey = metaInfo.IdempotencyKey
	_, _, err = durable.CreateDurably(t.Context(), changed, changedMeta)
	require.ErrorIs(t, err, submit.ErrIdempotencyConflict)
	require.Equal(t, priorRuns, runs.Load())
	cases["conflicting_input_rejected"] = true
	// A persisted answer by itself is insufficient for V2 acceptance. Reject a
	// changed reference even when its fingerprint and committed witness survive.
	_, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": uint64(101)}, bson.M{"$set": bson.M{"durable_acceptance.transaction_evidence.reference.event_type": "answersheet.changed"}})
	require.NoError(t, err)
	_, err = reader.FindCompleted(t.Context(), metaInfo)
	require.ErrorIs(t, err, answerrepo.ErrSubmissionUnresolved)
	require.Equal(t, 503, collectionReadback(metaInfo.IdempotencyKey, "accepted"))
	cases["collection_unknown_witness_no202"] = true
	_, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": uint64(101)}, bson.M{"$set": bson.M{"durable_acceptance.transaction_evidence.reference.event_type": domain.EventTypeSubmitted}})
	require.NoError(t, err)
	cases["full_reference_required_for_acceptance"] = true
	_, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": uint64(101)}, bson.M{"$set": bson.M{"durable_acceptance.schema_version": 1}})
	require.NoError(t, err)
	_, err = reader.FindCompleted(t.Context(), metaInfo)
	require.ErrorIs(t, err, answerrepo.ErrSubmissionUnresolved)
	require.Equal(t, 503, collectionReadback(metaInfo.IdempotencyKey, "accepted"))
	_, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": uint64(101)}, bson.M{"$set": bson.M{"durable_acceptance.schema_version": 2}})
	require.NoError(t, err)
	cases["mixed_protocol_marker_rejected"] = true
	session, err := client.StartSession()
	require.NoError(t, err)
	priorRuns = runs.Load()
	_, _, err = durable.CreateDurably(mongo.NewSessionContext(t.Context(), session), duplicate, metaInfo)
	require.Error(t, err)
	require.Equal(t, priorRuns, runs.Load())
	session.EndSession(t.Context())
	cases["nested_preparation_rejected"] = true
	// The host deliberately discards an actual successful root ACK. This is a
	// named client-level discard proof, not a claim of injected network loss.
	ackSheet, ackMeta := m3AnswerSheet(t, 104, "commit-ack-discard")
	ackCtx := context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "after_commit" {
			return errors.New("fixture discarded successful root ACK")
		}
		return nil
	})
	result, _, err = durable.CreateDurably(ackCtx, ackSheet, ackMeta)
	require.NoError(t, err)
	require.Equal(t, ackSheet.ID(), result.ID())
	waitMessage(ackMeta)
	cases["committed_ack_discard_exact_recovery"] = true
	// An error after callback writes but before root commit must roll back the
	// business document and witnesses together; no accepted result is returned.
	abortedSheet, abortedMeta := m3AnswerSheet(t, 105, "aborted")
	abortCtx := context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_commit" {
			return errors.New("fixture requires root abort")
		}
		return nil
	})
	result, _, err = durable.CreateDurably(abortCtx, abortedSheet, abortedMeta)
	require.Error(t, err)
	require.Nil(t, result)
	count, err = db.Collection("answersheets").CountDocuments(t.Context(), bson.M{"domain_id": uint64(105)})
	require.NoError(t, err)
	require.Zero(t, count)
	e, err := authority.Evidence(t.Context(), lookup(abortedMeta).Execution.References()[0])
	require.NoError(t, err)
	require.NotEqual(t, tx.Committed, e.ExecutionState)
	cases["root_abort_no_accepted_fact_or_witness"] = true
	// Real Mongo failpoint forces the driver's own callback to reenter. The
	// second attempt reuses the already prepared half and frozen BSON.
	retrySheet, retryMeta := m3AnswerSheet(t, 106, "callback-reentry")
	priorCallbacks := callbackAttempts.Load()
	retryCtx := context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_root" {
			return client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: bson.M{"times": 1}}, {Key: "data", Value: bson.M{"failCommands": bson.A{"insert"}, "appName": "qs-m3-answer-fixture", "errorCode": 112, "errorLabels": bson.A{"TransientTransactionError"}}}}).Err()
		}
		return nil
	})
	_, _, err = durable.CreateDurably(retryCtx, retrySheet, retryMeta)
	require.NoError(t, err)
	require.GreaterOrEqual(t, callbackAttempts.Load()-priorCallbacks, int32(2))
	waitMessage(retryMeta)
	cases["native_callback_reentry"] = true
	concurrentSheet, concurrentMeta := m3AnswerSheet(t, 201, "concurrent-input")
	priorRuns = runs.Load()
	concurrentEntered, concurrentRelease := make(chan struct{}), make(chan struct{})
	var enteredOnce sync.Once
	concurrentCtx := context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_root" {
			enteredOnce.Do(func() { close(concurrentEntered) })
			select {
			case <-concurrentRelease:
			case <-t.Context().Done():
				return t.Context().Err()
			}
		}
		return nil
	})
	concurrentFinished := make(chan error, 1)
	go func() {
		_, _, err := durable.CreateDurably(concurrentCtx, concurrentSheet, concurrentMeta)
		concurrentFinished <- err
	}()
	select {
	case <-concurrentEntered:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent winner not prepared")
	}
	contenders := make(chan error, 7)
	for id := uint64(202); id < 209; id++ {
		candidate, _ := m3AnswerSheet(t, id, "concurrent-input")
		go func() { _, _, err := durable.CreateDurably(t.Context(), candidate, concurrentMeta); contenders <- err }()
	}
	for i := 0; i < 7; i++ {
		require.Error(t, <-contenders, "contender accepted before original root commit")
	}
	require.Equal(t, priorRuns+1, runs.Load(), "reservation losers executed business")
	close(concurrentRelease)
	require.NoError(t, <-concurrentFinished)
	waitMessage(concurrentMeta)
	for id := uint64(202); id < 209; id++ {
		candidate, _ := m3AnswerSheet(t, id, "concurrent-input")
		result, existed, err := durable.CreateDurably(t.Context(), candidate, concurrentMeta)
		require.NoError(t, err)
		require.True(t, existed)
		require.Equal(t, concurrentSheet.ID(), result.ID())
	}
	require.Equal(t, priorRuns+1, runs.Load())
	cases["concurrent_reservation_one_execution"] = true
	for _, name := range []string{"rm_outbox", "domain_event_outbox"} {
		count, err := db.Collection(name).CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		require.Zero(t, count, "new transaction depended on Outbox")
	}
	cases["no_new_publish_outbox"] = true
	store.Runtime().StopAdmission()
	closedSheet, closedMeta := m3AnswerSheet(t, 107, "closed")
	priorRuns = runs.Load()
	_, _, err = durable.CreateDurably(t.Context(), closedSheet, closedMeta)
	require.Error(t, err)
	require.Equal(t, priorRuns, runs.Load())
	cases["closed_admission_no_business"] = true
	closeCtx, cancelClose := context.WithTimeout(t.Context(), 20*time.Second)
	require.NoError(t, store.Runtime().Close(closeCtx))
	cancelClose()
	// A new host client/runtime must obtain the same accepted result from a
	// fresh authority boundary without executing business again. Actual process
	// SIGKILL and packet-loss recovery remain separately named T22/T23 gates.
	newClient, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri.String()))
	require.NoError(t, err)
	newDB := newClient.Database(db.Name())
	newRepo := answerrepo.NewRepositoryWithoutSchema(newDB)
	var newExecutions atomic.Int32
	newRunner := apptransaction.RunnerFunc(func(context.Context, func(context.Context) error) error {
		newExecutions.Add(1)
		return errors.New("duplicate must not execute in a new instance")
	})
	newStore, err := answerrepo.NewRocketMQSubmissionStore(newRepo, newRunner, &producerOptions, catalog)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, newStore.Runtime().Close(ctx))
		require.NoError(t, newClient.Disconnect(ctx))
	})
	require.NoError(t, newStore.Runtime().Start(t.Context()))
	newCandidate, _ := m3AnswerSheet(t, 209, "accepted")
	result, existed, err = newStore.CreateDurably(t.Context(), newCandidate, metaInfo)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, sheet.ID(), result.ID())
	require.Zero(t, newExecutions.Load())
	cases["new_instance_exact_accepted_result"] = true
	require.Len(t, cases, 16)
	raw, err := json.Marshal(map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases,
		"root_ack_discard_is_client_level": true, "mongo_callback_failpoint": true, "original_transaction_slots": 2,
		"upstream_rpc": "actual_protobuf_service_over_bufconn", "system_login_not_claimed": true})
	require.NoError(t, err)
	fmt.Println("M3_QS_ANSWER_RECEIPT " + string(raw))
}
