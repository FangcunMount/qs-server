//go:build integration && reliable_messaging_m5

package eventing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

type brokerBestEffortObserver struct {
	consumed chan eventobservability.ConsumeEvent
}

func (*brokerBestEffortObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (*brokerBestEffortObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}
func (o *brokerBestEffortObserver) ObserveConsume(_ context.Context, evt eventobservability.ConsumeEvent) {
	o.consumed <- evt
}
func (*brokerBestEffortObserver) ObserveConsumeDuration(context.Context, eventobservability.ConsumeDurationEvent) {
}

type nsqBestEffortChannelStats struct {
	Name         string `json:"channel_name"`
	Depth        int64  `json:"depth"`
	InFlight     int64  `json:"in_flight_count"`
	Deferred     int64  `json:"deferred_count"`
	MessageCount int64  `json:"message_count"`
	RequeueCount int64  `json:"requeue_count"`
	TimeoutCount int64  `json:"timeout_count"`
}

func bestEffortChannelStats(ctx context.Context, client *http.Client, endpoint, topicName, channelName string) (nsqBestEffortChannelStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/stats?format=json", nil)
	if err != nil {
		return nsqBestEffortChannelStats{}, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nsqBestEffortChannelStats{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nsqBestEffortChannelStats{}, fmt.Errorf("NSQ stats returned HTTP %d", response.StatusCode)
	}
	var stats struct {
		Topics []struct {
			Name     string                      `json:"topic_name"`
			Channels []nsqBestEffortChannelStats `json:"channels"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		return nsqBestEffortChannelStats{}, err
	}
	for _, topic := range stats.Topics {
		if topic.Name != topicName {
			continue
		}
		for _, channel := range topic.Channels {
			if channel.Name == channelName {
				return channel, nil
			}
		}
	}
	return nsqBestEffortChannelStats{}, fmt.Errorf("NSQ channel %s/%s absent", topicName, channelName)
}

func waitBestEffortLookupdTopic(ctx context.Context, client *http.Client, topicName string) error {
	endpoint := "http://nsqlookupd:4161/lookup?topic=" + url.QueryEscape(topicName)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(req)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		if ctx.Err() != nil {
			return fmt.Errorf("lookupd producer for %s: %w", topicName, ctx.Err())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestBestEffortExternalFailureFinishesRealNSQDelivery(t *testing.T) {
	runBestEffortRealNSQDelivery(t, true)
}

// The current product contract leaves optional terminal Task notifications
// unconfigured. Broker FIN must not imply a successful external notification.
func TestBestEffortUnconfiguredTaskNotificationsFinishRealNSQDelivery(t *testing.T) {
	runBestEffortRealNSQDelivery(t, false)
}

func runBestEffortRealNSQDelivery(t *testing.T, notificationsConfigured bool) {
	if os.Getenv("QS_M5_BEST_EFFORT_NSQ") != "1" {
		t.Skip("requires the disposable M5-05 NSQ stack")
	}
	const (
		nsqdAddress   = "nsqd:4150"
		lookupAddress = "nsqlookupd:4161"
		statsEndpoint = "http://nsqd:4151"
		channelName   = "qs-worker"
		occurredAt    = "2026-09-25T00:00:00Z"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	logger := testLogger()
	cfg, err := eventcatalog.Load(os.Getenv("QS_M5_EVENTS_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	client := &failingBestEffortClient{}
	notifier := &failingTaskNotifier{}
	observer := &brokerBestEffortObserver{consumed: make(chan eventobservability.ConsumeEvent, 6)}
	deps := &HandlerDependencies{Logger: logger, InternalClient: client}
	if notificationsConfigured {
		deps.Notifier = notifier
	}
	dispatcher := NewDispatcher(logger, deps, handlers.NewRegistry())
	if err := dispatcher.Initialize(eventcatalog.NewCatalog(cfg)); err != nil {
		t.Fatal(err)
	}
	statsClient := &http.Client{Timeout: 2 * time.Second}
	subscriptions := dispatcher.GetTopicSubscriptions()
	topics := make([]string, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		topics = append(topics, subscription.TopicName)
	}
	channels := make([]messagingruntime.DurableChannel, 0, len(topics))
	for _, topic := range topics {
		channels = append(channels, messagingruntime.DurableChannel{Topic: topic, Channel: channelName})
	}
	if err := messagingruntime.EnsureNSQChannels(ctx, []string{statsEndpoint}, channels); err != nil {
		t.Fatal(err)
	}
	for _, topic := range topics {
		if err := waitBestEffortLookupdTopic(ctx, statsClient, topic); err != nil {
			t.Fatal(err)
		}
	}
	var terminal atomic.Int32
	subscriber, err := eventtransport.NewSDKDeliverySubscriber(eventtransport.SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: lookupAddress, NSQMessageTimeout: 2 * time.Second,
	}, 1, 2, func(context.Context, legacy.FailedHandoff) error {
		terminal.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { subscriber.Stop(); _ = subscriber.Close() }()
	if err := workermessaging.SubscribeSDKHandlersWithOptions(workermessaging.SubscribeSDKHandlersOptions{
		ServiceName: channelName, Logger: logger, Runtime: dispatcher, Subscriber: subscriber, Observer: observer,
		UnknownRecorder: func(context.Context, rmtransport.Received, string) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	publisher, err := messagingruntime.NewSDKNSQWirePublisher(nsqdAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = publisher.Close() }()

	cases := []struct {
		eventType string
		topic     string
		data      map[string]any
	}{
		{"questionnaire.changed", "qs.survey.lifecycle", map[string]any{"code": "Q-1", "version": "v1", "action": "published", "changed_at": occurredAt}},
		{"assessment_model.changed", "qs.survey.lifecycle", map[string]any{"kind": "scale", "code": "S-1", "version": "v1", "action": "published", "changed_at": occurredAt}},
		{"task.opened", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "entry_url": "https://example.test/entry", "open_at": occurredAt}},
		{"task.completed", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "assessment_id": "A-1", "completed_at": occurredAt}},
		{"task.expired", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "reason": "entry_timeout", "expired_at": occurredAt}},
		{"task.canceled", "qs.plan.task", map[string]any{"task_id": "T-1", "plan_id": "P-1", "testee_id": "123", "canceled_at": occurredAt}},
	}
	if !notificationsConfigured {
		// task.opened has a separate durable reminder contract and is not one
		// of the five optional post-actions under test.
		cases = append(cases[:2], cases[3:]...)
	}
	for i, tc := range cases {
		beforeNotConfigured := bestEffortSideEffectCount(t, tc.eventType, "not_configured")
		beforeSuccess := bestEffortSideEffectCount(t, tc.eventType, "call_succeeded")
		payload, err := json.Marshal(map[string]any{
			"id": fmt.Sprintf("m5-05-%d", i), "eventType": tc.eventType, "occurredAt": occurredAt,
			"aggregateType": "Test", "aggregateID": "1", "data": tc.data,
		})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := legacy.Encode(legacy.Envelope{UUID: fmt.Sprintf("m5-05-%d", i), Payload: payload, Metadata: map[string]string{"event_type": tc.eventType}}, legacy.Revision2)
		if err != nil {
			t.Fatal(err)
		}
		if err := publisher.PublishWire(ctx, tc.topic, wire); err != nil {
			t.Fatalf("publish %s: %v", tc.eventType, err)
		}
		select {
		case consumed := <-observer.consumed:
			if consumed.EventType != tc.eventType || consumed.Topic != tc.topic || consumed.Outcome != eventobservability.ConsumeOutcomeAcked || consumed.Attempts != 1 {
				t.Fatalf("consume %s: %+v, want first-attempt ACK", tc.eventType, consumed)
			}
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v", tc.eventType, ctx.Err())
		}
		if !notificationsConfigured && i >= 2 {
			if got := bestEffortSideEffectCount(t, tc.eventType, "not_configured") - beforeNotConfigured; got != 1 {
				t.Fatalf("%s not_configured delta=%v, want 1", tc.eventType, got)
			}
			if got := bestEffortSideEffectCount(t, tc.eventType, "call_succeeded") - beforeSuccess; got != 0 {
				t.Fatalf("%s falsely recorded sending success", tc.eventType)
			}
		}
	}
	wantCalls := len(cases)
	if !notificationsConfigured {
		wantCalls = 2
		if notifier.calls != 0 {
			t.Fatal("unconfigured Task notifier was called")
		}
	}
	if calls := client.calls + notifier.calls; calls != wantCalls {
		t.Fatalf("external calls = %d, want %d", calls, wantCalls)
	}
	t.Logf("notifications_configured=%t external_calls=%d first_attempt_ACKs=%d", notificationsConfigured, wantCalls, len(cases))
	taskCount := int64(4)
	if !notificationsConfigured {
		taskCount = 3
	}
	expected := map[string]int64{"qs.survey.lifecycle": 2, "qs.plan.task": taskCount}
	for topic, want := range expected {
		for {
			stats, err := bestEffortChannelStats(ctx, statsClient, statsEndpoint, topic, channelName)
			if err == nil && stats.MessageCount == want && stats.Depth == 0 && stats.InFlight == 0 && stats.Deferred == 0 {
				break
			}
			if ctx.Err() != nil {
				t.Fatalf("wait for settled channel on %s: stats=%+v err=%v", topic, stats, err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	// A missing FIN would become a timeout/redelivery after the configured message timeout.
	time.Sleep(3 * time.Second)
	for topic, want := range expected {
		stats, err := bestEffortChannelStats(ctx, statsClient, statsEndpoint, topic, channelName)
		if err != nil {
			t.Fatal(err)
		}
		if stats.MessageCount != want || stats.RequeueCount != 0 || stats.TimeoutCount != 0 || stats.Depth != 0 || stats.InFlight != 0 || stats.Deferred != 0 {
			t.Fatalf("%s channel state after timeout: %+v, want %d settled deliveries and no retry", topic, stats, want)
		}
		t.Logf("NSQ %s/%s: messages=%d depth=%d in_flight=%d deferred=%d requeue=%d timeout=%d", topic, channelName,
			stats.MessageCount, stats.Depth, stats.InFlight, stats.Deferred, stats.RequeueCount, stats.TimeoutCount)
	}
	if terminal.Load() != 0 {
		t.Fatalf("terminal handoffs = %d, want zero", terminal.Load())
	}
}
