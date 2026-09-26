package notification

import (
	"context"
	"time"
)

// ReminderBatchKey identifies one opening's complete recipient snapshot.
// It intentionally excludes AppID and template, which are frozen on first use.
type ReminderBatchKey struct {
	OrgID            int64
	TaskID           string
	OpeningEventID   string
	ScheduleRevision uint32
	ReminderVersion  uint32
}

type ReminderRecipientIdentity struct {
	UserID          string
	LoginIdentityID string
}

type ReminderBatch struct {
	Key            ReminderBatchKey
	AppID          string
	TemplateID     string
	ResolutionCode string
	Suppressed     bool
	Recipients     []ReminderRecipientIdentity
}

// ReminderBatchLedger freezes a complete identity set before any external
// call. Redelivery always returns the original set, even if IAM changes.
type ReminderBatchLedger interface {
	FreezeRecipients(ctx context.Context, key ReminderBatchKey, appID, templateID string, recipients []ReminderRecipientIdentity, now time.Time) (ReminderBatch, error)
	SuppressEmpty(ctx context.Context, key ReminderBatchKey, appID, templateID, code string, now time.Time) (ReminderBatch, error)
	FindBatch(ctx context.Context, key ReminderBatchKey) (ReminderBatch, bool, error)
	ReadBatch(ctx context.Context, key ReminderBatchKey) (ReminderBatch, error)
}
