package outboxcore

import (
	"fmt"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	StatusPending    = "pending"
	StatusPublishing = "publishing"
	StatusPublished  = "published"
	StatusFailed     = "failed"

	DefaultPublishingStaleFor       = time.Minute
	DefaultRelayRetryDelay          = 10 * time.Second
	DefaultDecodeFailureRetryDelay  = 10 * time.Second
	DefaultFailedTransitionAttempts = 1
)

var unfinishedStatuses = []string{StatusPending, StatusFailed, StatusPublishing}

// UnfinishedStatuses returns the statuses used for outbox backlog and lag views.
func UnfinishedStatuses() []string {
	return append([]string(nil), unfinishedStatuses...)
}

// Record is the DB-neutral outbox representation shared by concrete stores.
type Record struct {
	EventID       string
	EventType     string
	AggregateType string
	AggregateID   string
	TopicName     string
	PayloadJSON   string
	Status        string
	AttemptCount  int
	NextAttemptAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// StatusObservation is the DB-specific aggregate input used to build a
// DB-neutral read-only outbox status snapshot.
type StatusObservation struct {
	Status          string
	Count           int64
	OldestCreatedAt *time.Time
}

type BuildRecordsOptions struct {
	Events   []event.DomainEvent
	Resolver eventcatalog.TopicResolver
	Now      time.Time
}

// BuildRecords creates pending outbox records while enforcing the delivery contract
// when the resolver can expose delivery classes.
func BuildRecords(opts BuildRecordsOptions) ([]Record, error) {
	if len(opts.Events) == 0 {
		return nil, nil
	}

	resolver := opts.Resolver
	if resolver == nil {
		resolver = eventcatalog.NewCatalog(nil)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	records := make([]Record, 0, len(opts.Events))
	for _, evt := range opts.Events {
		topicName, ok := resolver.GetTopicForEvent(evt.EventType())
		if !ok {
			return nil, fmt.Errorf("event %q not found in event config", evt.EventType())
		}
		if deliveryResolver, ok := resolver.(eventcatalog.DeliveryClassResolver); ok {
			delivery, ok := deliveryResolver.GetDeliveryClass(evt.EventType())
			if !ok {
				return nil, fmt.Errorf("event %q has no delivery class", evt.EventType())
			}
			if delivery != eventcatalog.DeliveryClassDurableOutbox {
				return nil, fmt.Errorf("event %q delivery class %q cannot be staged to outbox", evt.EventType(), delivery)
			}
		}

		payload, err := eventcodec.EncodeDomainEvent(evt)
		if err != nil {
			return nil, err
		}
		records = append(records, Record{
			EventID:       evt.EventID(),
			EventType:     evt.EventType(),
			AggregateType: evt.AggregateType(),
			AggregateID:   evt.AggregateID(),
			TopicName:     topicName,
			PayloadJSON:   string(payload),
			Status:        StatusPending,
			AttemptCount:  0,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	return records, nil
}

// BuildStatusSnapshot creates a canonical unfinished outbox status snapshot.
// Unknown statuses are ignored and missing unfinished statuses are returned with
// zero count so Prometheus gauges can be reset deterministically.
func BuildStatusSnapshot(store string, now time.Time, observations []StatusObservation) outboxport.StatusSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	byStatus := make(map[string]StatusObservation, len(observations))
	for _, observation := range observations {
		if !isUnfinishedStatus(observation.Status) {
			continue
		}
		byStatus[observation.Status] = observation
	}

	buckets := make([]outboxport.StatusBucket, 0, len(unfinishedStatuses))
	for _, status := range unfinishedStatuses {
		observation := byStatus[status]
		ageSeconds := 0.0
		if observation.Count > 0 && observation.OldestCreatedAt != nil {
			ageSeconds = now.Sub(*observation.OldestCreatedAt).Seconds()
			if ageSeconds < 0 {
				ageSeconds = 0
			}
		}
		buckets = append(buckets, outboxport.StatusBucket{
			Status:           status,
			Count:            observation.Count,
			OldestCreatedAt:  observation.OldestCreatedAt,
			OldestAgeSeconds: ageSeconds,
		})
	}
	return outboxport.StatusSnapshot{
		Store:       store,
		GeneratedAt: now,
		Buckets:     buckets,
	}
}

func isUnfinishedStatus(status string) bool {
	for _, known := range unfinishedStatuses {
		if status == known {
			return true
		}
	}
	return false
}

// DecodePendingEvent converts persisted payload JSON back into the shared pending event contract.
func DecodePendingEvent(eventID, payloadJSON string) (outboxport.PendingEvent, error) {
	evt, err := eventcodec.DecodeDomainEvent([]byte(payloadJSON))
	if err != nil {
		return outboxport.PendingEvent{}, err
	}
	return outboxport.PendingEvent{
		EventID: eventID,
		Event:   evt,
	}, nil
}

type PublishedTransition struct {
	Status      string
	PublishedAt time.Time
	UpdatedAt   time.Time
}

func NewPublishedTransition(publishedAt time.Time) PublishedTransition {
	return PublishedTransition{
		Status:      StatusPublished,
		PublishedAt: publishedAt,
		UpdatedAt:   publishedAt,
	}
}

type FailedTransition struct {
	Status           string
	LastError        string
	NextAttemptAt    time.Time
	UpdatedAt        time.Time
	AttemptIncrement int
}

func NewFailedTransition(lastError string, nextAttemptAt, updatedAt time.Time) FailedTransition {
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	return FailedTransition{
		Status:           StatusFailed,
		LastError:        lastError,
		NextAttemptAt:    nextAttemptAt,
		UpdatedAt:        updatedAt,
		AttemptIncrement: DefaultFailedTransitionAttempts,
	}
}

func NewDecodeFailureTransition(decodeErr error, now time.Time) FailedTransition {
	if now.IsZero() {
		now = time.Now()
	}
	return NewFailedTransition(
		fmt.Sprintf("decode outbox payload: %v", decodeErr),
		now.Add(DefaultDecodeFailureRetryDelay),
		now,
	)
}
