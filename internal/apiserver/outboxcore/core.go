package outboxcore

import (
	"encoding/json"
	"fmt"
	"time"

	baseevent "github.com/FangcunMount/component-base/pkg/event"
	base "github.com/FangcunMount/component-base/pkg/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
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

// BuildRecordsOptions keeps QS domain-event interfaces outside the legacy
// component-base core. The adapter below is retired with the old Outbox path.
type BuildRecordsOptions struct {
	Events   []event.DomainEvent
	Resolver eventcatalog.TopicResolver
	Encoder  func(event.DomainEvent) ([]byte, error)
	Now      time.Time
}
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
	legacyEvents := make([]baseevent.DomainEvent, len(opts.Events))
	for i, evt := range opts.Events {
		legacyEvents[i] = evt
	}
	legacyOpts := base.BuildRecordsOptions{
		Events:   legacyEvents,
		Resolver: opts.Resolver,
		Now:      opts.Now,
	}
	if opts.Encoder != nil {
		legacyOpts.Encoder = func(evt baseevent.DomainEvent) ([]byte, error) {
			return opts.Encoder(evt)
		}
	}
	return base.BuildRecords(legacyOpts)
}

func BuildStatusSnapshot(store string, now time.Time, observations []StatusObservation) outboxport.StatusSnapshot {
	legacy := base.BuildStatusSnapshot(store, now, observations)
	buckets := make([]outboxport.StatusBucket, len(legacy.Buckets))
	for i, bucket := range legacy.Buckets {
		buckets[i] = outboxport.StatusBucket{
			Status: bucket.Status, Count: bucket.Count,
			OldestCreatedAt: bucket.OldestCreatedAt, OldestAgeSeconds: bucket.OldestAgeSeconds,
		}
	}
	return outboxport.StatusSnapshot{Store: legacy.Store, GeneratedAt: legacy.GeneratedAt, Buckets: buckets}
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
