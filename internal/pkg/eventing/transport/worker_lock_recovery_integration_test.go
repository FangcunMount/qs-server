//go:build integration

package transport

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/alicebob/miniredis/v2"
	"github.com/nsqio/go-nsq"
	redis "github.com/redis/go-redis/v9"
)

type lockRecoveryRuntime struct {
	topic   string
	handler handlers.HandlerFunc
}

func (r lockRecoveryRuntime) GetTopicSubscriptions() []eventcatalog.TopicSubscription {
	return []eventcatalog.TopicSubscription{{TopicName: r.topic, EventTypes: []string{"answersheet.submitted"}}}
}

func (r lockRecoveryRuntime) DispatchEvent(ctx context.Context, eventType string, payload []byte) (eventruntime.DispatchResult, error) {
	if err := r.handler(ctx, eventType, payload); err != nil {
		return eventruntime.DispatchResult{}, err
	}
	return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, nil
}

type lockRecoveryObserver struct {
	events chan eventobservability.ConsumeEvent
}

func (o *lockRecoveryObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (o *lockRecoveryObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}
func (o *lockRecoveryObserver) ObserveConsume(_ context.Context, event eventobservability.ConsumeEvent) {
	o.events <- event
}

type sqlAssessmentIntakeProof struct {
	db    *sql.DB
	calls atomic.Int32
}

func (p *sqlAssessmentIntakeProof) EnsureAssessment(ctx context.Context, req *evalpb.EnsureAssessmentRequest) (*evalpb.EnsureAssessmentResponse, error) {
	p.calls.Add(1)
	result, err := p.db.ExecContext(ctx, `INSERT IGNORE INTO assessment_lock_proof(answer_sheet_id) VALUES (?)`, req.AnswerSheetId)
	if err != nil {
		return nil, err
	}
	created, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	return &evalpb.EnsureAssessmentResponse{AssessmentId: req.AnswerSheetId, Created: created == 1, AutoSubmitted: true}, nil
}

func TestWorkerLockHolderExitRetriesBeforeAck(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE assessment_lock_proof (answer_sheet_id BIGINT UNSIGNED PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	keys := keyspace.NewBuilderWithNamespace(keyspace.ComposeNamespace("m4-worker", "cache:lock"))
	redisHandle := &redisruntime.Handle{Client: redisClient, Builder: keys}
	manager := redisadapter.NewManager("worker", "lock_lease", redisHandle)
	runner := locksubsystem.New(locksubsystem.Options{Component: "worker", Handle: redisHandle, Manager: manager, RenewalEnabled: true})
	capability, ok := locklease.Lookup(locklease.WorkloadAnswersheetProcessing)
	if !ok {
		t.Fatal("answersheet lock capability missing")
	}
	const answerSheetID = uint64(456)
	lease, acquired, err := manager.AcquireSpec(t.Context(), capability.Spec, "answersheet:processing:456")
	if err != nil || !acquired || lease == nil {
		t.Fatalf("hold original lease: acquired=%t lease=%v err=%v", acquired, lease, err)
	}
	// Leave the holder's lease in Redis: FastForward below simulates its process
	// exiting without releasing the lock or committing a business fact.
	intake := &sqlAssessmentIntakeProof{db: db}
	handler, ok := handlers.NewRegistry().Create("answersheet_submitted_handler", &handlers.Dependencies{
		Logger: slog.Default(), LockManager: manager, LockRunner: runner, LockKeyBuilder: keys, AssessmentIntakeClient: intake,
	})
	if !ok {
		t.Fatal("answersheet handler missing")
	}
	topic := fmt.Sprintf("qs-worker-lock-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)
	observer := &lockRecoveryObserver{events: make(chan eventobservability.ConsumeEvent, 8)}
	subscriber, err := NewSubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), NSQMessageTimeout: time.Minute,
	}, basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 3,
		RetryBackoff:         basemessaging.RetryBackoffOptions{BaseDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond},
		FailedMessageHandler: FailedMessageHandler(recorder),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		UnknownRecorder: NewUnknownEventRecorder("nsq", recorder),
		ServiceName:     channel, Logger: slog.Default(), Runtime: lockRecoveryRuntime{topic: topic, handler: handler}, Subscriber: subscriber, Observer: observer,
	}); err != nil {
		t.Fatal(err)
	}
	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	payload := []byte(`{"id":"lock-recovery-event","eventType":"answersheet.submitted","occurredAt":"2026-09-23T00:00:00Z","aggregateType":"AnswerSheet","aggregateID":"456","data":{"answersheet_id":"456","questionnaire_code":"Q-001","questionnaire_version":"v1","org_id":1,"testee_id":2,"filler_id":4,"admission":{"purpose":"assessment","model_kind":"scale","model_code":"MODEL-1","model_version":"1.0.0"}}}`)
	message := basemessaging.NewMessage("lock-recovery-event", payload)
	message.Metadata["event_type"] = "answersheet.submitted"
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}
	first := waitWorkerConsumeOutcome(t, observer.events, eventobservability.ConsumeOutcomeDispatchFailed)
	if first.Attempts != 1 || intake.calls.Load() != 0 {
		t.Fatalf("contended delivery: attempts=%d intakeCalls=%d, want 1/0", first.Attempts, intake.calls.Load())
	}
	mini.FastForward(capability.Spec.DefaultTTL)
	second := waitWorkerConsumeOutcome(t, observer.events, eventobservability.ConsumeOutcomeAcked)
	if second.Attempts < 2 || intake.calls.Load() != 1 {
		t.Fatalf("recovered delivery: attempts=%d intakeCalls=%d, want at least 2/1", second.Attempts, intake.calls.Load())
	}
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}
	waitWorkerConsumeOutcome(t, observer.events, eventobservability.ConsumeOutcomeAcked)
	var businessFacts, deadLetters int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM assessment_lock_proof`).Scan(&businessFacts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_delivery_dead_letter`).Scan(&deadLetters); err != nil {
		t.Fatal(err)
	}
	if businessFacts != 1 || intake.calls.Load() != 2 || deadLetters != 0 {
		t.Fatalf("facts=%d intakeCalls=%d deadLetters=%d, want 1/2/0", businessFacts, intake.calls.Load(), deadLetters)
	}
}

func waitWorkerConsumeOutcome(t *testing.T, events <-chan eventobservability.ConsumeEvent, want eventobservability.ConsumeOutcome) eventobservability.ConsumeEvent {
	t.Helper()
	select {
	case event := <-events:
		if event.Outcome != want {
			t.Fatalf("consume outcome = %q, want %q", event.Outcome, want)
		}
		return event
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for consume outcome %q", want)
		return eventobservability.ConsumeEvent{}
	}
}
