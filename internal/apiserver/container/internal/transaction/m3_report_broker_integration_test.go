//go:build integration && m3_report_integration

package transaction_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	dispatchapp "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	schedulemodel "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	schedulestore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	audit "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/mongoconsistency"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
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

type m3ReportAuditFacts map[meta.ID]*evaluationfact.Record

func (f m3ReportAuditFacts) FindByID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	r := f[id]
	if r == nil {
		return nil, evaluationfact.ErrNotFound
	}
	return r, nil
}
func (f m3ReportAuditFacts) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return nil, evaluationfact.ErrNotFound
}

func TestM3ReportTerminalBrokerHost(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	uri, err := url.Parse(os.Getenv("QS_M3_MONGO_URI"))
	require.NoError(t, err)
	host, _, err := net.SplitHostPort(uri.Host)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	require.Equal(t, "sdk-proof", uri.Query().Get("replicaSet"))
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri.String()).SetAppName("qs-m3-report-fixture"))
	require.NoError(t, err)
	db := client.Database(fmt.Sprintf("qs_m3_reports_%d", time.Now().UnixNano()))
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
	producer, observer := load("QS_M3_PRODUCER_CONFIG"), load("QS_M3_OBSERVER_CONFIG")
	config, err := catalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(config)
	topic, found := routes.GetTopicForEvent(catalog.InterpretationReportGenerated)
	require.True(t, found)
	failedTopic, found := routes.GetTopicForEvent(catalog.InterpretationReportFailed)
	require.True(t, found)
	require.Equal(t, topic, failedTopic)
	owner, err := guarded.New(db, &producer, []string{topic})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, owner.Runtime().Close(ctx))
	})
	limiter := backpressure.NewLimiter(2, time.Second)
	root := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{Boundary: "interpretation_commit", Limiter: limiter})
	var roots, callbacks atomic.Int32
	type hookKey struct{}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		roots.Add(1)
		hook, _ := ctx.Value(hookKey{}).(func(string) error)
		if hook != nil {
			if err := hook("before_root"); err != nil {
				return err
			}
		}
		err := root.WithinTransaction(ctx, func(local context.Context) error {
			callbacks.Add(1)
			if err := fn(local); err != nil {
				return err
			}
			if hook != nil {
				return hook("before_commit")
			}
			return nil
		})
		if err == nil && hook != nil {
			return hook("after_commit")
		}
		return err
	})
	committer, err := repository.NewTransactionCommitter(db, owner, runner, routes)
	require.NoError(t, err)
	generations := repository.NewGenerationRepositoryWithoutSchema(db)
	runs := repository.NewRunRepositoryWithoutSchema(db)
	reports := repository.NewReportRepositoryWithoutSchema(db)
	collections, err := db.ListCollectionNames(t.Context(), bson.M{})
	require.NoError(t, err)
	require.Empty(t, collections)
	cases := map[string]bool{"constructor_no_ddl": true}
	facts := m3ReportAuditFacts{}
	require.NoError(t, repository.InstallLifecycleSchema(t.Context(), db))
	require.NoError(t, txmongo.InstallSchema(t.Context(), db))
	require.NoError(t, db.CreateCollection(t.Context(), "rm_outbox"))
	retrySchema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mongodb", "000042_business_retry_schedule.up.json"))
	require.NoError(t, err)
	var retryCommands []json.RawMessage
	require.NoError(t, json.Unmarshal(retrySchema, &retryCommands))
	for _, raw := range retryCommands {
		var command bson.D
		require.NoError(t, bson.UnmarshalExtJSON(raw, true, &command))
		require.NoError(t, db.RunCommand(t.Context(), command).Err())
	}

	bound := producer.RocketMQ.Transactions["qs.mongo"]
	binding := tx.Binding{Owner: "qs.mongo", StoreID: bound.StoreID, WriterEpoch: bound.WriterEpoch}
	_, err = db.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": binding.Owner, "store_id": binding.StoreID, "writer_epoch": binding.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	authority, err := txmongo.New(db, binding)
	require.NoError(t, err)
	require.NoError(t, owner.Runtime().Start(t.Context()))
	consumerConfig, err := observer.RocketMQ.Load([]string{topic}, 2, 5*time.Second)
	require.NoError(t, err)
	consumer, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: consumerConfig, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, consumer.Close(ctx))
	})
	received := make(chan transport.Received, 64)
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-report-observer", func(_ context.Context, d transport.Delivery) error {
		if err := d.Ack(); err != nil {
			return err
		}
		select {
		case received <- d.Message():
			return nil
		default:
			return errors.New("report receipt queue full")
		}
	}, func(context.Context, transport.Failure) error { return errors.New("report observer failed") }))
	lookup := func(q app.CommitSuccessRequest) tx.Reservation {
		reservation, err := owner.Find(t.Context(), fmt.Sprintf("org:%d", q.InterpretReport.Association().OrgID), "report-terminal/"+q.Run.ID().String())
		require.NoError(t, err)
		return reservation
	}
	waitMessage := func(q app.CommitSuccessRequest) {
		r := lookup(q)
		branch := r.Execution.Plan.Input().Branches[0]
		timeout := time.NewTimer(45 * time.Second)
		defer timeout.Stop()
		for {
			select {
			case m := <-received:
				if m.ID != branch.Message.Input().ID {
					continue
				}
				require.True(t, bytes.Equal(branch.Wire, m.Wire), "first report wire changed")
				fact, err := authority.Evidence(t.Context(), r.Execution.References()[0])
				require.NoError(t, err)
				require.Equal(t, tx.Commit, tx.Resolve(r.Execution.References()[0], fact))
				return
			case <-timeout.C:
				t.Fatal("committed report event missing")
				return
			}
		}
	}
	request := func(id uint64) app.CommitSuccessRequest {
		at := time.Now().UTC()
		g, err := generation.New(meta.FromUint64(id), generation.Key{OutcomeID: meta.FromUint64(id + 9), ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
		require.NoError(t, err)
		r, err := run.NewPending(meta.FromUint64(id+1), g.ID(), 1)
		require.NoError(t, err)
		require.NoError(t, r.StartWithLease(at.Add(-time.Second), "original-worker", at.Add(time.Minute)))
		require.NoError(t, g.Begin(r.ID(), *r.StartedAt()))
		require.NoError(t, root.WithinTransaction(t.Context(), func(local context.Context) error {
			if err := generations.Create(local, g); err != nil {
				return err
			}
			return runs.Create(local, r)
		}))
		a, err := report.NewInterpretReport(report.InterpretReportInput{ID: meta.FromUint64(id + 2), GenerationID: g.ID(), OutcomeID: g.Key().OutcomeID, InterpretationRunID: r.ID(), Association: report.Association{OrgID: 7, AssessmentID: meta.FromUint64(id + 8), TesteeID: 9}, ReportType: g.Key().ReportType, TemplateVersion: g.Key().TemplateVersion,
			BuilderIdentity: report.BuilderIdentityFactorScoring, ContentSchemaVersion: report.ContentSchemaVersionV1, GeneratedAt: at, Content: report.Content{Model: report.ModelIdentity{Kind: "scale", Code: "SDS", Version: "v1", Title: "fixture"}, PrimaryScore: report.NewRawTotalScore(42, nil), Level: report.LevelFromRisk(report.RiskLevelLow), Conclusion: "first report", Dimensions: []report.DimensionInterpret{report.NewDimensionInterpret(report.NewFactorCode("TOTAL"), "total", 42, nil, report.RiskLevelLow, "fixture", "fixture")}}})
		require.NoError(t, err)
		facts[g.Key().OutcomeID] = evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: g.Key().OutcomeID, OrgID: a.Association().OrgID, AssessmentID: a.Association().AssessmentID, TesteeID: a.Association().TesteeID})
		return app.CommitSuccessRequest{Generation: g, Run: r, InterpretReport: a, BuilderIdentity: a.BuilderIdentity(), ContentSchemaVersion: a.ContentSchemaVersion(), CompletedAt: at}
	}
	assertNotCommitted := func(q app.CommitSuccessRequest) {
		g, err := generations.FindByID(t.Context(), q.Generation.ID())
		require.NoError(t, err)
		require.Equal(t, generation.StatusGenerating, g.Status())
		_, err = reports.FindByID(t.Context(), q.InterpretReport.ID())
		require.Error(t, err)
		r := lookup(q)
		fact, err := authority.Evidence(t.Context(), r.Execution.References()[0])
		require.NoError(t, err)
		require.NotEqual(t, tx.Commit, tx.Resolve(r.Execution.References()[0], fact))
	}
	first := request(1001)
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
	go func() { _, err := committer.CommitSuccess(ctx, first); finished <- err }()
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("prepared halves did not reach original runner")
	}
	require.Zero(t, callbacks.Load())
	assertNotCommitted(first)
	select {
	case <-received:
		t.Fatal("half report visible before root commit")
	case <-time.After(2 * time.Second):
	}
	cases["half_invisible"] = true
	close(release)
	require.NoError(t, <-finished)
	waitMessage(first)
	g, err := generations.FindByID(t.Context(), first.Generation.ID())
	require.NoError(t, err)
	require.NoError(t, g.GeneratedTransactionEvidence().Validate())
	r, err := runs.FindByID(t.Context(), first.Run.ID())
	require.NoError(t, err)
	require.Equal(t, run.StatusSucceeded, r.Status())
	cases["original_success_witness_wire"] = true
	prior := roots.Load()
	again, err := committer.CommitSuccess(t.Context(), first)
	require.NoError(t, err)
	require.Equal(t, first.InterpretReport.ID(), again.InterpretReport.ID())
	require.Equal(t, prior, roots.Load())
	cases["duplicate_first_artifact"] = true
	failure := request(2001)
	f, err := committer.CommitFailure(t.Context(), app.CommitFailureRequest{Generation: failure.Generation, Run: failure.Run, OutcomeID: failure.Generation.Key().OutcomeID, Association: failure.InterpretReport.Association(), Failure: run.Failure{Kind: run.FailureKindBuild, Code: "build_failed", SafeMessage: "fixture", Retryable: true}, FailedAt: failure.CompletedAt})
	require.NoError(t, err)
	require.Equal(t, run.StatusFailed, f.Run.Status())
	require.NoError(t, f.Run.FailedTransactionEvidence().Validate())
	waitMessage(failure)
	cases["original_failure_witness"] = true
	var futureRecord struct {
		Raw []byte `bson:"frozen_plan"`
	}
	err = db.Collection("qs_retry_schedule").FindOne(t.Context(), bson.M{"message_id": f.Run.RetryDecision().RetryEventID}).Decode(&futureRecord)
	require.NoError(t, err)
	frozenSchedule, err := schedulemodel.Decode(futureRecord.Raw)
	require.NoError(t, err)
	require.True(t, f.Run.RetryDecision().NextAttemptAt.Equal(frozenSchedule.DueAt().Truncate(time.Millisecond)))
	require.NoError(t, frozenSchedule.MatchDecision(f.Run.RetryDecision(), *f.Run.RetryDecision().NextAttemptAt))
	futureOutbox, err := db.Collection("rm_outbox").CountDocuments(t.Context(), bson.M{"message_id": f.Run.RetryDecision().RetryEventID})
	require.NoError(t, err)
	require.Zero(t, futureOutbox)
	require.Equal(t, 1, f.Run.Attempt())
	cases["future_retry_preserved"] = true
	schedules, err := schedulestore.New(db, binding)
	require.NoError(t, err)
	retryAuthority, err := repository.NewRetryDispatchAuthority(db)
	require.NoError(t, err)
	dispatchBoundary, err := guarded.NewCommitter(db, owner, runner, routes)
	require.NoError(t, err)
	dispatcher, err := dispatchapp.New(schedules, dispatchBoundary, retryAuthority)
	require.NoError(t, err)
	realBefore := time.Now().UTC()
	require.True(t, realBefore.Before(frozenSchedule.DueAt()))
	earlyDispatch, err := dispatcher.Once(t.Context(), realBefore, 10)
	require.NoError(t, err)
	require.Empty(t, earlyDispatch)
	early, err := db.Collection("rm_tx_request").CountDocuments(t.Context(), bson.M{"idempotency_key": schedulemodel.DispatchKey(frozenSchedule)})
	require.NoError(t, err)
	require.Zero(t, early)
	timer := time.NewTimer(time.Until(frozenSchedule.DueAt().Add(2 * time.Millisecond)))
	select {
	case <-timer.C:
	case <-t.Context().Done():
		timer.Stop()
		t.Fatal("deadline wait cancelled")
	}
	dueDispatch := []dispatchapp.Outcome{recoverAbortedDueDispatch(t, schedules, dispatchBoundary, retryAuthority, frozenSchedule)}
	require.NoError(t, dueDispatch[0].Err)
	require.Equal(t, tx.LocalCommitted, dueDispatch[0].Transaction.Local)
	require.Equal(t, schedulemodel.DispatchCommitted, dueDispatch[0].State)
	retryWait := time.NewTimer(45 * time.Second)
	foundRetry := false
	for !foundRetry {
		select {
		case m := <-received:
			if m.ID == frozenSchedule.Message().Input().ID {
				require.Equal(t, frozenSchedule.Message().Input().Payload, m.Wire)
				foundRetry = true
			}
		case <-retryWait.C:
			t.Fatal("committed report retry missing")
		}
	}
	retryWait.Stop()
	storedSchedule, err := schedules.Find(t.Context(), frozenSchedule)
	require.NoError(t, err)
	require.NoError(t, storedSchedule.Validate())
	ref, err := tx.DecodeReference(storedSchedule.Proof.TransactionReference)
	require.NoError(t, err)
	exact, err := authority.Evidence(t.Context(), ref)
	require.NoError(t, err)
	require.Equal(t, tx.Commit, tx.Resolve(ref, exact))
	repeatedDue, err := dispatcher.Once(t.Context(), time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Empty(t, repeatedDue)
	stillFailed, err := runs.FindByID(t.Context(), f.Run.ID())
	require.NoError(t, err)
	require.Equal(t, 1, stillFailed.Attempt())
	require.Equal(t, run.StatusFailed, stillFailed.Status())
	dispatchReceipt, err := json.Marshal(map[string]any{"owner": "qs.mongo", "passed": 9, "failed": 0, "skipped": 0, "cases": map[string]bool{"no_half_before_real_due": true, "first_wire_and_dispatch_witness": true, "no_business_attempt_or_input_repeat": true, "repeat_dispatch_is_empty": true, "automatic_pause_before_reservation": true, "real_root_abort_witness": true, "scan_does_not_retry_aborted": true, "explicit_retry_same_plan_new_fence": true, "old_aborted_evidence_retained": true}, "system_login_not_claimed": true, "business_consumer_not_claimed": true})
	require.NoError(t, err)
	t.Log("M3_QS_RETRY_DISPATCH_RECEIPT " + string(dispatchReceipt))

	abort := request(3001)
	_, err = committer.CommitSuccess(context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_commit" {
			return errors.New("fixture root abort")
		}
		return nil
	}), abort)
	require.ErrorIs(t, err, port.ErrUnresolved)
	assertNotCommitted(abort)
	cases["root_abort_atomic"] = true
	stale := request(4001)
	_, err = committer.CommitSuccess(context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_root" {
			_, err := db.Collection("interpretation_runs").UpdateOne(t.Context(), bson.M{"domain_id": stale.Run.ID().Uint64()}, bson.M{"$set": bson.M{"trace_id": "next-worker", "recovery_count": 1}})
			return err
		}
		return nil
	}), stale)
	require.ErrorIs(t, err, port.ErrUnresolved)
	assertNotCommitted(stale)
	cases["stale_claim_cannot_commit"] = true
	ack := request(5001)
	_, err = committer.CommitSuccess(context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "after_commit" {
			return errors.New("fixture discards committed root reply")
		}
		return nil
	}), ack)
	require.NoError(t, err)
	waitMessage(ack)
	cases["committed_reply_discard_recovery"] = true
	reentry := request(6001)
	priorCallbacks := callbacks.Load()
	_, err = committer.CommitSuccess(context.WithValue(t.Context(), hookKey{}, func(stage string) error {
		if stage == "before_root" {
			return client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: bson.M{"times": 1}}, {Key: "data", Value: bson.M{"failCommands": bson.A{"update"}, "appName": "qs-m3-report-fixture", "errorCode": 112, "errorLabels": bson.A{"TransientTransactionError"}}}}).Err()
		}
		return nil
	}), reentry)
	require.NoError(t, err)
	require.GreaterOrEqual(t, callbacks.Load()-priorCallbacks, int32(2))
	waitMessage(reentry)
	cases["native_callback_reentry"] = true
	count, err := db.Collection("rm_outbox").CountDocuments(t.Context(), bson.M{"event_type": bson.M{"$in": bson.A{catalog.InterpretationReportGenerated, catalog.InterpretationReportFailed}}})
	require.NoError(t, err)
	require.Zero(t, count)
	cases["no_new_publish_outbox"] = true
	scanner := audit.NewScanner(db, limiter).WithOutcomeFacts(facts).WithTransactionOwner(owner)
	upper, err := scanner.UpperBound(t.Context(), appaudit.PhaseGeneratedTerminal, 2*time.Second)
	require.NoError(t, err)
	batch, err := scanner.ScanBatch(t.Context(), appaudit.BatchRequest{Phase: appaudit.PhaseGeneratedTerminal, UpperBound: upper, Limit: 100, MaxTime: 2 * time.Second})
	require.NoError(t, err)
	require.Empty(t, batch.Findings)
	require.Equal(t, 3, batch.Scanned)
	require.Equal(t, int64(3), batch.EvidenceClasses["transaction_reference"])
	upper, err = scanner.UpperBound(t.Context(), appaudit.PhaseGenerationRun, 2*time.Second)
	require.NoError(t, err)
	batch, err = scanner.ScanBatch(t.Context(), appaudit.BatchRequest{Phase: appaudit.PhaseGenerationRun, UpperBound: upper, Limit: 100, MaxTime: 2 * time.Second})
	require.NoError(t, err)
	require.Empty(t, batch.Findings)
	require.Equal(t, int64(1), batch.EvidenceClasses["transaction_reference"])
	// Persist a deliberately inconsistent business clock in this disposable
	// fixture. Exact committed protocol evidence must not hide graph drift.
	originalFinished := *r.FinishedAt()
	_, err = db.Collection("interpretation_runs").UpdateOne(t.Context(), bson.M{"domain_id": first.Run.ID().Uint64()}, bson.M{"$set": bson.M{"finished_at": originalFinished.Add(time.Millisecond)}})
	require.NoError(t, err)
	generatedUpper, err := scanner.UpperBound(t.Context(), appaudit.PhaseGeneratedTerminal, 2*time.Second)
	require.NoError(t, err)
	corrupt, err := scanner.ScanBatch(t.Context(), appaudit.BatchRequest{Phase: appaudit.PhaseGeneratedTerminal, UpperBound: generatedUpper, Limit: 100, MaxTime: 2 * time.Second})
	require.NoError(t, err)
	require.Len(t, corrupt.Findings, 1)
	require.Equal(t, appaudit.DriftGeneratedTransactionEvidence, corrupt.Findings[0].Kind)
	_, err = db.Collection("interpretation_runs").UpdateOne(t.Context(), bson.M{"domain_id": first.Run.ID().Uint64()}, bson.M{"$set": bson.M{"finished_at": originalFinished}})
	require.NoError(t, err)
	cases["v2_authority_audit"] = true
	closed := request(7001)
	owner.Runtime().StopAdmission()
	prior = roots.Load()
	_, err = committer.CommitSuccess(t.Context(), closed)
	require.Error(t, err)
	require.Equal(t, prior, roots.Load())
	cases["closed_admission_no_business"] = true
	closeCtx, cancelClose := context.WithTimeout(t.Context(), 20*time.Second)
	require.NoError(t, owner.Runtime().Close(closeCtx))
	cancelClose()
	newClient, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri.String()))
	require.NoError(t, err)
	newDB := newClient.Database(db.Name())
	newOwner, err := guarded.New(newDB, &producer, []string{topic})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, newOwner.Runtime().Close(ctx))
		require.NoError(t, newClient.Disconnect(ctx))
	})
	var newExecutions atomic.Int32
	newRunner := apptransaction.RunnerFunc(func(context.Context, func(context.Context) error) error {
		newExecutions.Add(1)
		return errors.New("recovery must not enter root")
	})
	newCommitter, err := repository.NewTransactionCommitter(newDB, newOwner, newRunner, routes)
	require.NoError(t, err)
	require.NoError(t, newOwner.Runtime().Start(t.Context()))
	recovered, handled, err := newCommitter.RecoverPending(t.Context(), first.Generation.Key(), 7)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, first.InterpretReport.ID(), recovered.InterpretReport.ID())
	require.Zero(t, newExecutions.Load())
	cases["new_instance_exact_recovery"] = true
	require.Len(t, cases, 14)
	raw, err := json.Marshal(map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "root_ack_discard_is_client_level": true, "mongo_callback_failpoint": true, "host_entry": "interpretation_original_root_committer", "system_login_not_claimed": true})
	require.NoError(t, err)
	fmt.Println("M3_QS_REPORT_RECEIPT " + string(raw))
}
