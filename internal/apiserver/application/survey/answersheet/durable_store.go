package answersheet

import (
	"context"
	"time"

	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// DurableSubmitMeta carries submission metadata that must survive durable writes.
type DurableSubmitMeta struct {
	IdempotencyKey string
	WriterID       uint64
	TesteeID       uint64
	OrgID          uint64
	TaskID         string
}

// SubmissionDurableStore persists answersheets together with inbound idempotency
// metadata and the answersheet.submitted outbox entry.
type SubmissionDurableStore interface {
	CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error)
}

// PendingSubmittedEvent represents a due answersheet.submitted outbox record.
type PendingSubmittedEvent struct {
	EventID string
	Event   event.DomainEvent
}

// SubmittedEventOutboxStore manages due answersheet.submitted outbox records.
type SubmittedEventOutboxStore interface {
	ClaimDueSubmittedEvents(ctx context.Context, limit int, now time.Time) ([]PendingSubmittedEvent, error)
	MarkSubmittedEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkSubmittedEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error
}
