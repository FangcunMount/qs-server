//go:build integration

package transport

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
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

	publisher, err := messagingruntime.NewSDKNSQWirePublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	if err := publisher.PublishWire(t.Context(), topic, []byte(`{"version":2}`)); err != nil {
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
