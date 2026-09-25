package notification

import (
	"context"
	"time"
)

// ReminderDeliveryKey identifies one external notification responsibility.
// The OpenID and bearer entry URL must never be stored in this key or ledger.
type ReminderDeliveryKey struct {
	OrgID            int64
	TaskID           string
	OpeningEventID   string
	ScheduleRevision uint32
	LoginIdentityID  string
	AppID            string
	TemplateID       string
	ReminderVersion  uint32
}

type ReminderDeliveryState string

const (
	ReminderPending        ReminderDeliveryState = "pending"
	ReminderClaimed        ReminderDeliveryState = "claimed"
	ReminderSending        ReminderDeliveryState = "sending"
	ReminderConfirmed      ReminderDeliveryState = "confirmed"
	ReminderManualRequired ReminderDeliveryState = "manual_required"
	ReminderSuppressed     ReminderDeliveryState = "suppressed"
)

// ReminderDelivery records platform acceptance, not recipient reading.
// A sending row has an unknown external outcome after process loss and must
// never be reclaimed automatically.
type ReminderDelivery struct {
	Key                   ReminderDeliveryKey
	UserID                string
	State                 ReminderDeliveryState
	ClaimToken            string
	LeaseUntil            *time.Time
	ExternalCallStartedAt *time.Time
	PlatformMessageID     string
	ResolutionCode        string
	UpdatedAt             time.Time
}

// ReminderDeliveryLedger only owns local responsibility and results. Product
// relationship selection, Task validity, and the external send stay outside.
type ReminderDeliveryLedger interface {
	EnsurePending(ctx context.Context, key ReminderDeliveryKey, userID string, now time.Time) (ReminderDelivery, error)
	Claim(ctx context.Context, key ReminderDeliveryKey, lease time.Duration, now time.Time) (token string, claimed bool, err error)
	BeginExternalCall(ctx context.Context, key ReminderDeliveryKey, token string, now time.Time) (bool, error)
	Confirm(ctx context.Context, key ReminderDeliveryKey, token, platformMessageID string, now time.Time) (bool, error)
	MarkUnknown(ctx context.Context, key ReminderDeliveryKey, token, resolutionCode string, now time.Time) (bool, error)
	ReleaseUnsent(ctx context.Context, key ReminderDeliveryKey, token string, now time.Time) (bool, error)
	SuppressUnsent(ctx context.Context, key ReminderDeliveryKey, token, resolutionCode string, now time.Time) (bool, error)
	Read(ctx context.Context, key ReminderDeliveryKey) (ReminderDelivery, error)
	ListNeedsReview(ctx context.Context, orgID int64, sendingOlderThan time.Time, limit int) ([]ReminderDelivery, error)
}
