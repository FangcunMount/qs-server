package systemgovernance

import (
	"context"
	"time"
)

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
	AuthorizeReplay(context.Context, int64, string, []DeliveryReplayTarget, time.Time) ([]AuthorizedDelivery, error)
	CompleteReplay(context.Context, uint64, string, time.Time) error
	FailReplay(context.Context, uint64, string, string, time.Time) error
}
