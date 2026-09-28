package iamauth

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

type versionSDKSubscriberStub struct {
	topic, channel string
	handler        rmtransport.Handler
}

func (s *versionSDKSubscriberStub) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	s.topic, s.channel, s.handler = topic, channel, handler
	return nil
}

type versionSDKDeliveryStub struct {
	message rmtransport.Received
	acks    int
	ackErr  error
}

func (d *versionSDKDeliveryStub) Message() rmtransport.Received { return d.message }
func (d *versionSDKDeliveryStub) Ack() error {
	d.acks++
	return d.ackErr
}
func (*versionSDKDeliveryStub) Nack(error) error { return nil }
func (d *versionSDKDeliveryStub) Settled() bool  { return d.acks > 0 }

func TestSDKVersionSyncAcknowledgesAfterWatermarkAndIgnoresInvalidVersions(t *testing.T) {
	loader := NewSnapshotLoader(nil, SnapshotLoaderOptions{})
	key := cacheKey("42", "qs")
	if err := loader.setCached(key, &authz.Snapshot{AuthzVersion: 1}); err != nil {
		t.Fatal(err)
	}
	subscriber := &versionSDKSubscriberStub{}
	if err := SubscribeVersionChangesSDK(t.Context(), subscriber, "", "test-channel", loader); err != nil {
		t.Fatal(err)
	}
	if subscriber.topic != DefaultVersionTopic || subscriber.channel != "test-channel" || subscriber.handler == nil {
		t.Fatalf("subscription route = %q/%q", subscriber.topic, subscriber.channel)
	}
	for _, payload := range []string{`not-json`, `{"version":0}`} {
		delivery := &versionSDKDeliveryStub{message: rmtransport.Received{Payload: []byte(payload)}}
		if err := subscriber.handler(t.Context(), delivery); err != nil || delivery.acks != 1 || loader.getCached(key) == nil {
			t.Fatalf("invalid version %q: err=%v, acks=%d, cache=%v", payload, err, delivery.acks, loader.getCached(key))
		}
	}
	delivery := &versionSDKDeliveryStub{message: rmtransport.Received{Payload: []byte(`{"version":2}`)}}
	if err := subscriber.handler(t.Context(), delivery); err != nil || delivery.acks != 1 || loader.getCached(key) != nil {
		t.Fatalf("new version: err=%v, acks=%d, cache=%v", err, delivery.acks, loader.getCached(key))
	}
	if err := loader.setCached(key, &authz.Snapshot{AuthzVersion: 1}); err == nil {
		t.Fatal("snapshot behind SDK notification watermark accepted")
	}
}

func TestSDKVersionSyncReturnsAckFailure(t *testing.T) {
	subscriber := &versionSDKSubscriberStub{}
	if err := SubscribeVersionChangesSDK(t.Context(), subscriber, "iam.version", "channel", NewSnapshotLoader(nil, SnapshotLoaderOptions{})); err != nil {
		t.Fatal(err)
	}
	want := errors.New("ack failed")
	delivery := &versionSDKDeliveryStub{message: rmtransport.Received{Payload: []byte(`{"version":2}`)}, ackErr: want}
	if err := subscriber.handler(context.Background(), delivery); !errors.Is(err, want) || delivery.acks != 1 {
		t.Fatalf("ack failure = %v, acks=%d", err, delivery.acks)
	}
}
