package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
)

// DurableSubmitMeta 携带application-等级 持久化 write 元数据。
type DurableSubmitMeta = submitport.DurableSubmitMeta

// SubmissionDurableStore 持久化answersheets together 使用 inbound idempotency。
// 元数据 和 staged 领域事件。
type SubmissionDurableStore interface {
	CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error)
}

// SubmissionIdempotencyReader is an optional preflight seam. It lets the
// application return an already accepted business intent before revalidating a
// mutable source resource such as an Entry or Task.
type SubmissionIdempotencyReader interface {
	FindCompleted(ctx context.Context, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error)
}

type SubmissionDurableWriter interface {
	FindCompletedSubmission(ctx context.Context, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error)
	SaveSubmittedAnswerSheet(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) ([]event.DomainEvent, error)
	WaitForCompletedSubmission(ctx context.Context, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error)
}

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

func NewTransactionalSubmissionDurableStore(
	runner apptransaction.Runner,
	writer SubmissionDurableWriter,
	stager EventStager,
	postCommit appEventing.PostCommitDispatcher,
) SubmissionDurableStore {
	return transactionalSubmissionDurableStore{
		runner:     runner,
		writer:     writer,
		stager:     stager,
		postCommit: postCommit,
	}
}
