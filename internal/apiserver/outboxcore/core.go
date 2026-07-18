package outboxcore

import (
	"encoding/json"
	"time"

	base "github.com/FangcunMount/component-base/pkg/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

// OrgIDFromPayloadJSON extracts the optional organization scope from the
// canonical event envelope without coupling Outbox storage to event payloads.
func OrgIDFromPayloadJSON(payload string) *int64 {
	var envelope struct {
		Data struct {
			OrgID int64 `json:"org_id"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(payload), &envelope) != nil || envelope.Data.OrgID == 0 {
		return nil
	}
	orgID := envelope.Data.OrgID
	return &orgID
}

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
