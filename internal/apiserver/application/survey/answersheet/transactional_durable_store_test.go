package answersheet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type durableStoreTxMarkerKey struct{}

type durableStoreRunnerStub struct {
	called bool
	err    error
}

func (r *durableStoreRunnerStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	txCtx := context.WithValue(ctx, durableStoreTxMarkerKey{}, "tx")
	if err := fn(txCtx); err != nil {
		return err
	}
	return r.err
}

type durableStoreWriterStub struct {
	existing     *domainAnswerSheet.AnswerSheet
	waitExisting *domainAnswerSheet.AnswerSheet
	saveEvents   []event.DomainEvent
	saveErr      error
	findCalled   bool
	saveCalled   bool
	waitCalled   bool
	saveSawTxCtx bool
	waitKey      string
}

func (w *durableStoreWriterStub) FindCompletedSubmission(_ context.Context, _ string) (*domainAnswerSheet.AnswerSheet, error) {
	w.findCalled = true
	return w.existing, nil
}

func (w *durableStoreWriterStub) SaveSubmittedAnswerSheet(ctx context.Context, _ *domainAnswerSheet.AnswerSheet, _ DurableSubmitMeta) ([]event.DomainEvent, error) {
	w.saveCalled = true
	w.saveSawTxCtx = ctx.Value(durableStoreTxMarkerKey{}) == "tx"
	return w.saveEvents, w.saveErr
}

func (w *durableStoreWriterStub) WaitForCompletedSubmission(_ context.Context, key string) (*domainAnswerSheet.AnswerSheet, error) {
	w.waitCalled = true
	w.waitKey = key
	return w.waitExisting, nil
}

type durableStoreStagerStub struct {
	err        error
	called     bool
	sawTxCtx   bool
	eventTypes []string
}

func (s *durableStoreStagerStub) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.called = true
	s.sawTxCtx = ctx.Value(durableStoreTxMarkerKey{}) == "tx"
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return s.err
}

func TestTransactionalSubmissionDurableStoreIdempotencyHitDoesNotOpenTransaction(t *testing.T) {
	existing := newDurableStoreTestSheet(t)
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{existing: existing}
	stager := &durableStoreStagerStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager)

	got, existed, err := store.CreateDurably(t.Context(), newDurableStoreTestSheet(t), DurableSubmitMeta{IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatalf("CreateDurably() error = %v", err)
	}
	if !existed || got != existing {
		t.Fatalf("CreateDurably() = (%p, %v), want existing sheet", got, existed)
	}
	if runner.called || writer.saveCalled || stager.called {
		t.Fatalf("idempotency hit should not open write transaction: runner=%v save=%v stage=%v", runner.called, writer.saveCalled, stager.called)
	}
}

func TestTransactionalSubmissionDurableStoreStagesInTransactionContext(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	sheet.RaiseSubmittedEvent(1, 2, "task-1")
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{
		saveEvents: []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{"id": "1"})},
	}
	stager := &durableStoreStagerStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager)

	got, existed, err := store.CreateDurably(t.Context(), sheet, DurableSubmitMeta{})
	if err != nil {
		t.Fatalf("CreateDurably() error = %v", err)
	}
	if existed || got != sheet {
		t.Fatalf("CreateDurably() = (%p, %v), want stored sheet", got, existed)
	}
	if !writer.saveSawTxCtx || !stager.sawTxCtx {
		t.Fatalf("writer/stager must receive transaction context: writer=%v stager=%v", writer.saveSawTxCtx, stager.sawTxCtx)
	}
	if len(sheet.Events()) != 0 {
		t.Fatalf("events were not cleared after successful durable save")
	}
}

func TestTransactionalSubmissionDurableStoreStageFailureDoesNotClearEvents(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	sheet.RaiseSubmittedEvent(1, 2, "task-1")
	stageErr := errors.New("stage failed")
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{
		saveEvents: []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{})},
	}
	stager := &durableStoreStagerStub{err: stageErr}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager)

	_, existed, err := store.CreateDurably(t.Context(), sheet, DurableSubmitMeta{})
	if !errors.Is(err, stageErr) {
		t.Fatalf("CreateDurably() error = %v, want %v", err, stageErr)
	}
	if existed {
		t.Fatalf("stage failure should not be reported as idempotency hit")
	}
	if len(sheet.Events()) == 0 {
		t.Fatalf("events should remain on stage failure")
	}
}

func TestTransactionalSubmissionDurableStoreFailureCanReturnCompletedIdempotentResult(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	sheet.RaiseSubmittedEvent(1, 2, "task-1")
	existing := newDurableStoreTestSheet(t)
	stageErr := errors.New("stage failed")
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{
		waitExisting: existing,
		saveEvents:   []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{})},
	}
	stager := &durableStoreStagerStub{err: stageErr}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager)

	got, existed, err := store.CreateDurably(t.Context(), sheet, DurableSubmitMeta{IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatalf("CreateDurably() error = %v", err)
	}
	if !existed || got != existing {
		t.Fatalf("CreateDurably() = (%p, %v), want completed idempotent result", got, existed)
	}
	if !writer.waitCalled || writer.waitKey != "idem-1" {
		t.Fatalf("WaitForCompletedSubmission was not called with idempotency key")
	}
	if len(sheet.Events()) != 0 {
		t.Fatalf("events should be cleared when idempotent result is returned")
	}
}

func newDurableStoreTestSheet(t *testing.T) *domainAnswerSheet.AnswerSheet {
	t.Helper()
	sheet := domainAnswerSheet.Reconstruct(
		meta.FromUint64(1),
		domainAnswerSheet.NewQuestionnaireRef("QNR-1", "1.0.0", "Questionnaire"),
		actor.NewFillerRef(301, actor.FillerTypeSelf),
		mustAnswersForSubmissionTest(t),
		time.Unix(1, 0),
		0,
	)
	return sheet
}
