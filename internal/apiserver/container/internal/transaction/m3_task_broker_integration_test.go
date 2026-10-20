//go:build integration && m3_task_broker

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
	"testing"
	"time"

	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	appplan "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/taskopen"
	planinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/plan"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/tasktransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestM3TaskBrokerHost(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_TASK_DSN"))
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
	name := fmt.Sprintf("qs_m3_task_broker_%d", time.Now().UnixNano())
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
	require.NoError(t, db.AutoMigrate(&repository.AssessmentTaskPO{}))
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
	source, err := catalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	topic, ok := routes.GetTopicForEvent(domain.EventTypeTaskOpenedReminderRequested)
	require.True(t, ok)
	var before int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&before))
	owner, err := boundary.New(db, &producerOptions, routes, []string{topic})
	require.NoError(t, err)
	repo := repository.NewTaskRepository(db)
	opener, err := taskopen.New(db, owner, routes, repo.(taskopen.SnapshotWriter))
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
	require.NoError(t, consumer.Subscribe(t.Context(), topic, "qs-m3-task-observer", func(_ context.Context, d transport.Delivery) error {
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
	at := time.Now().UTC().Truncate(time.Second).Add(123499501 * time.Nanosecond)
	seed := func(seq int) *domain.AssessmentTask {
		task := domain.NewAssessmentTaskAt(domain.NewAssessmentPlanID(), seq, 1, testee.NewID(3), "TASK-SCALE", at.Add(-time.Minute), at.Add(-time.Hour))
		po := repository.NewTaskMapper().ToPO(task)
		po.EnrollmentID = 700
		require.NoError(t, db.Create(po).Error)
		saved, err := repo.FindByID(t.Context(), task.GetID())
		require.NoError(t, err)
		require.NotNil(t, saved)
		return saved
	}
	lookup := func(task *domain.AssessmentTask) transaction.Execution {
		r, err := owner.Find(t.Context(), "org:1", fmt.Sprintf("task-open/%d/%d", task.GetID().Uint64(), task.GetScheduleRevision()))
		require.NoError(t, err)
		require.Equal(t, [32]byte{}, r.Execution.Token, "read-only lookup must not mint execution capability")
		return r.Execution
	}
	waitWire := func(e transaction.Execution) {
		select {
		case m := <-received:
			require.Equal(t, e.References()[0].MessageID, m.ID)
			require.Equal(t, e.Plan.Input().Branches[0].Wire, m.Wire)
			envelope, err := domainwire.DecodeEnvelope(m.Payload)
			require.NoError(t, err)
			require.Equal(t, domain.EventTypeTaskOpenedReminderRequested, envelope.EventType)
			require.NotContains(t, string(envelope.Data), "entry_url")
			require.NotContains(t, string(envelope.Data), "private-entry")
		case <-time.After(15 * time.Second):
			t.Fatal("original reminder wire not received")
		}
	}
	first := seed(1)
	require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), first, "https://example.test/private-entry-first", at))
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseRoot := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRoot)
	type outcome struct {
		task *domain.AssessmentTask
		err  error
	}
	finished := make(chan outcome, 1)
	go func() {
		task, err := opener.CommitOpened(t.Context(), port.OpenRequest{Candidate: first, Guard: func(ctx context.Context, _ *domain.AssessmentTask) error {
			if _, err := hostmysql.RequireTx(ctx); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}})
		finished <- outcome{task, err}
	}()
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("original root not reached")
	}
	e := lookup(first)
	require.Equal(t, transaction.LocalUnknown, owner.Confirm(t.Context(), e))
	original, err := repo.FindByID(t.Context(), first.GetID())
	require.NoError(t, err)
	require.True(t, original.IsPending())
	select {
	case <-received:
		t.Fatal("half message visible before original root commit")
	case <-time.After(time.Second):
	}
	releaseRoot()
	var committed outcome
	select {
	case committed = <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("original root not completed")
	}
	require.NoError(t, committed.err)
	require.NotNil(t, committed.task)
	require.Equal(t, transaction.LocalCommitted, owner.Confirm(t.Context(), e))
	evidence, err := authority.Evidence(t.Context(), e.References()[0])
	require.NoError(t, err)
	require.NotNil(t, evidence)
	waitWire(e)
	saved, err := repo.FindByID(t.Context(), first.GetID())
	require.NoError(t, err)
	require.Equal(t, first.GetEntryURL(), saved.GetEntryURL())
	require.True(t, saved.GetOpenAt().Equal(at.Round(time.Microsecond).Round(time.Millisecond)))
	state, err := appplan.NewTaskReminderStateReader(repo).GetTaskReminderState(t.Context(), 1, saved.GetID().String())
	require.NoError(t, err)
	decision, err := notification.EvaluateTaskOpenedReminder(notification.TaskOpenedReminderIntent{OrgID: 1, TaskID: saved.GetID().String(), TesteeID: saved.GetTesteeID().String(), ScheduleRevision: saved.GetScheduleRevision(), OpenAt: at}, state, at.Add(time.Second))
	require.NoError(t, err)
	require.Empty(t, decision.SuppressCode)
	// Losing candidates reuse the first artifact rather than writing a new entry.
	require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), original, "https://example.test/private-entry-competing", at.Add(time.Second)))
	duplicate, err := opener.CommitOpened(t.Context(), port.OpenRequest{Candidate: original, Guard: func(context.Context, *domain.AssessmentTask) error {
		return errors.New("duplicate re-executed local business")
	}})
	require.NoError(t, err)
	require.Equal(t, first.GetEntryURL(), duplicate.GetEntryURL())
	require.Equal(t, e.Plan.Digest(), lookup(first).Plan.Digest())
	// An original-root guard failure must roll back both task and witness.
	aborted := seed(2)
	require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), aborted, "https://example.test/private-entry-aborted", at))
	_, err = opener.CommitOpened(t.Context(), port.OpenRequest{Candidate: aborted, Guard: func(context.Context, *domain.AssessmentTask) error { return errors.New("original authority denied") }})
	require.ErrorIs(t, err, eventport.ErrUnresolved)
	failed, err := repo.FindByID(t.Context(), aborted.GetID())
	require.NoError(t, err)
	require.True(t, failed.IsPending())
	require.Empty(t, failed.GetEntryURL())
	require.NotEqual(t, transaction.LocalCommitted, owner.Confirm(t.Context(), lookup(aborted)))
	// Fresh owner and adapter recover exact committed evidence without a token.
	fresh, err := boundary.New(db, &producerOptions, routes, []string{topic})
	require.NoError(t, err)
	recovery, err := taskopen.New(db, fresh, routes, repo.(taskopen.SnapshotWriter))
	require.NoError(t, err)
	recovered, handled, err := recovery.RecoverOpened(t.Context(), saved)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, first.GetEntryURL(), recovered.GetEntryURL())
	require.NoError(t, owner.Runtime().SetAdmission(false))
	paused := seed(3)
	require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), paused, "https://example.test/private-entry-paused", at))
	_, err = opener.CommitOpened(t.Context(), port.OpenRequest{Candidate: paused, Guard: func(context.Context, *domain.AssessmentTask) error {
		t.Error("closed admission ran business")
		return nil
	}})
	require.Error(t, err)
	pending, err := repo.FindByID(t.Context(), paused.GetID())
	require.NoError(t, err)
	require.True(t, pending.IsPending())
	// No legacy publish-outbox schema is installed by the selected module.
	require.False(t, db.Migrator().HasTable("rm_outbox"))
	require.False(t, db.Migrator().HasTable("domain_event_outbox"))

	// Best-effort notifications use separate NORMAL physical topics. Both
	// independent logical subscriptions receive the same original wire.
	normalOptions := load("QS_M3_NORMAL_PRODUCER_CONFIG")
	topics := []string{"qs.plan.task", "qs.survey.lifecycle"}
	normalPublisher, err := messagingruntime.NewNormalWirePublisher(&normalOptions, topics, 2)
	require.NoError(t, err)
	require.NoError(t, normalPublisher.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, normalPublisher.Close(ctx))
	})
	outputs := []chan transport.Received{make(chan transport.Received, 16), make(chan transport.Received, 16)}
	for i, key := range []string{"QS_M3_NORMAL_A_CONFIG", "QS_M3_NORMAL_B_CONFIG"} {
		opt := load(key)
		config, err := opt.RocketMQ.Load(topics, 2, 5*time.Second)
		require.NoError(t, err)
		subscriber, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: config, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
		require.NoError(t, err)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			require.NoError(t, subscriber.Close(ctx))
		})
		for _, topic := range topics {
			require.NoError(t, subscriber.Subscribe(t.Context(), topic, []string{"qs-m3-normal-a", "qs-m3-normal-b"}[i], func(_ context.Context, d transport.Delivery) error {
				m := d.Message()
				if err := d.Ack(); err != nil {
					return err
				}
				select {
				case outputs[i] <- m:
					return nil
				default:
					return errors.New("normal fixture queue full")
				}
			}, func(context.Context, transport.Failure) error { return errors.New("normal fixture failed") }))
		}
	}
	routed, err := eventruntime.NewBestEffortPublisher(routes, normalPublisher)
	require.NoError(t, err)
	notifications := []event.DomainEvent{
		event.New(domain.EventTypeTaskCompleted, domain.AggregateTypeTask, first.GetID().String(), map[string]string{"task_id": first.GetID().String()}),
		event.New("questionnaire.changed", "Questionnaire", "fixture-questionnaire", map[string]string{"code": "fixture-questionnaire"}),
		event.New("assessment_model.changed", "AssessmentModel", "fixture-model", map[string]string{"code": "fixture-model"}),
	}
	committedPublisher := planinfra.NewCommitPublisher(routed)
	uow := hostmysql.NewUnitOfWork(db)
	rollback := errors.New("normal original root abort")
	require.ErrorIs(t, uow.WithinTransaction(t.Context(), func(root context.Context) error {
		require.NoError(t, committedPublisher.Publish(root, notifications[0]))
		return rollback
	}), rollback)
	for _, output := range outputs {
		select {
		case <-output:
			t.Fatal("NORMAL published after root rollback")
		case <-time.After(500 * time.Millisecond):
		}
	}
	require.NoError(t, uow.WithinTransaction(t.Context(), func(root context.Context) error {
		for _, notification := range notifications {
			if err := committedPublisher.Publish(root, notification); err != nil {
				return err
			}
		}
		for _, output := range outputs {
			select {
			case <-output:
				t.Fatal("NORMAL published before original root commit")
			default:
			}
		}
		return nil
	}))
	wires := map[string][]byte{}
	for _, output := range outputs {
		collected := map[string]bool{}
		deadline := time.NewTimer(15 * time.Second)
		for len(collected) < len(notifications) {
			select {
			case m := <-output:
				envelope, err := domainwire.DecodeEnvelope(m.Payload)
				require.NoError(t, err)
				found := false
				for _, want := range notifications {
					if want.EventID() == m.ID {
						require.Equal(t, want.EventType(), envelope.EventType)
						found = true
						break
					}
				}
				require.True(t, found)
				if previous, ok := wires[m.ID]; ok {
					require.Equal(t, previous, m.Wire)
				} else {
					wires[m.ID] = m.Wire
				}
				collected[m.ID] = true
			case <-deadline.C:
				t.Fatal("independent NORMAL fanout incomplete")
			}
		}
		deadline.Stop()
	}
	// Same logical topic does not allow durable events through NORMAL.
	require.Error(t, routed.Publish(t.Context(), domain.NewTaskOpenedReminderRequestedEvent(first.Events()[0].(domain.TaskOpenedEvent), first.GetScheduleRevision())))
	cases := map[string]bool{"constructor_no_ddl": true, "half_invisible": true, "original_task_witness_wire": true, "first_entry_reused": true, "root_abort_atomic": true, "fresh_owner_recovery": true, "closed_admission_no_business": true, "no_new_publish_outbox": true, "normal_best_effort_fanout": true, "normal_root_commit_boundary": true, "normal_rejects_transaction_event": true, "reminder_precision_compatible": true}
	receipt, err := json.Marshal(map[string]any{"cases": cases, "passed": len(cases), "failed": 0, "skipped": 0, "root": "original_mysql_gorm", "process_exit_not_claimed": true, "system_login_not_claimed": true})
	require.NoError(t, err)
	t.Log("M3_QS_TASK_RECEIPT " + string(receipt))
}
