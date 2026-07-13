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

func (s transactionalSubmissionDurableStore) CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	if sheet == nil {
		return nil, false, fmt.Errorf("answer sheet is required")
	}

	if s.runner == nil || s.writer == nil || s.stager == nil {
		return nil, false, fmt.Errorf("answersheet transactional durable store requires transaction runner, writer and event stager")
	}

	if meta.IdempotencyKey != "" {
		existing, err := s.writer.FindCompletedSubmission(ctx, meta.IdempotencyKey)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			return existing, true, nil
		}
	}

	var stagedEvents []event.DomainEvent
	if err := s.runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		events, err := s.writer.SaveSubmittedAnswerSheet(txCtx, sheet, meta)
		if err != nil {
			return err
		}
		stagedEvents = events
		if len(events) == 0 {
			return nil
		}
		return s.stager.Stage(txCtx, events...)
	}); err != nil {
		if meta.IdempotencyKey != "" {
			existing, lookupErr := s.writer.WaitForCompletedSubmission(ctx, meta.IdempotencyKey)
			if lookupErr == nil && existing != nil {
				sheet.ClearEvents()
				return existing, true, nil
			}
		}
		return nil, false, err
	}
	if s.postCommit != nil && len(stagedEvents) > 0 {
		s.postCommit.AfterCommit(ctx, stagedEvents, time.Now())
	}

	sheet.ClearEvents()
	return sheet, false, nil
}
