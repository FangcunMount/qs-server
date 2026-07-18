package answersheet

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

type transactionalSubmissionDurableStore struct {
	runner interface {
		WithinTransaction(context.Context, func(context.Context) error) error
	}
	writer     SubmissionDurableWriter
	stager     EventStager
	postCommit appEventing.PostCommitDispatcher
}

const durableSubmitRecoveryTimeout = 500 * time.Millisecond

func (s transactionalSubmissionDurableStore) CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	if sheet == nil {
		return nil, false, fmt.Errorf("answer sheet is required")
	}

	if s.runner == nil || s.writer == nil || s.stager == nil {
		return nil, false, fmt.Errorf("answersheet transactional durable store requires transaction runner, writer and event stager")
	}

	if meta.IdempotencyKey != "" {
		existing, err := s.writer.FindCompletedSubmission(ctx, meta)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			return existing, true, nil
		}
	}

	var stagedEvents []event.DomainEvent
	transactionStarted := time.Now()
	if err := s.runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		events, err := s.writer.SaveSubmittedAnswerSheet(txCtx, sheet, meta)
		if err != nil {
			return err
		}
		stagedEvents = events
		if len(events) == 0 {
			return nil
		}
		outboxStarted := time.Now()
		err = s.stager.Stage(txCtx, events...)
		if err != nil {
			observeDurableStage("outbox_stage", "failed", outboxStarted)
			return err
		}
		observeDurableStage("outbox_stage", "ok", outboxStarted)
		return nil
	}); err != nil {
		observeDurableStage("mongo_transaction", "failed", transactionStarted)
		if meta.IdempotencyKey != "" {
			// A Mongo commit result may be unknown precisely because the request
			// context was canceled. Use a short detached read-only recovery window;
			// never acknowledge 202 unless the completed row is actually found.
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), durableSubmitRecoveryTimeout)
			existing, lookupErr := s.writer.WaitForCompletedSubmission(recoveryCtx, meta)
			cancel()
			if lookupErr == nil && existing != nil {
				sheet.ClearEvents()
				return existing, true, nil
			}
		}
		return nil, false, err
	}
	observeDurableStage("mongo_transaction", "ok", transactionStarted)
	if s.postCommit != nil && len(stagedEvents) > 0 {
		s.postCommit.AfterCommit(ctx, stagedEvents, time.Now())
	}

	sheet.ClearEvents()
	return sheet, false, nil
}
