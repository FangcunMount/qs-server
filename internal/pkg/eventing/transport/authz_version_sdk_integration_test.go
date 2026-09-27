//go:build integration

package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	"github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/nsqio/go-nsq"
)

type committedVersionOneReader struct{}

func (committedVersionOneReader) GetCommittedPolicyVersion(context.Context) (int64, error) {
	return 1, nil
}

func TestSDKSubscriberRaisesIAMVersionWatermarkOnEphemeralChannel(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ integration services")
	}
	topic := fmt.Sprintf("qs-authz-sdk-%d", time.Now().UnixNano())
	channel, group := "qs-authz-sdk#ephemeral", "qs-authz-sdk-group"
	cleanupNSQTopics(t, topic, legacy.FailedHandoffTopicForGroup(topic, group))
	createNSQTopicAndChannel(t, topic, channel)

	guard, err := iamauth.NewVersionGuard(committedVersionOneReader{}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if version, err := guard.Verify(t.Context()); err != nil || version != 1 {
		t.Fatalf("initial IAM committed version proof = %d, %v", version, err)
	}
	loader := iamauth.NewSnapshotLoader(nil, iamauth.SnapshotLoaderOptions{VersionGuard: guard})
	subscriber, err := NewSDKDeliverySubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), FailedHandoffGroup: group,
	}, 1, 2,
		func(_ context.Context, failed legacy.FailedHandoff) error {
			return fmt.Errorf("unexpected IAM version failed handoff for %s", failed.Topic)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { subscriber.Stop(); _ = subscriber.Close() })
	if err := iamauth.SubscribeVersionChangesSDK(t.Context(), subscriber, topic, channel, loader); err != nil {
		t.Fatal(err)
	}

	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	if err := publisher.Publish(t.Context(), topic, []byte(`{"version":2}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, err := guard.Verify(t.Context())
		if err != nil && strings.Contains(err.Error(), "behind the observed watermark") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("SDK subscriber did not raise IAM notification watermark before the deadline")
}

// Replacement subscribers use different broadcast channels. The old NSQ
// channel must disappear, while the durable failed-handoff channel remains
// stable so an unfinished audit still has an owner.
func TestSDKAuthzEphemeralChannelLifecycleAcrossSubscriberReplacements(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ integration services")
	}
	topic := fmt.Sprintf("qs-authz-lifecycle-%d", time.Now().UnixNano())
	group := "qs-authz-lifecycle"
	failureTopic := legacy.FailedHandoffTopicForGroup(topic, group)
	channels := []string{"qs-authz-first#ephemeral", "qs-authz-second#ephemeral"}
	cleanupNSQTopics(t, topic, failureTopic)
	createNSQTopicAndChannel(t, topic, channels[0])

	for _, channel := range channels {
		subscriber, err := NewSDKDeliverySubscriber(SubscriberConfig{
			Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), FailedHandoffGroup: group,
		}, 1, 2, func(context.Context, legacy.FailedHandoff) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = subscriber.Close() })
		if err := subscriber.Subscribe(topic, channel, func(_ context.Context, delivery rmtransport.Delivery) error {
			return delivery.Ack()
		}); err != nil {
			t.Fatal(err)
		}
		waitForSDKAuthzChannelState(t, topic, failureTopic, channel, true)
		if err := subscriber.Close(); err != nil {
			t.Fatal(err)
		}
		waitForSDKAuthzChannelState(t, topic, failureTopic, channel, false)
	}
}

func waitForSDKAuthzChannelState(t *testing.T, topic, failureTopic, channel string, connected bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		stats, err := readSDKAuthzNSQStats()
		if err != nil {
			last = err.Error()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var currentPresent, currentConnected, oldChannel, failureChannel, perChannelFailure bool
		for _, item := range stats.Topics {
			switch item.TopicName {
			case topic:
				for _, c := range item.Channels {
					if c.ChannelName == channel {
						currentPresent = true
						currentConnected = len(c.Clients) > 0
					} else if strings.HasPrefix(c.ChannelName, "qs-authz-") {
						oldChannel = true
					}
				}
			case failureTopic:
				for _, c := range item.Channels {
					if c.ChannelName == legacy.FailedHandoffChannel {
						failureChannel = true
					}
				}
			case legacy.FailedHandoffTopic(topic, channel):
				perChannelFailure = true
			}
		}
		if currentPresent == connected && (!connected || currentConnected) && !oldChannel && failureChannel && !perChannelFailure {
			return
		}
		last = fmt.Sprintf("current_present=%t current_connected=%t old=%t stable_failure=%t per_channel_failure=%t", currentPresent, currentConnected, oldChannel, failureChannel, perChannelFailure)
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("NSQ authorization channel lifecycle did not settle: %s", last)
}

type sdkAuthzNSQStats struct {
	Topics []struct {
		TopicName string `json:"topic_name"`
		Channels  []struct {
			ChannelName string            `json:"channel_name"`
			Clients     []json.RawMessage `json:"clients"`
		} `json:"channels"`
	} `json:"topics"`
}

func readSDKAuthzNSQStats() (sdkAuthzNSQStats, error) {
	var stats sdkAuthzNSQStats
	response, err := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151") + "/stats?format=json")
	if err != nil {
		return stats, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return stats, fmt.Errorf("NSQ stats: %s", response.Status)
	}
	err = json.NewDecoder(response.Body).Decode(&stats)
	return stats, err
}
