//go:build integration

package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type claimedEvaluationClient struct {
	runs   evaluationrun.Repository
	calls  atomic.Int32
	claims atomic.Int32
}

func (c *claimedEvaluationClient) ExecuteEvaluation(ctx context.Context, assessmentID uint64) (*pb.ExecuteEvaluationResponse, error) {
	c.calls.Add(1)
	now := time.Now().UTC()
	claim, err := c.runs.Claim(ctx, evaluationrun.ClaimRequest{
		AssessmentID: assessmentID, Token: uuid.NewString(), ClaimedAt: now, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		return nil, err
	}
	if !claim.Claimed {
		if claim.Run.Attempt().Status != evalrun.StatusSucceeded {
			return nil, fmt.Errorf("duplicate delivery found unfinished run %s", claim.Run.ID())
		}
		return &pb.ExecuteEvaluationResponse{Status: "already_evaluated", RunId: claim.Run.ID().String(), CurrentAttempt: int32(claim.Run.Attempt().Number)}, nil
	}
	c.claims.Add(1)
	if err := claim.Run.AttachInputSnapshot("frozen-input-v1"); err != nil {
		return nil, err
	}
	if err := claim.Run.Succeed(now); err != nil {
		return nil, err
	}
	if err := c.runs.SaveClaimed(ctx, claim.Run); err != nil {
		return nil, err
	}
	return &pb.ExecuteEvaluationResponse{Status: "evaluated", RunId: claim.Run.ID().String(), CurrentAttempt: int32(claim.Run.Attempt().Number)}, nil
}

type claimedEvaluationRuntime struct {
	topic   string
	handler handlers.HandlerFunc
	results chan error
}

func (r *claimedEvaluationRuntime) GetTopicSubscriptions() []eventcatalog.TopicSubscription {
	return []eventcatalog.TopicSubscription{{TopicName: r.topic, EventTypes: []string{eventcatalog.EvaluationRequested}}}
}

func (r *claimedEvaluationRuntime) DispatchEvent(ctx context.Context, eventType string, payload []byte) (eventruntime.DispatchResult, error) {
	if eventType != eventcatalog.EvaluationRequested {
		return eventruntime.DispatchResult{Outcome: eventruntime.DispatchUnknown}, nil
	}
	err := r.handler(ctx, eventType, payload)
	r.results <- err
	return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, err
}

func TestWorkerDuplicateNSQEvaluationRequestKeepsOneDurableAttempt(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.AutoMigrate(&checkpoint.RuntimeCheckpointPO{}); err != nil {
		t.Fatal(err)
	}
	client := &claimedEvaluationClient{runs: checkpoint.NewRunRepository(gormDB)}
	handler, ok := handlers.NewRegistry().Create("evaluation_requested_handler", &handlers.Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), EvaluationWorkerClient: client,
	})
	if !ok {
		t.Fatal("evaluation_requested_handler is not registered")
	}
	topic := fmt.Sprintf("qs-worker-evaluation-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)
	runtime := &claimedEvaluationRuntime{topic: topic, handler: handler, results: make(chan error, 2)}
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	subscriber, err := NewSubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), NSQMessageTimeout: time.Minute,
	}, basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 2,
		RetryBackoff:         basemessaging.RetryBackoffOptions{BaseDelay: 100 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
		FailedMessageHandler: FailedMessageHandler(recorder),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		ServiceName: channel, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Runtime: runtime, Subscriber: subscriber,
	}); err != nil {
		t.Fatal(err)
	}
	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	const assessmentID = uint64(90020001)
	payload, err := json.Marshal(map[string]any{
		"id": "evaluation-requested-90020001", "eventType": eventcatalog.EvaluationRequested,
		"occurredAt": time.Now().UTC(), "aggregateType": "Evaluation", "aggregateID": "90020001",
		"data": map[string]any{
			"org_id": 501, "assessment_id": assessmentID, "testee_id": 6001,
			"questionnaire_code": "QNR-M4", "questionnaire_version": "1.0.0",
			"answersheet_id": "90010003", "model_code": "MODEL-1", "requested_at": time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	message := basemessaging.NewMessage("evaluation-requested-90020001", payload)
	message.Metadata["event_type"] = eventcatalog.EvaluationRequested
	for i := range 2 {
		if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
			t.Fatal(err)
		}
		select {
		case dispatchErr := <-runtime.results:
			if dispatchErr != nil {
				t.Fatalf("delivery %d: %v", i+1, dispatchErr)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("delivery %d did not reach the original Worker handler", i+1)
		}
	}
	var attempts int64
	if err := gormDB.Model(&checkpoint.RuntimeCheckpointPO{}).Where("scope=? AND assessment_id=?", "evaluation_run", assessmentID).Count(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || client.calls.Load() != 2 || client.claims.Load() != 1 {
		t.Fatalf("attempt rows=%d worker calls=%d claimed=%d, want 1/2/1", attempts, client.calls.Load(), client.claims.Load())
	}
	latest, err := client.runs.FindLatestByAssessmentID(t.Context(), assessmentID)
	if err != nil || latest == nil {
		t.Fatalf("read durable attempt: run=%v err=%v", latest, err)
	}
	if latest.Attempt().Number != 1 || latest.Attempt().Status != evalrun.StatusSucceeded || latest.InputSnapshotRef() != "frozen-input-v1" {
		t.Fatalf("durable attempt=%d status=%s input=%q", latest.Attempt().Number, latest.Attempt().Status, latest.InputSnapshotRef())
	}
	var deadLetters int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_delivery_dead_letter`).Scan(&deadLetters); err != nil {
		t.Fatal(err)
	}
	if deadLetters != 0 {
		t.Fatalf("unexpected dead letters: %d", deadLetters)
	}
}
