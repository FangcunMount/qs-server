package answersheet

import (
	"context"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// DurableSubmitMeta carries submission metadata that must survive durable writes.
type DurableSubmitMeta = submitport.DurableSubmitMeta

// SubmissionDurableStore persists answersheets together with inbound idempotency
// metadata and the answersheet.submitted outbox entry.
type SubmissionDurableStore interface {
	CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error)
}

type SubmissionDurableWriter interface {
	FindCompletedSubmission(ctx context.Context, idempotencyKey string) (*domainAnswerSheet.AnswerSheet, error)
	SaveSubmittedAnswerSheet(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) ([]event.DomainEvent, error)
	WaitForCompletedSubmission(ctx context.Context, idempotencyKey string) (*domainAnswerSheet.AnswerSheet, error)
}

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

func NewTransactionalSubmissionDurableStore(
	runner apptransaction.Runner,
	writer SubmissionDurableWriter,
	stager EventStager,
) SubmissionDurableStore {
	return transactionalSubmissionDurableStore{
		runner: runner,
		writer: writer,
		stager: stager,
	}
}

// SubmittedEventRelay keeps a compatibility name for the shared outbox relay.
type SubmittedEventRelay = appEventing.OutboxRelay

// SubmittedEventOutboxStore keeps a compatibility name for the shared outbox store.
type SubmittedEventOutboxStore = appEventing.OutboxStore
