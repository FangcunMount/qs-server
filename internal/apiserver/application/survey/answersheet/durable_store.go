package answersheet

import (
	"context"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// DurableSubmitMeta 携带application-等级 持久化 write 元数据。
type DurableSubmitMeta = submitport.DurableSubmitMeta

// SubmissionDurableStore 持久化answersheets together 使用 inbound idempotency。
// 元数据 和 staged 领域事件。
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
	immediate *appEventing.ImmediateDispatcher,
) SubmissionDurableStore {
	return transactionalSubmissionDurableStore{
		runner:    runner,
		writer:    writer,
		stager:    stager,
		immediate: immediate,
	}
}

// SubmittedEventRelay 保留兼容性 name 用于 共享 outbox relay。
type SubmittedEventRelay = appEventing.OutboxRelay

// SubmittedEventOutboxStore 保留兼容性 name 用于 共享 outbox 存储。
type SubmittedEventOutboxStore = appEventing.OutboxStore
