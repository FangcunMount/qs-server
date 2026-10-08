package answersheet

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
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

func (s transactionalSubmissionDurableStore) FindCompleted(ctx context.Context, meta DurableSubmitMeta) (*CompletedSubmission, error) {
	if s.writer == nil || meta.IdempotencyKey == "" {
		return nil, nil
	}
	completed, err := s.writer.FindCompletedSubmission(ctx, meta)
	if err == nil {
		err = validateCompletedSubmission(completed)
	}
	return completed, err
}

func (s transactionalSubmissionDurableStore) CreateDurably(ctx context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	if sheet == nil {
		return nil, false, fmt.Errorf("answer sheet is required")
	}

	if s.runner == nil || s.writer == nil || s.stager == nil {
		return nil, false, fmt.Errorf("answersheet transactional durable store requires transaction runner, writer and event stager")
	}

	if meta.IdempotencyKey != "" {
		completed, err := s.writer.FindCompletedSubmission(ctx, meta)
		if err == nil {
			err = validateCompletedSubmission(completed)
		}
		observeDurableLookupOperation("pretransaction_lookup", completed, err)
		if err != nil {
			return nil, false, err
		}
		if completed != nil {
			return completed.Sheet, true, nil
		}
	}

	preparer, ok := s.stager.(outboxport.ReferencePreparer)
	if !ok {
		return nil, false, fmt.Errorf("answersheet durable store requires original event reference preparation")
	}
	for _, evt := range sheet.Events() {
		var originalRequestID string
		switch submitted := evt.(type) {
		case domainAnswerSheet.AnswerSheetSubmittedEvent:
			originalRequestID = submitted.Data.RequestID
		case *domainAnswerSheet.AnswerSheetSubmittedEvent:
			if submitted != nil {
				originalRequestID = submitted.Data.RequestID
			}
		}
		if originalRequestID != "" && meta.RequestID != "" && originalRequestID != meta.RequestID {
			return nil, false, fmt.Errorf("answersheet request identity conflict")
		}
	}
	stagedEvents := withSubmissionRequestID(append([]event.DomainEvent(nil), sheet.Events()...), meta.RequestID)
	if len(stagedEvents) != 1 || stagedEvents[0] == nil {
		return nil, false, fmt.Errorf("answersheet requires one frozen submitted event")
	}
	submitted, ok := stagedEvents[0].(domainAnswerSheet.AnswerSheetSubmittedEvent)
	if !ok {
		return nil, false, fmt.Errorf("answersheet submitted event payload is invalid")
	}
	binding, err := eventevidencebinding.AnswerSheet(submitted.Data)
	if err != nil {
		return nil, false, err
	}
	ref, err := preparer.PrepareReference(submitted)
	if err != nil {
		return nil, false, err
	}
	proof, err := evidence.NewStandard(ref, binding)
	if err != nil {
		return nil, false, err
	}
	transactionStarted := time.Now()
	if err := s.runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		events, err := s.writer.SaveSubmittedAnswerSheet(txCtx, sheet, meta, proof)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0] == nil || events[0].EventID() != proof.EventID {
			return fmt.Errorf("answersheet durable writer did not preserve original submitted event")
		}
		outboxStarted := time.Now()
		err = s.stager.Stage(txCtx, stagedEvents...)
		if err != nil {
			observeDurableStage("outbox_stage", "failed", outboxStarted)
			return err
		}
		observeDurableStage("outbox_stage", "ok", outboxStarted)
		return nil
	}); err != nil {
		observeDurableOperation("transaction", "failed")
		observeDurableStage("mongo_transaction", "failed", transactionStarted)
		if meta.IdempotencyKey != "" {
			// A Mongo commit result may be unknown precisely because the request
			// context was canceled. Use a short detached read-only recovery window;
			// never acknowledge 202 unless the completed row is actually found.
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), durableSubmitRecoveryTimeout)
			existing, lookupErr := s.writer.WaitForCompletedSubmission(recoveryCtx, meta)
			cancel()
			if lookupErr != nil && stderrors.Is(lookupErr, submitport.ErrIdempotencyConflict) {
				observeDurableOperation("transaction_error_recovery", "conflict")
				return nil, false, lookupErr
			}
			if lookupErr == nil && existing != nil {
				observeDurableOperation("transaction_error_recovery", "hit")
				sheet.ClearEvents()
				return existing, true, nil
			}
			if lookupErr != nil {
				observeDurableOperation("transaction_error_recovery", "error")
			} else {
				observeDurableOperation("transaction_error_recovery", "miss")
			}
		}
		return nil, false, err
	}
	observeDurableOperation("transaction", "committed")
	observeDurableStage("mongo_transaction", "ok", transactionStarted)
	if s.postCommit != nil && len(stagedEvents) > 0 {
		s.postCommit.AfterCommit(ctx, stagedEvents, time.Now())
	}

	sheet.ClearEvents()
	return sheet, false, nil
}

func observeDurableLookupOperation(operation string, completed *CompletedSubmission, err error) {
	switch {
	case stderrors.Is(err, submitport.ErrIdempotencyConflict):
		observeDurableOperation(operation, "conflict")
	case err != nil:
		observeDurableOperation(operation, "error")
	case completed != nil:
		observeDurableOperation(operation, "hit")
	default:
		observeDurableOperation(operation, "miss")
	}
}

func withSubmissionRequestID(events []event.DomainEvent, requestID string) []event.DomainEvent {
	enriched := append([]event.DomainEvent(nil), events...)
	for index, evt := range enriched {
		var submitted domainAnswerSheet.AnswerSheetSubmittedEvent
		switch value := evt.(type) {
		case domainAnswerSheet.AnswerSheetSubmittedEvent:
			submitted = value
		case *domainAnswerSheet.AnswerSheetSubmittedEvent:
			if value == nil {
				continue
			}
			submitted = *value
		default:
			continue
		}
		if requestID != "" {
			submitted.Data.RequestID = requestID
		}
		if submitted.Data.Admission != nil {
			copy := *submitted.Data.Admission
			submitted.Data.Admission = &copy
		}
		if submitted.Data.Attribution != nil {
			copy := *submitted.Data.Attribution
			submitted.Data.Attribution = &copy
		}
		enriched[index] = submitted
	}
	return enriched
}
