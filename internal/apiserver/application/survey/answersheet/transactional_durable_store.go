package answersheet

import (
	"context"
	"fmt"

	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

type transactionalSubmissionDurableStore struct {
	runner interface {
		WithinTransaction(context.Context, func(context.Context) error) error
	}
	writer SubmissionDurableWriter
	stager EventStager
}

func (s transactionalSubmissionDurableStore) CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	if sheet == nil {
		return nil, false, nil
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

	if err := s.runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		events, err := s.writer.SaveSubmittedAnswerSheet(txCtx, sheet, meta)
		if err != nil {
			return err
		}
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

	sheet.ClearEvents()
	return sheet, false, nil
}
