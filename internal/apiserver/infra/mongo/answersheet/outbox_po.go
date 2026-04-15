package answersheet

import (
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	outboxStatusPending    = "pending"
	outboxStatusPublishing = "publishing"
	outboxStatusPublished  = "published"
	outboxStatusFailed     = "failed"
)

type storedDomainEvent struct {
	event.BaseEvent
	Data json.RawMessage `json:"data"`
}

// AnswerSheetSubmittedOutboxPO stores answersheet.submitted events until they
// are durably published.
type AnswerSheetSubmittedOutboxPO struct {
	EventID       string    `bson:"event_id"`
	EventType     string    `bson:"event_type"`
	AggregateType string    `bson:"aggregate_type"`
	AggregateID   string    `bson:"aggregate_id"`
	TopicName     string    `bson:"topic_name"`
	PayloadJSON   string    `bson:"payload_json"`
	Status        string    `bson:"status"`
	AttemptCount  int       `bson:"attempt_count"`
	NextAttemptAt time.Time `bson:"next_attempt_at"`
	LastError     string    `bson:"last_error,omitempty"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
	PublishedAt   time.Time `bson:"published_at,omitempty"`
}

func (AnswerSheetSubmittedOutboxPO) CollectionName() string {
	return "domain_event_outbox"
}

func (p *AnswerSheetSubmittedOutboxPO) ToEvent() (event.DomainEvent, error) {
	if p == nil {
		return nil, nil
	}

	var evt storedDomainEvent
	if err := json.Unmarshal([]byte(p.PayloadJSON), &evt); err != nil {
		return nil, err
	}
	return evt, nil
}
