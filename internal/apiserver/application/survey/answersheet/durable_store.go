package answersheet

import (
	"context"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
)

// DurableSubmitMeta carries submission metadata that must survive durable writes.
type DurableSubmitMeta = submitport.DurableSubmitMeta

// SubmissionDurableStore persists answersheets together with inbound idempotency
// metadata and the answersheet.submitted outbox entry.
type SubmissionDurableStore interface {
	CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error)
}

// SubmittedEventRelay keeps a compatibility name for the shared outbox relay.
type SubmittedEventRelay = appEventing.OutboxRelay

// SubmittedEventOutboxStore keeps a compatibility name for the shared outbox store.
type SubmittedEventOutboxStore = appEventing.OutboxStore
