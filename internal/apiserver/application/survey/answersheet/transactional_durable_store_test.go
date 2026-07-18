package answersheet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
	findMeta     DurableSubmitMeta
	waitMeta     DurableSubmitMeta
	waitCtxErr   error
}

func (w *durableStoreWriterStub) FindCompletedSubmission(_ context.Context, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	w.findCalled = true
	w.findMeta = meta
	return w.existing, nil
}

func (w *durableStoreWriterStub) SaveSubmittedAnswerSheet(ctx context.Context, _ *domainAnswerSheet.AnswerSheet, _ DurableSubmitMeta) ([]event.DomainEvent, error) {
	w.saveCalled = true
	w.saveSawTxCtx = ctx.Value(durableStoreTxMarkerKey{}) == "tx"
	return w.saveEvents, w.saveErr
}

func (w *durableStoreWriterStub) WaitForCompletedSubmission(ctx context.Context, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	w.waitCalled = true
	w.waitCtxErr = ctx.Err()
	w.waitKey = meta.IdempotencyKey
	w.waitMeta = meta
	return w.waitExisting, nil
}

func TestTransactionalSubmissionDurableStoreUnknownCommitRecoversAfterRequestCancellation(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	existing := newDurableStoreTestSheet(t)
	runner := &durableStoreRunnerStub{err: context.Canceled}
	writer := &durableStoreWriterStub{waitExisting: existing}
	store := NewTransactionalSubmissionDurableStore(runner, writer, &durableStoreStagerStub{}, nil)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, existed, err := store.CreateDurably(ctx, sheet, DurableSubmitMeta{IdempotencyKey: "idem-unknown-commit"})
	if err != nil {
		t.Fatalf("CreateDurably() error = %v", err)
	}
	if !existed || got != existing {
		t.Fatalf("CreateDurably() = (%p, %v), want recovered existing sheet", got, existed)
	}
	if !writer.waitCalled || writer.waitCtxErr != nil {
		t.Fatalf("recovery lookup context error = %v, want detached live context", writer.waitCtxErr)
	}
}

type durableStoreStagerStub struct {
	err        error
	called     bool
	sawTxCtx   bool
	eventTypes []string
}

type durableStorePostCommitStub struct {
	calls      int
	eventTypes []string
}

func (s *durableStorePostCommitStub) AfterCommit(_ context.Context, events []event.DomainEvent, _ time.Time) {
	s.calls++
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
}

func (s *durableStoreStagerStub) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.called = true
	s.sawTxCtx = ctx.Value(durableStoreTxMarkerKey{}) == "tx"
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return s.err
}

func TestTransactionalSubmissionDurableStoreRequiresAnswerSheet(t *testing.T) {
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{}
	stager := &durableStoreStagerStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager, nil)

	_, existed, err := store.CreateDurably(t.Context(), nil, DurableSubmitMeta{})
	if err == nil {
		t.Fatal("CreateDurably() error = nil, want answer sheet required error")
	}
	if existed {
		t.Fatalf("CreateDurably() existed = true, want false")
	}
	if runner.called || writer.findCalled || writer.saveCalled || stager.called {
		t.Fatalf("nil sheet should not touch collaborators: runner=%v find=%v save=%v stage=%v", runner.called, writer.findCalled, writer.saveCalled, stager.called)
	}
}

func TestTransactionalSubmissionDurableStoreIdempotencyHitDoesNotOpenTransaction(t *testing.T) {
	existing := newDurableStoreTestSheet(t)
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{existing: existing}
	stager := &durableStoreStagerStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager, nil)

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
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{
		saveEvents: []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{"id": "1"})},
	}
	stager := &durableStoreStagerStub{}
	postCommit := &durableStorePostCommitStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager, postCommit)

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
	if postCommit.calls != 1 || len(postCommit.eventTypes) != 1 {
		t.Fatalf("post-commit calls=%d event types=%v, want one notification", postCommit.calls, postCommit.eventTypes)
	}
}

func TestTransactionalSubmissionDurableStoreStageFailureDoesNotClearEvents(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	stageErr := errors.New("stage failed")
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{
		saveEvents: []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{})},
	}
	stager := &durableStoreStagerStub{err: stageErr}
	postCommit := &durableStorePostCommitStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager, postCommit)

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
	if postCommit.calls != 0 {
		t.Fatalf("rollback notified post-commit %d times", postCommit.calls)
	}
}

func TestTransactionalSubmissionDurableStoreCommitFailureDoesNotAcknowledgeOrClearEvents(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	commitErr := errors.New("commit failed")
	runner := &durableStoreRunnerStub{err: commitErr}
	writer := &durableStoreWriterStub{
		saveEvents: []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{})},
	}
	stager := &durableStoreStagerStub{}
	postCommit := &durableStorePostCommitStub{}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager, postCommit)

	_, existed, err := store.CreateDurably(t.Context(), sheet, DurableSubmitMeta{})
	if !errors.Is(err, commitErr) {
		t.Fatalf("CreateDurably() error = %v, want %v", err, commitErr)
	}
	if existed {
		t.Fatal("commit failure must not be acknowledged as an existing result")
	}
	if len(sheet.Events()) == 0 {
		t.Fatal("events must remain when commit is not confirmed")
	}
	if postCommit.calls != 0 {
		t.Fatalf("commit failure notified post-commit %d times", postCommit.calls)
	}
}

func TestTransactionalSubmissionDurableStoreFailureCanReturnCompletedIdempotentResult(t *testing.T) {
	sheet := newDurableStoreTestSheet(t)
	existing := newDurableStoreTestSheet(t)
	stageErr := errors.New("stage failed")
	runner := &durableStoreRunnerStub{}
	writer := &durableStoreWriterStub{
		waitExisting: existing,
		saveEvents:   []event.DomainEvent{event.New("survey.answersheet.submitted", "AnswerSheet", "1", map[string]string{})},
	}
	stager := &durableStoreStagerStub{err: stageErr}
	store := NewTransactionalSubmissionDurableStore(runner, writer, stager, nil)

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
	sheet, err := domainAnswerSheet.Submit(
		meta.FromUint64(1),
		mustQuestionnaireRefForSubmissionTest(t),
		mustSubmissionContextForSubmissionTest(t),
		mustAnswersForSubmissionTest(t),
		time.Unix(1, 0),
	)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	return sheet
}

func mustQuestionnaireRefForSubmissionTest(t *testing.T) domainAnswerSheet.QuestionnaireRef {
	t.Helper()
	ref, err := domainAnswerSheet.NewQuestionnaireRef("QNR-1", "1.0.0", "Questionnaire")
	if err != nil {
		t.Fatalf("NewQuestionnaireRef() error = %v", err)
	}
	return ref
}

func mustSubmissionContextForSubmissionTest(t *testing.T) domainAnswerSheet.SubmissionContext {
	t.Helper()
	ctx, err := domainAnswerSheet.NewSubmissionContext(
		actor.NewFillerRef(301, actor.FillerTypeSelf),
		actor.NewTesteeRef(meta.FromUint64(401)),
		meta.FromUint64(501),
		"task-1",
	)
	if err != nil {
		t.Fatalf("NewSubmissionContext() error = %v", err)
	}
	return ctx
}
