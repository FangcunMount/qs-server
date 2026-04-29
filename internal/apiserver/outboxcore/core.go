package outboxcore

import (
	"time"

	base "github.com/FangcunMount/component-base/pkg/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

const (
	StatusPending    = base.StatusPending
	StatusPublishing = base.StatusPublishing
	StatusPublished  = base.StatusPublished
	StatusFailed     = base.StatusFailed

	DefaultPublishingStaleFor       = base.DefaultPublishingStaleFor
	DefaultRelayRetryDelay          = base.DefaultRelayRetryDelay
	DefaultDecodeFailureRetryDelay  = base.DefaultDecodeFailureRetryDelay
	DefaultFailedTransitionAttempts = base.DefaultFailedTransitionAttempts
)

type Record = base.Record
type StatusObservation = base.StatusObservation
type BuildRecordsOptions = base.BuildRecordsOptions
type PublishedTransition = base.PublishedTransition
type FailedTransition = base.FailedTransition

func UnfinishedStatuses() []string {
	return base.UnfinishedStatuses()
}

func BuildRecords(opts BuildRecordsOptions) ([]Record, error) {
	return base.BuildRecords(opts)
}

func BuildStatusSnapshot(store string, now time.Time, observations []StatusObservation) outboxport.StatusSnapshot {
	return base.BuildStatusSnapshot(store, now, observations)
}

func DecodePendingEvent(eventID, payloadJSON string) (outboxport.PendingEvent, error) {
	return base.DecodePendingEvent(eventID, payloadJSON)
}

func NewPublishedTransition(publishedAt time.Time) PublishedTransition {
	return base.NewPublishedTransition(publishedAt)
}

func NewFailedTransition(lastError string, nextAttemptAt, updatedAt time.Time) FailedTransition {
	return base.NewFailedTransition(lastError, nextAttemptAt, updatedAt)
}

func NewDecodeFailureTransition(decodeErr error, now time.Time) FailedTransition {
	return base.NewDecodeFailureTransition(decodeErr, now)
}
