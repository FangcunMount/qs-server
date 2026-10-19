package iamauth

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestSDKVersionSyncReorderedHintsDoNotRenewProofOrRestoreOldSubjects(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	version := int64(10)
	unavailable := false
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		if unavailable {
			return 0, errors.New("primary unavailable")
		}
		return version, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	if _, err = guard.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	loader := NewSnapshotLoader(nil, SnapshotLoaderOptions{VersionGuard: guard, CacheTTL: time.Minute})
	keys := []string{cacheKey("42", "qs"), cacheKey("43", "qs")}
	for _, key := range keys {
		if err = loader.setCached(key, &authz.Snapshot{AuthzVersion: 10}); err != nil {
			t.Fatal(err)
		}
	}
	subscriber := &versionSDKSubscriberStub{}
	if err = SubscribeVersionChangesSDK(t.Context(), subscriber, "", "m3-qs-fixture", loader); err != nil {
		t.Fatal(err)
	}
	originalProof := guard.started
	clock.advance(9 * time.Second)
	for _, payload := range []string{`{"version":11}`, `{"version":9}`, `{"version":11}`, `{"version":10}`} {
		delivery := &versionSDKDeliveryStub{message: rmtransport.Received{Payload: []byte(payload)}}
		if err = subscriber.handler(t.Context(), delivery); err != nil || delivery.acks != 1 {
			t.Fatal("actual SDK callback did not settle valid hint")
		}
	}
	if !guard.started.Equal(originalProof) || guard.observed != 11 || loader.globalVersion != 11 {
		t.Fatal("reordered notifications regressed target or renewed authority")
	}
	for _, key := range keys {
		if loader.getCached(key) != nil {
			t.Fatal("one subject retained a snapshot older than the notification watermark")
		}
		if err = loader.setCached(key, &authz.Snapshot{AuthzVersion: 10}); err == nil {
			t.Fatal("old subject restored after revocation hint")
		}
	}
	unavailable = true
	clock.advance(time.Second)
	if err = loader.VerifySnapshot(t.Context(), &authz.Snapshot{AuthzVersion: 11}); err == nil {
		t.Fatal("notification alone authorized a decision during primary outage")
	}
	unavailable = false
	version = 11
	if err = guard.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = loader.VerifySnapshot(t.Context(), &authz.Snapshot{AuthzVersion: 11}); err != nil {
		t.Fatal(err)
	}
	if err = loader.VerifySnapshot(t.Context(), &authz.Snapshot{AuthzVersion: 10}); err == nil {
		t.Fatal("fresh authority accepted an old in-flight snapshot")
	}
}
