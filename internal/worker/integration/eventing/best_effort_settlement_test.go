package eventing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/FangcunMount/qs-server/internal/worker/port"
)

type failingBestEffortClient struct {
	handlers.InternalClient
	calls int
}

func (c *failingBestEffortClient) HandleQuestionnairePublishedPostActions(context.Context, string, string) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	c.calls++
	return nil, errors.New("post-action unavailable")
}

func (c *failingBestEffortClient) HandleScalePublishedPostActions(context.Context, string) (*pb.GenerateScaleQRCodeResponse, error) {
	c.calls++
	return nil, errors.New("post-action unavailable")
}

func (c *failingBestEffortClient) SendTaskOpenedMiniProgramNotification(context.Context, int64, string, uint64, string, time.Time) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	c.calls++
	return nil, errors.New("mini-program unavailable")
}

type failingTaskNotifier struct {
	calls int
}

func (n *failingTaskNotifier) NotifyTaskCompleted(context.Context, port.NotificationMeta, port.TaskCompletedNotification) error {
	n.calls++
	return errors.New("notification gateway unavailable")
}

func (n *failingTaskNotifier) NotifyTaskExpired(context.Context, port.NotificationMeta, port.TaskExpiredNotification) error {
	n.calls++
	return errors.New("notification gateway unavailable")
}

func (n *failingTaskNotifier) NotifyTaskCanceled(context.Context, port.NotificationMeta, port.TaskCanceledNotification) error {
	n.calls++
	return errors.New("notification gateway unavailable")
}

type capturedBestEffortSubscriber struct {
	handlers map[string]basemessaging.Handler
}

func (s *capturedBestEffortSubscriber) Subscribe(topic, _ string, handler basemessaging.Handler) error {
	if s.handlers == nil {
		s.handlers = make(map[string]basemessaging.Handler)
	}
	s.handlers[topic] = handler
	return nil
}

func (s *capturedBestEffortSubscriber) SubscribeWithMiddleware(topic, channel string, handler basemessaging.Handler, middlewares ...basemessaging.Middleware) error {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return s.Subscribe(topic, channel, handler)
}

func (*capturedBestEffortSubscriber) Stop()        {}
func (*capturedBestEffortSubscriber) Close() error { return nil }

type bestEffortConsumeObserver struct {
	events []eventobservability.ConsumeEvent
}

func (*bestEffortConsumeObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (*bestEffortConsumeObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}
func (o *bestEffortConsumeObserver) ObserveConsume(_ context.Context, evt eventobservability.ConsumeEvent) {
	o.events = append(o.events, evt)
}
func (*bestEffortConsumeObserver) ObserveConsumeDuration(context.Context, eventobservability.ConsumeDurationEvent) {
}

func subscribeBestEffortHandlers(t *testing.T, client handlers.InternalClient, notifier port.TaskNotifier, observer eventobservability.Observer) *capturedBestEffortSubscriber {
	t.Helper()
	logger := testLogger()
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(logger, &HandlerDependencies{Logger: logger, InternalClient: client, Notifier: notifier}, handlers.NewRegistry())
	if err := dispatcher.Initialize(eventcatalog.NewCatalog(cfg)); err != nil {
		t.Fatal(err)
	}
	subscriber := &capturedBestEffortSubscriber{}
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		ServiceName: "qs-worker", Logger: logger, Runtime: dispatcher, Subscriber: subscriber, Observer: observer,
		UnknownRecorder: func(context.Context, *basemessaging.Message, string) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	return subscriber
}

func TestBestEffortExternalFailureStillAcknowledgesOriginalEvent(t *testing.T) {
	const occurredAt = "2026-09-25T00:00:00Z"
	cases := []struct {
		name      string
		eventType string
		topic     string
		data      map[string]any
	}{
		{"questionnaire", "questionnaire.changed", "qs.survey.lifecycle", map[string]any{"code": "Q-1", "version": "v1", "action": "published", "changed_at": occurredAt}},
		{"scale", "assessment_model.changed", "qs.survey.lifecycle", map[string]any{"kind": "scale", "code": "S-1", "version": "v1", "action": "published", "changed_at": occurredAt}},
		{"opened", "task.opened", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "entry_url": "https://example.test/entry", "open_at": occurredAt}},
		{"completed", "task.completed", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "assessment_id": "A-1", "completed_at": occurredAt}},
		{"expired", "task.expired", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "reason": "entry_timeout", "expired_at": occurredAt}},
		{"canceled", "task.canceled", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "canceled_at": occurredAt}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &failingBestEffortClient{}
			notifier := &failingTaskNotifier{}
			observer := &bestEffortConsumeObserver{}
			subscriber := subscribeBestEffortHandlers(t, client, notifier, observer)
			consume := subscriber.handlers[tc.topic]
			if consume == nil {
				t.Fatalf("no subscription for %q", tc.topic)
			}
			payload, err := json.Marshal(map[string]any{
				"id": "evt-1", "eventType": tc.eventType, "occurredAt": occurredAt,
				"aggregateType": "Test", "aggregateID": "1", "data": tc.data,
			})
			if err != nil {
				t.Fatal(err)
			}
			msg := basemessaging.NewMessage("broker-msg-1", payload)
			msg.Metadata["event_type"] = tc.eventType
			acks, nacks := 0, 0
			msg.SetAckFunc(func() error { acks++; return nil })
			msg.SetNackFunc(func() error { nacks++; return nil })
			if err := consume(context.Background(), msg); err != nil {
				t.Fatalf("best-effort external failure changed transport result: %v", err)
			}
			if calls := client.calls + notifier.calls; calls != 1 {
				t.Fatalf("external calls = %d, want one failed call", calls)
			}
			if acks != 1 || nacks != 0 || !msg.IsSettled() {
				t.Fatalf("acks=%d nacks=%d settled=%t, want one ack and no nack", acks, nacks, msg.IsSettled())
			}
			if len(observer.events) != 1 || observer.events[0].Outcome != eventobservability.ConsumeOutcomeAcked || observer.events[0].Service != "qs-worker" || observer.events[0].Topic != tc.topic {
				t.Fatalf("consume outcomes = %#v, want one qs-worker ack", observer.events)
			}
		})
	}
}
