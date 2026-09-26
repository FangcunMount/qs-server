package systemgovernance

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
)

// ValidateDeliveryReplaySafety runs before a whole batch is claimed. Malformed
// payloads retain the existing per-item failure settlement in ActionExecutor.
func ValidateDeliveryReplaySafety(eventID, payloadJSON string) error {
	pending, err := outboxcore.DecodePendingEvent(eventID, payloadJSON)
	if err != nil {
		return nil
	}
	return deliveryReplaySafetyError(pending.Event.EventType())
}

func deliveryReplaySafetyError(eventType string) error {
	if eventType == "task.opened" {
		return fmt.Errorf("legacy task.opened needs task and recipient reconciliation; generic replay can resend an expired or duplicate external notification")
	}
	return nil
}

type DeliveryReplayTarget struct {
	ID                       uint64 `json:"id"`
	ExpectedDeliveryAttempts int    `json:"expected_delivery_attempts"`
}

type AuthorizedDelivery struct {
	ID          uint64
	MessageID   string
	EventID     string
	PayloadJSON string
}

type DeliveryReplayStore interface {
	ValidateReplayBatch(context.Context, int64, []DeliveryReplayTarget) error
	AuthorizeReplay(context.Context, int64, string, []DeliveryReplayTarget, time.Time) ([]AuthorizedDelivery, error)
	CompleteReplay(context.Context, uint64, string, time.Time) error
	FailReplay(context.Context, uint64, string, string, time.Time) error
	RecordReplayUncertain(context.Context, uint64, string, string, time.Time) error
}
