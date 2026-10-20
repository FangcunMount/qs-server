//go:build integration && m3_evaluation_broker

package transaction_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluationfailure"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
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
	_, err = pool.ExecContext(t.Context(), string(schema))
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
	var mu sync.Mutex
	seen := map[string]bool{}
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-failure-observer", func(_ context.Context, d transport.Delivery) error {
		m := d.Message()
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
	failureWriter, err := evaluationfailure.New(db, owner, routes, repo, runRepo)
	require.NoError(t, err)
	dependency := &failureInputFixture{}
	engine := execute.NewEngine(repo, dependency, execute.WithRunRepository(runRepo), execute.WithFailureTransactionCommitter(failureWriter))
	require.Error(t, engine.Evaluate(t.Context(), failing.ID().Uint64()))
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
	var futureID string
	var futureWire []byte
	var due time.Time
	var attempts, failures int
	require.NoError(t, pool.QueryRowContext(t.Context(), `SELECT message_id,payload,next_attempt_at,attempt_count,failure_count FROM rm_outbox WHERE message_id=?`, decision.RetryEventID).Scan(&futureID, &futureWire, &due, &attempts, &failures))
	require.Equal(t, decision.RetryEventID, futureID)
	require.Equal(t, *decision.NextAttemptAt, due)
	require.Zero(t, attempts)
	require.Zero(t, failures)
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
	require.NoError(t, engine.Evaluate(t.Context(), failing.ID().Uint64()))
	require.Equal(t, int32(1), dependency.calls.Load())
	sameAttempt, err := runRepo.FindLatestByAssessmentID(t.Context(), failing.ID().Uint64())
	require.NoError(t, err)
	require.Equal(t, failedRun.ID(), sameAttempt.ID())
	require.Equal(t, decision, sameAttempt.RetryDecision())
	historical, err := checkpoint.NewExactRunReader(db).FindRunByID(t.Context(), failedRun.ID())
	require.NoError(t, err)
	require.Equal(t, failedRun.ID(), historical.ID())
	cases["failure_redelivery_no_extra_attempt_or_input_call"] = true

	receipt := map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "root": "original_mysql_gorm", "source_input": "controlled_input_failure_no_provider", "process_exit_not_claimed": true}
	raw, err := json.Marshal(receipt)
	require.NoError(t, err)
	fmt.Println("M3_QS_FAILURE_RECEIPT " + string(raw))
}
