package transport

import (
	"context"
	"fmt"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
)

// These adapters exercise historical message semantics in existing tests. The
// production Worker uses SDK deliveries and does not expose either adapter.
func FailedMessageHandler(recorder DeadLetterRecorder) basemessaging.FailedMessageHandler {
	return func(ctx context.Context, failed basemessaging.FailedMessage) error {
		if recorder == nil || failed.Message == nil {
			return fmt.Errorf("dead-letter audit store is not configured")
		}
		lastError := "transport delivery exhausted"
		if failed.Cause != nil {
			lastError = failed.Cause.Error()
		}
		return recorder.RecordDeadLetter(ctx, deadLetterRecord(
			failed.Provider, failed.Topic, failed.Channel, failed.Attempts,
			failed.Message.UUID, failed.Message.TransportMessageID, failed.Message.Payload, lastError,
		))
	}
}

// NewUnknownEventRecorder preserves an unsupported historical event before
// acknowledgement. A failed database write leaves the test delivery unsettled.
func NewUnknownEventRecorder(provider string, recorder DeadLetterRecorder) func(context.Context, *basemessaging.Message, string) error {
	return func(ctx context.Context, msg *basemessaging.Message, eventType string) error {
		if recorder == nil || msg == nil || provider == "" || eventType == "" {
			return fmt.Errorf("unknown-event audit store or identity is not configured")
		}
		return recorder.RecordDeadLetter(ctx, deadLetterRecord(
			provider, msg.Topic, msg.Channel, max(int(msg.Attempts), 1), msg.UUID, msg.TransportMessageID, msg.Payload,
			"unknown event type: "+eventType,
		))
	}
}
