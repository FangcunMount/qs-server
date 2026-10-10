package transport

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

type deliverySourceFactsStub struct {
	onSubscribe func(string, string) error
	closed      bool
}

func (s *deliverySourceFactsStub) Subscribe(_ context.Context, topic, channel string, _ rmtransport.Handler, _ func(context.Context, legacy.FailedHandoff) error) error {
	return s.onSubscribe(topic, channel)
}
func (s *deliverySourceFactsStub) Close(context.Context) error { s.closed = true; return nil }

func TestSDKDeliveryFactsCaptureActualConstructorAndPartialSubscribe(t *testing.T) {
	owner, err := runtimefacts.New("worker", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	loaded := SubscriberConfig{Provider: "nsq", NSQLookupdAddr: "resolved-lookupd.internal:14161", NSQDHTTPEndpoints: []string{"https://resolved-nsqd.internal:14451"}, FailedHandoffGroup: "resolved-group"}
	s, err := NewSDKDeliverySubscriberWithFacts(loaded, 1, 8, func(context.Context, legacy.FailedHandoff) error { return nil }, owner, "worker-events")
	if err != nil {
		t.Fatal(err)
	}
	// New constructs the actual SDK subscriber without opening a broker connection.
	if err := s.inner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded.NSQDHTTPEndpoints[0] = "http://mutated.invalid:1"
	fact := owner.Snapshot().Transports[0]
	if fact.State != "declared" || fact.ClientID != owner.ClientID("worker-events") || fact.Hostname != owner.Hostname() || !reflect.DeepEqual(fact.LookupdAddresses, []string{"resolved-lookupd.internal:14161"}) || !reflect.DeepEqual(fact.NSQDHTTPAddresses, []string{"https://resolved-nsqd.internal:14451"}) || len(fact.NSQDTCPAddresses) != 0 {
		t.Fatal("actual loaded constructor projection changed or inferred a TCP source")
	}
	wantErr := errors.New("subscription rejected")
	stub := &deliverySourceFactsStub{onSubscribe: func(topic, channel string) error {
		if topic == "event.first" {
			return nil
		}
		if owner.Snapshot().Transports[0].State != "started" {
			t.Fatal("second subscription observed before first completed")
		}
		return wantErr
	}}
	s.inner = stub
	handler := func(context.Context, rmtransport.Delivery) error { return nil }
	if err := s.Subscribe("event.first", "worker-channel", handler); err != nil {
		t.Fatal(err)
	}
	fact = owner.Snapshot().Transports[0]
	if len(fact.Subscriptions) != 1 || fact.Subscriptions[0].FailureTopic != legacy.FailedHandoffTopicForGroup("event.first", "resolved-group") || fact.Subscriptions[0].FailureChannel != legacy.FailedHandoffChannel {
		t.Fatal("successful business/failure subscription configuration absent")
	}
	if err := s.Subscribe("event.second", "worker-channel", handler); !errors.Is(err, wantErr) {
		t.Fatalf("original subscription error lost: %v", err)
	}
	snapshot := owner.Snapshot()
	if snapshot.Transports[0].State != "incomplete" || len(snapshot.Transports[0].Subscriptions) != 1 || snapshot.ObservationComplete || snapshot.BrokerConnectionsVerified {
		t.Fatal("partial registration claimed completion or broker connection")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !stub.closed || owner.Snapshot().Transports[0].State != "stopped" {
		t.Fatal("actual stop did not invalidate observation")
	}
}

func TestSDKDeliveryFactsDifferentProcessesOnSameChannel(t *testing.T) {
	first, _ := runtimefacts.New("worker", strings.Repeat("a", 40))
	second, _ := runtimefacts.New("worker", strings.Repeat("a", 40))
	defer func() { _ = first.Close(); _ = second.Close() }()
	config := SubscriberConfig{Provider: "nsq", NSQLookupdAddr: "lookupd.internal:4161"}
	for _, owner := range []*runtimefacts.Owner{first, second} {
		s, err := NewSDKDeliverySubscriberWithFacts(config, 1, 8, func(context.Context, legacy.FailedHandoff) error { return nil }, owner, "worker-events")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if first.Snapshot().Transports[0].ClientID == second.Snapshot().Transports[0].ClientID {
		t.Fatal("two process owners share consumer identity")
	}
}
