//go:build integration && m3_evaluation_broker

package transaction_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	dispatchapp "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	schedulemodel "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	schedulestore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	evaluationworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	evaluationmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	legacyStager "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type failureLegacyPost struct{ calls atomic.Int32 }

func (p *failureLegacyPost) AfterCommit(context.Context, []event.DomainEvent, time.Time) {
	p.calls.Add(1)
}

type failureInputFixture struct{ calls atomic.Int32 }
type failureInputError struct{}

func (failureInputError) Error() string   { return "fixture dependency unavailable" }
func (failureInputError) Retryable() bool { return true }
func (f *failureInputFixture) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	f.calls.Add(1)
	return nil, failureInputError{}
}

func TestM3FailureBrokerHost(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_EVALUATION_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net)
	require.Empty(t, cfg.DBName)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	cfg.ParseTime, cfg.MultiStatements = true, true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	name := fmt.Sprintf("qs_m3_failure_broker_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
		require.NoError(t, err)
	})
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mysql", "000106_transaction_messaging.up.sql"))
	require.NoError(t, err)
	nativeSchema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mysql", "000110_native_transaction_limit_notices.up.sql"))
	require.NoError(t, err)
	schema = append(append(schema, '\n'), nativeSchema...)
	_, err = pool.ExecContext(t.Context(), string(schema))
	require.NoError(t, err)
	retrySchema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mysql", "000107_business_retry_schedule.up.sql"))
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), string(retrySchema))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&repository.AssessmentPO{}, &repository.EvaluationOutcomePO{}, &repository.AssessmentScorePO{}, &checkpoint.RuntimeCheckpointPO{}))
	legacySchema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mysql", "000084_standard_reliable_outbox.up.sql"))
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), string(legacySchema))
	require.NoError(t, err)

	load := func(key string) qsoptions.MessagingOptions {
		raw, err := os.ReadFile(os.Getenv(key))
		require.NoError(t, err)
		var o qsoptions.MessagingOptions
		require.NoError(t, json.Unmarshal(raw, &o))
		require.NotNil(t, o.RocketMQ)
		host, _, err := net.SplitHostPort(o.RocketMQ.Endpoint)
		require.NoError(t, err)
		require.True(t, net.ParseIP(host).IsLoopback())
		return o
	}
	producerOptions, observerOptions := load("QS_M3_FAILURE_PRODUCER_CONFIG"), load("QS_M3_FAILURE_OBSERVER_CONFIG")
	b := producerOptions.RocketMQ.Transactions["qs.mysql"]
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: b.StoreID, WriterEpoch: b.WriterEpoch}
	_, err = pool.ExecContext(t.Context(), `INSERT INTO rm_tx_binding VALUES(?,?,?,1)`, []byte(binding.Owner), []byte(binding.StoreID), []byte(binding.WriterEpoch))
	require.NoError(t, err)
	authority, err := txmysql.New(pool, binding)
	require.NoError(t, err)
	source, err := catalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	topic, ok := routes.GetTopicForEvent(assessment.EventTypeRequested)
	require.True(t, ok)
	var before int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&before))
	owner, err := boundary.New(db, &producerOptions, routes, []string{topic})
	require.NoError(t, err)
	repo := repository.NewAssessmentRepository(db)
	var after int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&after))
	require.Equal(t, before, after)
	require.False(t, owner.Runtime().Stats().Ready)
	require.NoError(t, owner.Runtime().Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, owner.Runtime().Close(ctx))
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
	retryReceived := make(chan transport.Received, 16)
	var mu sync.Mutex
	seen := map[string]bool{}
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-failure-observer", func(_ context.Context, d transport.Delivery) error {
		m := d.Message()
		envelope, err := domainwire.DecodeEnvelope(m.Payload)
		if err != nil {
			return err
		}
		if envelope.EventType == assessment.EventTypeRetryRequested && envelope.AggregateID == "5003" {
			if err := d.Ack(); err != nil {
				return err
			}
			select {
			case retryReceived <- m:
				return nil
			default:
				return errors.New("retry observer queue full")
			}
		}
		// Independent fanout subscriptions also receive earlier fixture facts.
		// Apply the failure handler's event/aggregate filter before checking its wire.
		if envelope.EventType != assessment.EventTypeFailed || envelope.AggregateID != "5003" {
			return d.Ack()
		}
		if err := d.Ack(); err != nil {
			return err
		}
		mu.Lock()
		duplicate := seen[m.ID]
		seen[m.ID] = true
		mu.Unlock()
		if !duplicate {
			select {
			case received <- m:
			default:
				return errors.New("fixture queue full")
			}
		}
		return nil
	}, func(context.Context, transport.Failure) error { return errors.New("fixture delivery failed") }))
	waitWire := func(e transaction.Execution) {
		select {
		case m := <-received:
			require.Equal(t, e.References()[0].MessageID, m.ID)
			require.Equal(t, e.Plan.Input().Branches[0].Wire, m.Wire)
			envelope, recognized, err := legacy.Decode(m.Wire)
			require.NoError(t, err)
			require.True(t, recognized)
			require.Equal(t, envelope.Payload, m.Payload, "normalized payload must retain the original domain envelope")
		case <-time.After(15 * time.Second):
			t.Fatal("committed original wire not received")
		}
	}
	runRepo := checkpoint.NewRunRepository(db)
	// Exercise the actual engine's input-failure finalizer. No scoring/provider
	// call is made by this dependency fixture; future wake responsibility stays
	// in the original scheduled row until the separate T13 handoff.
	failing, _ := m3EvaluationFixture(t)
	failing.ClearEvents()
	assessment.WithID(meta.FromUint64(5003))(failing)
	require.NoError(t, db.Create(repository.NewAssessmentMapper().ToPO(failing)).Error)
	failing, err = repo.FindByID(t.Context(), failing.ID())
	require.NoError(t, err)
	cases := map[string]bool{}
	dependency := &failureInputFixture{}
	oldStager, err := legacyStager.NewStager(routes, "fixture")
	require.NoError(t, err)
	oldPost := &failureLegacyPost{}
	module, err := evaluationmod.New(evaluationmod.Deps{MySQLDB: db, InputResolver: dependency, ExecutionPaths: []modelcatalog.ExecutionPath{modelcatalog.ExecutionPathScaleDescriptor}, OutboxProfile: appEventing.ProfileBinding{Stager: oldStager, PostCommit: oldPost}, EventTransactionOwner: owner, TransactionRoutes: routes})
	require.NoError(t, err)
	receipt, err := module.WorkerService.Execute(t.Context(), evaluationworker.Command{AssessmentID: failing.ID().Uint64()})
	require.NoError(t, err)
	require.Equal(t, "failed", receipt.Status)
	require.True(t, receipt.Retryable)
	failedRun, err := runRepo.FindLatestByAssessmentID(t.Context(), failing.ID().Uint64())
	require.NoError(t, err)
	require.NotNil(t, failedRun)
	require.Equal(t, evalrun.StatusFailed, failedRun.Attempt().Status)
	failedFact, err := repo.FindByID(t.Context(), failing.ID())
	require.NoError(t, err)
	require.True(t, failedFact.Status().IsFailed())
	failureExecution, err := owner.Find(t.Context(), "org:1", "evaluation-failure/"+failedRun.ID().String())
	require.NoError(t, err)
	require.Equal(t, transaction.Commit, (transaction.StoreChecker{Store: authority}).Check(t.Context(), failureExecution.Execution.References()[0]))
	waitWire(failureExecution.Execution)
	cases["failure_original_root_and_witness"] = true
	decision := failedRun.RetryDecision()
	require.NotNil(t, decision)
	require.NotNil(t, decision.NextAttemptAt)
	require.NotEmpty(t, decision.RetryEventID)
	var frozenRaw []byte
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT frozen_plan FROM qs_retry_schedule WHERE message_id=?", []byte(decision.RetryEventID)).Scan(&frozenRaw))
	frozenSchedule, err := schedulemodel.Decode(frozenRaw)
	require.NoError(t, err)
	futureID, futureWire, due := frozenSchedule.Message().Input().ID, frozenSchedule.Message().Input().Payload, frozenSchedule.DueAt()
	require.Equal(t, decision.RetryEventID, futureID)
	require.True(t, decision.NextAttemptAt.Equal(due.Round(time.Microsecond).Round(time.Millisecond)))
	require.NoError(t, frozenSchedule.MatchDecision(decision, *decision.NextAttemptAt))
	var futureOutbox int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_outbox WHERE message_id=?", futureID).Scan(&futureOutbox))
	require.Zero(t, futureOutbox)
	outer, recognized, err := legacy.Decode(futureWire)
	require.NoError(t, err)
	require.True(t, recognized)
	require.Equal(t, futureID, outer.UUID)
	futureEnvelope, err := domainwire.DecodeEnvelope(outer.Payload)
	require.NoError(t, err)
	var wake assessment.EvaluationRequestedData
	require.NoError(t, json.Unmarshal(futureEnvelope.Data, &wake))
	require.Equal(t, failedRun.Attempt().Number, wake.ExpectedAttempt)
	require.Equal(t, due, wake.RequestedAt)
	var immediateRows int
	require.NoError(t, pool.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM rm_outbox WHERE event_type IN (?,?,?)`, assessment.EventTypeRequested, assessment.EventTypeOutcomeCommitted, assessment.EventTypeFailed).Scan(&immediateRows))
	require.Zero(t, immediateRows)
	cases["future_retry_retains_due_authorization_and_wire"] = true
	repeated, err := module.WorkerService.Execute(t.Context(), evaluationworker.Command{AssessmentID: failing.ID().Uint64()})
	require.NoError(t, err)
	require.Equal(t, receipt.RunID, repeated.RunID)
	require.Zero(t, oldPost.calls.Load(), "transaction facts used old immediate Outbox wake")
	require.Equal(t, int32(1), dependency.calls.Load())
	sameAttempt, err := runRepo.FindLatestByAssessmentID(t.Context(), failing.ID().Uint64())
	require.NoError(t, err)
	require.Equal(t, failedRun.ID(), sameAttempt.ID())
	require.Equal(t, decision, sameAttempt.RetryDecision())
	historical, err := checkpoint.NewExactRunReader(db).FindRunByID(t.Context(), failedRun.ID())
	require.NoError(t, err)
	require.Equal(t, failedRun.ID(), historical.ID())
	cases["failure_redelivery_no_extra_attempt_or_input_call"] = true

	// The observer ACKs only; it never starts the next business attempt.
	// Wait for the real deadline instead of advancing a fixture clock.
	schedules, err := schedulestore.NewWithPlanReader(db, binding, owner.Artifact)
	require.NoError(t, err)
	dispatcher, err := dispatchapp.New(schedules, owner, checkpoint.NewRepository(db))
	require.NoError(t, err)
	beforeDue := time.Now().UTC()
	require.True(t, beforeDue.Before(due))
	dispatched, err := dispatcher.Once(t.Context(), beforeDue, 10)
	require.NoError(t, err)
	require.Empty(t, dispatched)
	var early int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_request WHERE idempotency_key=?", []byte(schedulemodel.DispatchKey(frozenSchedule))).Scan(&early))
	require.Zero(t, early)
	timer := time.NewTimer(time.Until(due.Add(2 * time.Millisecond)))
	select {
	case <-timer.C:
	case <-t.Context().Done():
		timer.Stop()
		t.Fatal("deadline wait cancelled")
	}
	dispatched = []dispatchapp.Outcome{recoverAbortedDueDispatch(t, schedules, owner, checkpoint.NewRepository(db), authority, frozenSchedule)}
	require.NoError(t, dispatched[0].Err)
	require.Equal(t, transaction.LocalCommitted, dispatched[0].Transaction.Local)
	require.Equal(t, schedulemodel.DispatchCommitted, dispatched[0].State)
	select {
	case received := <-retryReceived:
		require.Equal(t, futureID, received.ID)
		require.Equal(t, futureWire, received.Wire)
	case <-time.After(45 * time.Second):
		t.Fatal("committed due retry missing")
	}
	storedSchedule, err := schedules.Find(t.Context(), frozenSchedule)
	require.NoError(t, err)
	require.NoError(t, storedSchedule.Validate())
	require.NotNil(t, storedSchedule.Proof)
	ref, err := transaction.DecodeReference(storedSchedule.Proof.TransactionReference)
	require.NoError(t, err)
	require.Equal(t, transaction.Commit, (transaction.StoreChecker{Store: authority}).Check(t.Context(), ref))
	repeatedDispatch, err := dispatcher.Once(t.Context(), time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Empty(t, repeatedDispatch)
	afterDispatch, err := runRepo.FindLatestByAssessmentID(t.Context(), failing.ID().Uint64())
	require.NoError(t, err)
	require.Equal(t, failedRun.ID(), afterDispatch.ID())
	require.Equal(t, failedRun.Attempt().Number, afterDispatch.Attempt().Number)
	require.Equal(t, int32(1), dependency.calls.Load())
	dispatchReceipt, err := json.Marshal(map[string]any{"owner": "qs.mysql", "passed": 9, "failed": 0, "skipped": 0, "cases": map[string]bool{"no_half_before_real_due": true, "first_wire_and_dispatch_witness": true, "no_business_attempt_or_input_repeat": true, "repeat_dispatch_is_empty": true, "automatic_pause_before_reservation": true, "real_root_abort_witness": true, "scan_does_not_retry_aborted": true, "explicit_retry_same_plan_new_fence": true, "old_aborted_evidence_retained": true}, "system_login_not_claimed": true, "business_consumer_not_claimed": true})
	require.NoError(t, err)
	t.Log("M3_QS_RETRY_DISPATCH_RECEIPT " + string(dispatchReceipt))

	proofReceipt := map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "root": "original_mysql_gorm", "source_input": "controlled_input_failure_no_provider", "process_exit_not_claimed": true}
	raw, err := json.Marshal(proofReceipt)
	require.NoError(t, err)
	fmt.Println("M3_QS_FAILURE_RECEIPT " + string(raw))
}
