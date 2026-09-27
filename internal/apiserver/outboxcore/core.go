package outboxcore

import (
	"encoding/json"
	"fmt"
	"time"

	base "github.com/FangcunMount/component-base/pkg/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
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
	// The old core recognizes only component-base's DeliveryClassResolver.
	// Preserve its durable-only guard while the host catalog uses the SDK type.
	if resolver, ok := opts.Resolver.(eventcatalog.DeliveryClassResolver); ok {
		for _, evt := range opts.Events {
			if _, found := opts.Resolver.GetTopicForEvent(evt.EventType()); !found {
				continue // preserve the old core's unknown-topic error
			}
			delivery, found := resolver.GetDeliveryClass(evt.EventType())
			if !found {
				return nil, fmt.Errorf("event %q has no delivery class", evt.EventType())
			}
			if delivery != eventcatalog.DeliveryClassDurableOutbox {
				return nil, fmt.Errorf("event %q delivery class %q cannot be staged to outbox", evt.EventType(), delivery)
			}
		}
	}
	return base.BuildRecords(opts)
}

func BuildStatusSnapshot(store string, now time.Time, observations []StatusObservation) outboxport.StatusSnapshot {
	return base.BuildStatusSnapshot(store, now, observations)
}

func DecodePendingEvent(eventID, payloadJSON string) (outboxport.PendingEvent, error) {
	pending, err := base.DecodePendingEvent(eventID, payloadJSON)
	if err != nil {
		return outboxport.PendingEvent{}, err
	}
	return outboxport.PendingEvent{EventID: pending.EventID, Event: pending.Event}, nil
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
