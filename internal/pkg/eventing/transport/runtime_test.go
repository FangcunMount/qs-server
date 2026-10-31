package transport

import (
	"context"
	"errors"
	"testing"

	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

type deadLetterRecorderStub struct {
	record DeadLetterRecord
	err    error
}

func (s *deadLetterRecorderStub) RecordDeadLetter(_ context.Context, record DeadLetterRecord) error {
	s.record = record
	return s.err
}

func TestSDKDeliverySubscriberRejectsRetiredProviderBeforeResources(t *testing.T) {
	failed := func(context.Context, rmtransport.Failure) error { return nil }
	for _, provider := range []string{"", "nsq", "unknown"} {
		if _, err := NewSDKDeliverySubscriber(SubscriberConfig{Provider: provider}, 1, 8, failed); err == nil {
			t.Fatalf("retired or missing provider accepted: %q", provider)
		}
	}
	if _, err := NewSDKDeliverySubscriber(SubscriberConfig{Provider: "rocketmq"}, 1, 8, nil); err == nil {
		t.Fatal("missing provider-aware durable failure handler accepted")
	}
}

func TestSDKFailedHandoffAndUnknownDeliveryKeepBothMessageIdentities(t *testing.T) {
	recorder := &deadLetterRecorderStub{}
	handoff := legacy.FailedHandoff{
		Topic: "evaluation", Channel: "worker", UUID: "app-1", TransportMessageID: "physical-1",
		Payload: []byte(`{"id":"event-1","data":{"org_id":7}}`), Attempts: 8, Cause: "handler failed",
	}
	if err := SDKFailedHandoffHandler(recorder)(t.Context(), handoff); err != nil {
		t.Fatal(err)
	}
	if got := recorder.record; got.MessageID != "app-1" || got.TransportMessageID != "physical-1" || got.EventID != "event-1" || got.DeliveryAttempts != 8 || got.LastError != "handler failed" || got.OrgID == nil || *got.OrgID != 7 {
		t.Fatalf("SDK handoff audit = %#v", got)
	}
	received := rmtransport.Received{
		ID: "app-2", TransportID: "physical-2", Topic: "evaluation", Channel: "worker",
		Payload: []byte(`{"id":"event-2","data":{"org_id":9}}`), Attempts: 2,
	}
	if err := NewDeliveryUnknownEventRecorder("nsq", recorder)(t.Context(), received, "future.event"); err != nil {
		t.Fatal(err)
	}
	if got := recorder.record; got.MessageID != "app-2" || got.TransportMessageID != "physical-2" || got.EventID != "event-2" || got.DeliveryAttempts != 2 || got.LastError != "unknown event type: future.event" || got.Provider != "nsq" || got.OrgID == nil || *got.OrgID != 9 || string(got.Payload) != string(received.Payload) {
		t.Fatalf("SDK unknown audit = %#v", got)
	}
	wantErr := errors.New("audit unavailable")
	recorder.err = wantErr
	if err := SDKFailedHandoffHandler(recorder)(t.Context(), handoff); !errors.Is(err, wantErr) {
		t.Fatalf("handoff audit error = %v, want %v", err, wantErr)
	}
	if err := NewDeliveryUnknownEventRecorder("nsq", recorder)(t.Context(), received, "future.event"); !errors.Is(err, wantErr) {
		t.Fatalf("unknown audit error = %v, want %v", err, wantErr)
	}
	if err := SDKFailedHandoffHandler(recorder)(t.Context(), legacy.FailedHandoff{}); err == nil {
		t.Fatal("invalid terminal handoff could be acknowledged")
	}
}
