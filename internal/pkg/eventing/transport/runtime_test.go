package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
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
	wantErr := errors.New("decode failed")
	if err := handler(t.Context(), basemessaging.FailedMessage{
		Provider: "nsq", Topic: "evaluation", Channel: "worker", Message: message, Attempts: 8, Cause: wantErr,
	}); err != nil {
		t.Fatal(err)
	}
	if recorder.record.MessageID != "message-1" || recorder.record.EventID != "event-1" || recorder.record.OrgID == nil || *recorder.record.OrgID != 7 || recorder.record.DeliveryAttempts != 8 || recorder.record.LastError != wantErr.Error() {
		t.Fatalf("record = %#v", recorder.record)
	}
}
