package systemgovernance

import (
	"context"
	"time"
)

// ReminderReview is an organization-scoped, read-only responsibility. A
// sending or manual_required state is not permission to send again.
type ReminderReview struct {
	DeliveryID            uint64     `json:"delivery_id"`
	TaskID                string     `json:"task_id"`
	OpeningEventID        string     `json:"opening_event_id"`
	ScheduleRevision      uint32     `json:"schedule_revision"`
	UserID                string     `json:"user_id"`
	State                 string     `json:"state"`
	ExternalCallStartedAt *time.Time `json:"external_call_started_at,omitempty"`
	ResolutionCode        string     `json:"resolution_code,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ReminderReviewPage struct {
	Items      []ReminderReview `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type ReminderReviewReader interface {
	ListReminderReviews(context.Context, int64, time.Time, string, int) (ReminderReviewPage, error)
}
