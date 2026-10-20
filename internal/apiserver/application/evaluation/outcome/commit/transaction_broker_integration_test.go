//go:build integration && m3_evaluation_broker

package commit

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
	"testing"
	"time"

	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluationsubmit"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type rootBarrierProjector struct {
	entered  chan struct{}
	release  chan struct{}
	delegate outcomescoring.Projector
}

func (p rootBarrierProjector) Project(ctx context.Context, r *domainoutcome.Record, a *assessment.Assessment, e *domainoutcome.Execution) error {
	close(p.entered)
	select {
	case <-p.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return p.delegate.Project(ctx, r, a, e)
}

func TestM3EvaluationBrokerHost(t *testing.T) {
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
	name := fmt.Sprintf("qs_m3_evaluation_broker_%d", time.Now().UnixNano())
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
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", "mysql", "000106_transaction_messaging.up.sql"))
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), string(schema))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&repository.AssessmentPO{}, &repository.EvaluationOutcomePO{}, &repository.AssessmentScorePO{}, &checkpoint.RuntimeCheckpointPO{}))
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
	producerOptions, observerOptions := load("QS_M3_PRODUCER_CONFIG"), load("QS_M3_OBSERVER_CONFIG")
	b := producerOptions.RocketMQ.Transactions["qs.mysql"]
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: b.StoreID, WriterEpoch: b.WriterEpoch}
	_, err = pool.ExecContext(t.Context(), `INSERT INTO rm_tx_binding VALUES(?,?,?,1)`, []byte(binding.Owner), []byte(binding.StoreID), []byte(binding.WriterEpoch))
	require.NoError(t, err)
	authority, err := txmysql.New(pool, binding)
	require.NoError(t, err)
	source, err := catalog.Load("../../../../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	topic, ok := routes.GetTopicForEvent(assessment.EventTypeRequested)
	require.True(t, ok)
	var before int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&before))
	owner, err := boundary.New(db, &producerOptions, routes, []string{topic})
	require.NoError(t, err)
	repo := repository.NewAssessmentRepository(db)
	submitter, err := evaluationsubmit.New(db, owner, repo.(assessment.PendingSubmissionRepository))
	require.NoError(t, err)
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
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-evaluation-observer", func(_ context.Context, d transport.Delivery) error {
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
			require.Equal(t, e.Plan.Input().Branches[0].Wire, m.Payload)
		case <-time.After(15 * time.Second):
			t.Fatal("committed original wire not received")
		}
	}
	runRepo := checkpoint.NewRunRepository(db)
	outcomes := repository.NewOutcomeRepository(db)
	seed := func(id uint64, pending bool) (*assessment.Assessment, *domainoutcome.Execution) {
		a, result := commitTestOutcome(t)
		assessment.WithID(meta.FromUint64(id))(a)
		po := repository.NewAssessmentMapper().ToPO(a)
		if pending {
			po.Status = assessment.StatusPending.String()
			po.SubmittedAt = nil
		}
		require.NoError(t, db.Create(po).Error)
		a, err = repo.FindByID(t.Context(), po.ID)
		require.NoError(t, err)
		return a, result
	}
	cases := map[string]bool{"constructor_no_ddl": true}
	pending, _ := seed(5000, true)
	submitted, err := submitter.Submit(t.Context(), pending, time.Now())
	require.NoError(t, err)
	require.True(t, pending.Status().IsPending())
	require.True(t, submitted.Status().IsSubmitted())
	submission, err := owner.Find(t.Context(), "org:1", "evaluation-submit/5000")
	require.NoError(t, err)
	require.Equal(t, transaction.Commit, (transaction.StoreChecker{Store: authority}).Check(t.Context(), submission.Execution.References()[0]))
	waitWire(submission.Execution)
	cases["request_cas_wire_and_witness"] = true
	repeated, err := submitter.Submit(t.Context(), pending, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, *submitted.SubmittedAt(), *repeated.SubmittedAt())
	cases["request_duplicate_first_wire"] = true
	makeRequest := func(id uint64) (CommitRequest, *assessment.Assessment) {
		a, result := seed(id, false)
		claim, err := runRepo.Claim(t.Context(), evaluationrun.ClaimRequest{AssessmentID: id, Token: fmt.Sprintf("original-%d", id), ClaimedAt: time.Now(), LeaseUntil: time.Now().Add(time.Minute)})
		require.NoError(t, err)
		require.True(t, claim.Claimed)
		run := claim.Run
		require.NoError(t, run.AttachInputSnapshot("isn:v2:"+strings.Repeat("a", 64)))
		require.NoError(t, runRepo.SaveClaimed(t.Context(), run))
		return CommitRequest{Assessment: a, Execution: result, Input: commitTestInput(), Run: &run, EvaluatedAt: time.Now(), DescriptorKey: evalpipeline.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange}, OutcomePolicy: evalpipeline.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange)}, a
	}
	projector := outcomescoring.NewAssessmentScoreProjector(repository.NewScoreRepository(db))
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseRoot := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRoot)
	committer, err := NewTransactionalCommitter(owner, repo, outcomes, runRepo, rootBarrierProjector{entered, release, projector})
	require.NoError(t, err)
	q, a := makeRequest(5001)
	stale := *a
	done := make(chan error, 1)
	go func() { _, err := committer.Commit(t.Context(), q); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("root did not reach guarded projector: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("root barrier timeout")
	}
	select {
	case <-received:
		t.Fatal("half message became consumable before business root")
	case <-time.After(350 * time.Millisecond):
	}
	releaseRoot()
	require.NoError(t, <-done)
	execution, err := owner.Find(t.Context(), "org:1", "evaluation-outcome/"+q.Run.ID().String())
	require.NoError(t, err)
	waitWire(execution.Execution)
	record, err := outcomes.FindByAssessmentID(t.Context(), a.ID())
	require.NoError(t, err)
	require.NotNil(t, record)
	require.NotNil(t, record.CommittedTransactionEvidence())
	actual, err := repo.FindByID(t.Context(), a.ID())
	require.NoError(t, err)
	require.True(t, actual.Status().IsEvaluated())
	latest, err := runRepo.FindLatestByAssessmentID(t.Context(), a.ID().Uint64())
	require.NoError(t, err)
	require.Equal(t, evalrun.StatusSucceeded, latest.Attempt().Status)
	require.Equal(t, transaction.Commit, (transaction.StoreChecker{Store: authority}).Check(t.Context(), execution.Execution.References()[0]))
	cases["half_hidden_until_original_outcome_root"] = true
	require.NoError(t, owner.Runtime().SetAdmission(false))
	closed, _ := makeRequest(5002)
	simple, err := NewTransactionalCommitter(owner, repo, outcomes, runRepo, projector)
	require.NoError(t, err)
	_, err = simple.Commit(t.Context(), closed)
	require.ErrorIs(t, err, port.ErrUnresolved)
	require.True(t, closed.Assessment.Status().IsSubmitted())
	require.Equal(t, evalrun.StatusRunning, closed.Run.Attempt().Status)
	absent, err := outcomes.FindByAssessmentID(t.Context(), closed.Assessment.ID())
	require.NoError(t, err)
	require.Nil(t, absent)
	require.NoError(t, owner.Runtime().SetAdmission(true))
	cases["closed_admission_no_business"] = true
	handled, err := simple.(PendingRecoverer).RecoverPending(t.Context(), &stale)
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, stale.Status().IsEvaluated())
	cases["exact_frozen_result_recovery"] = true
	receipt := map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "broker": "actual_5.5.1", "root": "original_mysql_gorm", "source_input": "deterministic_fixture_not_system_account", "process_exit_not_claimed": true}
	raw, err := json.Marshal(receipt)
	require.NoError(t, err)
	fmt.Println("M3_QS_EVALUATION_RECEIPT " + string(raw))
}
