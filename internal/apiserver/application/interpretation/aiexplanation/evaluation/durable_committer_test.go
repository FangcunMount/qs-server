package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	evaluationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestDurableCommitterBindsRunAndEveryAttemptToNextEvent(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	repository := &evidenceRepositoryStub{}
	stager := &evaluationEventStagerStub{}
	postCommit := &evaluationPostCommitStub{}
	capacity := &capacityRepositoryStub{}
	committer, err := NewDurableCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			stager.insideTransaction = true
			capacity.insideTransaction = true
			defer func() {
				stager.insideTransaction = false
				capacity.insideTransaction = false
			}()
			return fn(ctx)
		}),
		repository, evaluationevents.Factory{}, stager, postCommit, capacity, 140, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	runRecord, err := domainevaluation.NewRequested(meta.ID(601), evidenceRelease(), 12, "user:42", "release evaluation", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: "preflight", Attempt: 1, Stage: domainevaluation.AttemptStagePreflight,
		StartedAt: now, FinishedAt: now, RejectionReason: "insufficient_eligible_dimensions",
		Assertions: []domainevaluation.AssertionReceipt{{
			Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1,
			Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := committer.CommitStart(context.Background(), runRecord); err != nil {
		t.Fatal(err)
	}
	assertEvaluationStepEvent(t, stager.events[0], "g1", 1)
	if !stager.stagedInTransaction || postCommit.calls != 1 {
		t.Fatalf("start transaction/postcommit = %v/%d", stager.stagedInTransaction, postCommit.calls)
	}
	if !capacity.reservedInTransaction || len(capacity.reservations) != 1 {
		t.Fatalf("capacity transaction/reservations = %v/%#v", capacity.reservedInTransaction, capacity.reservations)
	}
	reservation := capacity.reservations[0]
	if reservation.RunID != runRecord.ID() || reservation.OrgID != 12 || reservation.ProviderInvocations != MaxProviderInvocationsV1 ||
		reservation.DailyLimit != 140 || !reservation.BudgetDay.Equal(domainevaluation.UTCBudgetDay(now)) || reservation.RequestedBy != "user:42" {
		t.Fatalf("capacity reservation = %#v", reservation)
	}

	evidence, err := NewEvidenceService(repository, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	_, err = evidence.ClaimAttempt(context.Background(), runRecord.ID(), ClaimAttemptCommand{
		CaseID: "g1", Attempt: 1, Owner: "event-g1-1", InvocationID: "invocation-g1-1",
		ClaimedAt: now, LeaseExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = evidence.MarkAttemptDispatching(context.Background(), runRecord.ID(), "event-g1-1", now)
	if err != nil {
		t.Fatal(err)
	}
	normalized := []byte(`{"summary":"synthetic"}`)
	receipt := aiexplanation.ProviderReceipt{
		InvocationID: "invocation-g1-1", RequestID: "request-1", Provider: "provider-a", Model: "model-a", Latency: time.Second,
	}
	updated, err := committer.CommitAttempt(context.Background(), runRecord.ID(), "event-g1-1", domainevaluation.AttemptRecord{
		CaseID: "g1", Attempt: 1, Stage: domainevaluation.AttemptStageGeneration,
		StartedAt: now, FinishedAt: now, ProviderCallCount: 1, ProviderReceipt: &receipt,
		RawOutput: normalized, NormalizedOutput: normalized, OutputFingerprint: aiexplanation.NewFingerprint(normalized),
		Assertions: []domainevaluation.AssertionReceipt{{
			Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1,
			Hard: true, Evaluator: "contract-v1", Status: domainevaluation.AssertionPassed,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Execution() != nil || !updated.HasAttempt("g1", 1) {
		t.Fatalf("updated run = %#v", updated)
	}
	assertEvaluationStepEvent(t, stager.events[1], "g1", 2)
	if postCommit.calls != 2 {
		t.Fatalf("postcommit calls = %d", postCommit.calls)
	}
	recovered, err := committer.CommitRecovery(context.Background(), runRecord.ID(), "recovery-1", "user:88", "redeliver exhausted step")
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.Recoveries()) != 1 || len(stager.events) != 3 ||
		stager.events[2].EventID() != evaluationevents.PromptEvaluationRecoveryEventID(runRecord.ID().String(), "recovery-1") || postCommit.calls != 3 {
		t.Fatalf("recovery audit/event/postcommit = %#v / %#v / %d", recovered.Recoveries(), stager.events, postCommit.calls)
	}
	assertEvaluationStepTarget(t, stager.events[2], "g1", 2)
}

func TestDurableCommitterRejectsStartBeforeRunAndEventWhenDailyBudgetIsExhausted(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	repository := &evidenceRepositoryStub{}
	stager := &evaluationEventStagerStub{}
	postCommit := &evaluationPostCommitStub{}
	capacity := &capacityRepositoryStub{reserveErr: domainevaluation.ErrDailyBudgetExceeded}
	committer, err := NewDurableCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }),
		repository, evaluationevents.Factory{}, stager, postCommit, capacity, 140, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	runRecord, err := domainevaluation.NewRequested(meta.ID(603), evidenceRelease(), 12, "user:42", "release evaluation", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: "preflight", Attempt: 1, Stage: domainevaluation.AttemptStagePreflight, StartedAt: now, FinishedAt: now,
		RejectionReason: "insufficient_eligible_dimensions", Assertions: []domainevaluation.AssertionReceipt{{
			Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true,
			Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	err = committer.CommitStart(context.Background(), runRecord)
	if !errors.Is(err, domainevaluation.ErrDailyBudgetExceeded) {
		t.Fatalf("start error = %v", err)
	}
	if repository.created != nil || len(stager.events) != 0 || postCommit.calls != 0 || len(capacity.reservations) != 1 {
		t.Fatalf("rejected start side effects: run=%#v events=%d postcommit=%d reservations=%d", repository.created, len(stager.events), postCommit.calls, len(capacity.reservations))
	}
}

func TestNewDurableCommitterRejectsMissingCapacityAndInvalidBudget(t *testing.T) {
	dependencies := func(capacity domainevaluation.CapacityRepository, budget int) error {
		_, err := NewDurableCommitter(
			apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }),
			&evidenceRepositoryStub{}, evaluationevents.Factory{}, &evaluationEventStagerStub{}, &evaluationPostCommitStub{},
			capacity, budget, time.Now,
		)
		return err
	}
	if err := dependencies(nil, 140); err == nil {
		t.Fatal("expected nil capacity repository to be rejected")
	}
	if err := dependencies(&capacityRepositoryStub{}, 69); err == nil {
		t.Fatal("expected budget below one complete v1 run to be rejected")
	}
	if err := dependencies(&capacityRepositoryStub{}, 1024); err != nil {
		t.Fatalf("expected budget with a partial remainder to be accepted: %v", err)
	}
}

func TestDurableCommitterRechecksExactPreparedInvocationBeforeRecovery(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	now := startedAt.Add(3 * time.Minute)
	repository := &evidenceRepositoryStub{}
	stager := &evaluationEventStagerStub{}
	postCommit := &evaluationPostCommitStub{}
	committer, err := NewDurableCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }),
		repository, evaluationevents.Factory{}, stager, postCommit, &capacityRepositoryStub{}, 140, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	runRecord, err := domainevaluation.NewRequested(meta.ID(602), evidenceRelease(), 12, "user:42", "release evaluation", startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: "preflight", Attempt: 1, Stage: domainevaluation.AttemptStagePreflight, StartedAt: startedAt, FinishedAt: startedAt,
		RejectionReason: "insufficient_eligible_dimensions", Assertions: []domainevaluation.AssertionReceipt{{
			Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true,
			Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.BeginAttemptExecution(domainevaluation.AttemptExecution{
		CaseID: "g1", Attempt: 1, Owner: "event-g1-1", InvocationID: "invocation-g1-1",
		Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: startedAt, LeaseExpiresAt: startedAt.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	repository.created = runRecord

	leaseExpiresAt := startedAt.Add(time.Minute)
	if _, err := committer.CommitExpiredPreparationRecovery(context.Background(), runRecord.ID(), "stale-invocation", leaseExpiresAt, "auto-1", "system:scanner", "expired prepared"); err == nil {
		t.Fatal("expected stale invocation to be rejected")
	}
	recovered, err := committer.CommitExpiredPreparationRecovery(context.Background(), runRecord.ID(), "invocation-g1-1", leaseExpiresAt, "auto-1", "system:scanner", "expired prepared")
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.Recoveries()) != 1 || len(stager.events) != 1 || postCommit.calls != 1 {
		t.Fatalf("recovery evidence = %#v, events=%d postcommit=%d", recovered.Recoveries(), len(stager.events), postCommit.calls)
	}
}

func assertEvaluationStepEvent(t *testing.T, value event.DomainEvent, caseID string, attempt int) {
	t.Helper()
	if value == nil || value.EventType() != eventcatalog.AIExplanationPromptEvaluationStepRequested ||
		value.AggregateType() != evaluationevents.PromptEvaluationAggregateType ||
		value.EventID() != evaluationevents.PromptEvaluationStepEventID(value.AggregateID(), caseID, attempt) {
		t.Fatalf("evaluation event identity = %#v", value)
	}
	typed, ok := value.(evaluationevents.PromptEvaluationStepEvent)
	if !ok || typed.Data.CaseID != caseID || typed.Data.Attempt != attempt || typed.Data.RequestedBy != "user:42" {
		t.Fatalf("evaluation event payload = %#v", value)
	}
}

func assertEvaluationStepTarget(t *testing.T, value event.DomainEvent, caseID string, attempt int) {
	t.Helper()
	typed, ok := value.(evaluationevents.PromptEvaluationStepEvent)
	if !ok || typed.Data.CaseID != caseID || typed.Data.Attempt != attempt {
		t.Fatalf("evaluation event target = %#v", value)
	}
}

type evaluationEventStagerStub struct {
	insideTransaction   bool
	stagedInTransaction bool
	events              []event.DomainEvent
}

func (s *evaluationEventStagerStub) Stage(_ context.Context, values ...event.DomainEvent) error {
	s.stagedInTransaction = s.stagedInTransaction || s.insideTransaction
	s.events = append(s.events, values...)
	return nil
}

type evaluationPostCommitStub struct{ calls int }

func (s *evaluationPostCommitStub) AfterCommit(context.Context, []event.DomainEvent, time.Time) {
	s.calls++
}

type capacityRepositoryStub struct {
	ensureErr             error
	reserveErr            error
	insideTransaction     bool
	reservedInTransaction bool
	reservations          []domainevaluation.DailyCapacityReservation
}

func (s *capacityRepositoryStub) EnsureDailyBucket(context.Context, int64, time.Time, time.Time) error {
	return s.ensureErr
}

func (s *capacityRepositoryStub) ReserveDailyProviderInvocations(_ context.Context, value domainevaluation.DailyCapacityReservation) error {
	s.reservedInTransaction = s.reservedInTransaction || s.insideTransaction
	s.reservations = append(s.reservations, value)
	return s.reserveErr
}
