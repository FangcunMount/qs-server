package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
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

func TestNewSubscriberOptionsLocksGovernedTransportPolicy(t *testing.T) {
	handler := func(context.Context, basemessaging.FailedMessage) error { return nil }
	options, err := NewSubscriberOptions(17, 8, handler)
	if err != nil {
		t.Fatal(err)
	}
	if options.MaxInFlight != 17 || options.MaxAttempts != 8 || options.FailedMessageHandler == nil {
		t.Fatalf("options = %#v", options)
	}
	if options.RetryBackoff.BaseDelay != 30*time.Second || options.RetryBackoff.MaxDelay != 5*time.Minute || options.RetryBackoff.JitterFraction != 0.2 {
		t.Fatalf("retry backoff = %#v", options.RetryBackoff)
	}
}

func TestNewSubscriberOptionsRejectsMissingTerminalHandlerAndHardCap(t *testing.T) {
	if _, err := NewSubscriberOptions(1, 8, nil); err == nil {
		t.Fatal("missing failed-message handler accepted")
	}
	handler := func(context.Context, basemessaging.FailedMessage) error { return nil }
	for _, attempts := range []int{0, 9} {
		if _, err := NewSubscriberOptions(1, attempts, handler); err == nil {
			t.Fatalf("attempts %d accepted", attempts)
		}
	}
}

func TestNewNSQConfigPreservesDefaultAndAppliesExplicitMessageTimeout(t *testing.T) {
	t.Parallel()

	defaultConfig, err := newNSQConfig(0)
	if err != nil {
		t.Fatal(err)
	}
	if defaultConfig.MsgTimeout != 0 {
		t.Fatalf("default NSQ message timeout = %s, want server-negotiated zero value", defaultConfig.MsgTimeout)
	}
	if _, err := newNSQConfig(-time.Second); err == nil {
		t.Fatal("negative NSQ message timeout accepted")
	}
	config, err := newNSQConfig(6 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if config.MsgTimeout != 6*time.Minute {
		t.Fatalf("NSQ message timeout = %s, want 6m", config.MsgTimeout)
	}
}

func TestFailedMessageHandlerPreservesTransportEvidence(t *testing.T) {
	recorder := &deadLetterRecorderStub{}
	handler := FailedMessageHandler(recorder)
	message := basemessaging.NewMessage("message-1", []byte(`{"id":"event-1","data":{"org_id":7}}`))
	message.TransportMessageID = "physical-nsq-1"
	wantErr := errors.New("decode failed")
	if err := handler(t.Context(), basemessaging.FailedMessage{
		Provider: "nsq", Topic: "evaluation", Channel: "worker", Message: message, Attempts: 8, Cause: wantErr,
	}); err != nil {
		t.Fatal(err)
	}
	if recorder.record.MessageID != "message-1" || recorder.record.TransportMessageID != "physical-nsq-1" || recorder.record.EventID != "event-1" || recorder.record.OrgID == nil || *recorder.record.OrgID != 7 || recorder.record.DeliveryAttempts != 8 || recorder.record.LastError != wantErr.Error() {
		t.Fatalf("record = %#v", recorder.record)
	}
}

func TestUnknownEventRecorderPreservesPayloadAndRejectsMissingAudit(t *testing.T) {
	recorder := &deadLetterRecorderStub{}
	msg := basemessaging.NewMessage("message-2", []byte(`{"id":"event-2","data":{"org_id":501}}`))
	msg.Topic, msg.Channel, msg.Attempts = "evaluation", "worker", 1
	if err := NewUnknownEventRecorder("nsq", recorder)(t.Context(), msg, "future.event"); err != nil {
		t.Fatal(err)
	}
	if recorder.record.MessageID != msg.UUID || recorder.record.EventID != "event-2" || recorder.record.OrgID == nil || *recorder.record.OrgID != 501 || recorder.record.Provider != "nsq" || recorder.record.LastError != "unknown event type: future.event" || string(recorder.record.Payload) != string(msg.Payload) {
		t.Fatalf("unknown event record = %#v", recorder.record)
	}
	wantErr := errors.New("audit unavailable")
	recorder.err = wantErr
	if err := NewUnknownEventRecorder("nsq", recorder)(t.Context(), msg, "future.event"); !errors.Is(err, wantErr) {
		t.Fatalf("audit error = %v, want unavailable", err)
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
	if got := recorder.record; got.MessageID != "app-2" || got.TransportMessageID != "physical-2" || got.EventID != "event-2" || got.DeliveryAttempts != 2 || got.LastError != "unknown event type: future.event" {
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
