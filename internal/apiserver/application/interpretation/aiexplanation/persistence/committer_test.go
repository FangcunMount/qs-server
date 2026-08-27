package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestCommitterStagesRequestAndFailureInsideTransactionBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	generationRecord := testGeneration(t, now)
	sequence := []string{}
	generations := &generationStub{sequence: &sequence}
	runs := &runStub{sequence: &sequence}
	artifacts := &artifactStub{}
	stager := &stagerStub{sequence: &sequence}
	postCommit := &postCommitStub{sequence: &sequence}
	capacity := &participantCapacityStub{sequence: &sequence}
	activeCapacity := &participantActiveCapacityStub{sequence: &sequence}
	tx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		sequence = append(sequence, "tx.begin")
		err := fn(ctx)
		sequence = append(sequence, "tx.end")
		return err
	})
	committer, err := NewCommitter(tx, generations, runs, artifacts, capacity, activeCapacity, testParticipantCapacityPolicy(), eventFactoryStub{}, stager, postCommit)
	if err != nil {
		t.Fatal(err)
	}
	if err := committer.CommitRequested(context.Background(), generationRecord); err != nil {
		t.Fatal(err)
	}
	assertSequence(t, sequence, []string{"capacity.ensure", "tx.begin", "capacity.reserve", "generation.create", "event.ai.requested", "tx.end", "postcommit.ai.requested"})

	sequence = sequence[:0]
	runRecord, err := domainrun.NewPending(meta.FromUint64(800), generationRecord.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now, "trace-1", now.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	expected := generationRecord.Version()
	if err := generationRecord.Begin(runRecord.ID(), now); err != nil {
		t.Fatal(err)
	}
	if err := committer.CommitStart(context.Background(), generationRecord, runRecord, expected); err != nil {
		t.Fatal(err)
	}
	assertSequence(t, sequence, []string{"active.ensure", "tx.begin", "active.acquire", "run.create", "generation.save", "tx.end"})

	sequence = sequence[:0]
	if err := runRecord.BeginProviderDispatch(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := committer.SaveDispatching(context.Background(), runRecord); err != nil {
		t.Fatal(err)
	}
	assertSequence(t, sequence, []string{"run.save"})

	sequence = sequence[:0]
	expected = generationRecord.Version()
	failure := domainrun.Failure{Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "AI 解读暂时不可用", Retryable: true}
	if err := runRecord.Fail(now.Add(2*time.Second), failure); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Fail(runRecord.ID(), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := committer.CommitFailure(context.Background(), generationRecord, runRecord, expected); err != nil {
		t.Fatal(err)
	}
	assertSequence(t, sequence, []string{"tx.begin", "run.save", "generation.save", "event.ai.failed", "active.release", "tx.end", "postcommit.ai.failed"})
}

func TestCommitRequestedStopsBeforeGenerationWhenCapacityRejects(t *testing.T) {
	sequence := []string{}
	capacity := &participantCapacityStub{sequence: &sequence, reserveErr: domaingeneration.ErrAssessmentDailyBudgetExceeded}
	committer, err := NewCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			sequence = append(sequence, "tx.begin")
			err := fn(ctx)
			sequence = append(sequence, "tx.end")
			return err
		}),
		&generationStub{sequence: &sequence}, &runStub{sequence: &sequence}, &artifactStub{}, capacity,
		&participantActiveCapacityStub{sequence: &sequence},
		testParticipantCapacityPolicy(), eventFactoryStub{}, &stagerStub{sequence: &sequence}, &postCommitStub{sequence: &sequence},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = committer.CommitRequested(context.Background(), testGeneration(t, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)))
	if err != domaingeneration.ErrAssessmentDailyBudgetExceeded {
		t.Fatalf("capacity error = %v", err)
	}
	assertSequence(t, sequence, []string{"capacity.ensure", "tx.begin", "capacity.reserve", "tx.end"})
}

func TestCommitStartStopsBeforeRunWhenActiveCapacityRejects(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	sequence := []string{}
	generationRecord := testGeneration(t, now)
	runRecord, err := domainrun.NewPending(meta.FromUint64(800), generationRecord.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now, "trace-1", now.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	expected := generationRecord.Version()
	if err := generationRecord.Begin(runRecord.ID(), now); err != nil {
		t.Fatal(err)
	}
	committer, err := NewCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			sequence = append(sequence, "tx.begin")
			err := fn(ctx)
			sequence = append(sequence, "tx.end")
			return err
		}),
		&generationStub{sequence: &sequence}, &runStub{sequence: &sequence}, &artifactStub{},
		&participantCapacityStub{sequence: &sequence},
		&participantActiveCapacityStub{sequence: &sequence, acquireErr: domaingeneration.ErrAssessmentActiveCapacityExceeded},
		testParticipantCapacityPolicy(), eventFactoryStub{}, &stagerStub{sequence: &sequence}, &postCommitStub{sequence: &sequence},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = committer.CommitStart(context.Background(), generationRecord, runRecord, expected)
	if err != domaingeneration.ErrAssessmentActiveCapacityExceeded {
		t.Fatalf("active capacity error = %v", err)
	}
	assertSequence(t, sequence, []string{"active.ensure", "tx.begin", "active.acquire", "tx.end"})
}

func TestCommitLeaseRecoveryWakeupStagesExactEventOnlyOnce(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	sequence := []string{}
	generationRecord := testGeneration(t, now)
	runRecord, err := domainrun.NewPending(meta.FromUint64(800), generationRecord.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now, "trace-1", now.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Begin(runRecord.ID(), now); err != nil {
		t.Fatal(err)
	}
	runs := &runStub{sequence: &sequence, authorizationRun: runRecord}
	committer, err := NewCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			sequence = append(sequence, "tx.begin")
			err := fn(ctx)
			sequence = append(sequence, "tx.end")
			return err
		}),
		&generationStub{sequence: &sequence}, runs, &artifactStub{},
		&participantCapacityStub{sequence: &sequence}, &participantActiveCapacityStub{sequence: &sequence},
		testParticipantCapacityPolicy(), eventFactoryStub{}, &stagerStub{sequence: &sequence}, &postCommitStub{sequence: &sequence},
	)
	if err != nil {
		t.Fatal(err)
	}
	wakeup := domainrun.RecoveryWakeup{
		EventID: "lease-recovery-1", ExpectedLeaseExpiresAt: now.Add(time.Minute),
		InvocationPhase: domainrun.InvocationPhasePrepared, RequestedAt: now.Add(2 * time.Minute),
	}
	created, err := committer.CommitLeaseRecoveryWakeup(context.Background(), generationRecord, runRecord, wakeup)
	if err != nil || !created {
		t.Fatalf("first wake-up = created:%t error:%v", created, err)
	}
	assertSequence(t, sequence, []string{"tx.begin", "run.schedule_recovery", "event.ai.lease_recovery_requested", "tx.end", "postcommit.ai.lease_recovery_requested"})

	sequence = sequence[:0]
	created, err = committer.CommitLeaseRecoveryWakeup(context.Background(), generationRecord, runRecord, wakeup)
	if err != nil || created {
		t.Fatalf("replayed wake-up = created:%t error:%v", created, err)
	}
	assertSequence(t, sequence, []string{"tx.begin", "run.schedule_recovery", "tx.end"})
}

func TestCommitRetryAuthorizationBudgetsAndStagesExactlyOnce(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	generationRecord := testGeneration(t, now)
	latest, err := domainrun.NewPending(meta.FromUint64(800), generationRecord.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := latest.StartWithLease(now, "trace-1", now.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Begin(latest.ID(), now); err != nil {
		t.Fatal(err)
	}
	failureAt := now.Add(time.Second)
	if err := latest.Fail(failureAt, domainrun.Failure{Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Fail(latest.ID(), failureAt); err != nil {
		t.Fatal(err)
	}
	authorization := domainrun.RetryAuthorization{
		ExpectedAttempt: 1, NextAttempt: 2, Origin: retrygovernance.AttemptOriginManual,
		RequestID: "retry-request-1", EventID: "retry-event-1", Actor: "user:42", Reason: "manual recovery", AuthorizedAt: now.Add(2 * time.Second),
	}
	sequence := []string{}
	runs := &runStub{sequence: &sequence, authorizationRun: latest}
	capacity := &participantCapacityStub{sequence: &sequence}
	committer, err := NewCommitter(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			sequence = append(sequence, "tx.begin")
			err := fn(ctx)
			sequence = append(sequence, "tx.end")
			return err
		}),
		&generationStub{sequence: &sequence}, runs, &artifactStub{}, capacity, &participantActiveCapacityStub{sequence: &sequence},
		testParticipantCapacityPolicy(), eventFactoryStub{}, &stagerStub{sequence: &sequence}, &postCommitStub{sequence: &sequence},
	)
	if err != nil {
		t.Fatal(err)
	}
	authorized, created, err := committer.CommitRetryAuthorization(context.Background(), generationRecord, latest, authorization)
	if err != nil || !created || authorized == nil {
		t.Fatalf("first retry authorization = %#v/%v/%v", authorized, created, err)
	}
	assertSequence(t, sequence, []string{"capacity.ensure", "tx.begin", "run.authorize", "capacity.reserve", "event.ai.retry_requested", "tx.end", "postcommit.ai.retry_requested"})
	if capacity.lastReservation.Attempt != 2 || capacity.lastReservation.ReservationID != domaingeneration.ParticipantCapacityReservationID(generationRecord.ID(), 2) || capacity.lastReservation.Origin != retrygovernance.AttemptOriginManual {
		t.Fatalf("retry capacity reservation = %#v", capacity.lastReservation)
	}

	sequence = sequence[:0]
	authorized, created, err = committer.CommitRetryAuthorization(context.Background(), generationRecord, latest, authorization)
	if err != nil || created || authorized == nil {
		t.Fatalf("idempotent retry authorization = %#v/%v/%v", authorized, created, err)
	}
	assertSequence(t, sequence, []string{"capacity.ensure", "tx.begin", "run.authorize", "tx.end"})
}

func testParticipantCapacityPolicy() domaingeneration.ParticipantCapacityPolicy {
	return domaingeneration.ParticipantCapacityPolicy{
		DailyProviderInvocationBudgetPerOrg:        500,
		DailyProviderInvocationBudgetPerUser:       5,
		DailyProviderInvocationBudgetPerAssessment: 3,
		MaxActiveProviderExecutionsPerOrg:          10,
		MaxActiveProviderExecutionsPerUser:         2,
		MaxActiveProviderExecutionsPerAssessment:   1,
	}
}

func testGeneration(t *testing.T, now time.Time) *domaingeneration.AIExplanationGeneration {
	t.Helper()
	snapshot, err := domaininput.NewSnapshot([]byte(`{"schema_version":"ai-explanation-input/v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	profile := aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("profile"))}
	execution := aiexplanation.ProviderExecutionSpec{Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))}
	value, err := domaingeneration.New(domaingeneration.NewInput{
		ID: meta.FromUint64(700), Key: domaingeneration.Key{SourceReportID: meta.FromUint64(101), Audience: policy.AudienceParticipant, Profile: profile, InputFingerprint: snapshot.Fingerprint(), ExecutionSpecFingerprint: execution.Fingerprint},
		Association: aiexplanation.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 9}, RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: "user-1"}, Input: snapshot,
		Prompt: aiexplanation.PromptRef{TemplateID: "cross-dimension-participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123"}, ExecutionSpec: execution, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type generationStub struct{ sequence *[]string }

func (s *generationStub) Create(context.Context, *domaingeneration.AIExplanationGeneration) error {
	*s.sequence = append(*s.sequence, "generation.create")
	return nil
}
func (*generationStub) FindByID(context.Context, meta.ID) (*domaingeneration.AIExplanationGeneration, error) {
	return nil, domaingeneration.ErrNotFound
}
func (*generationStub) FindByKey(context.Context, domaingeneration.Key) (*domaingeneration.AIExplanationGeneration, error) {
	return nil, domaingeneration.ErrNotFound
}
func (s *generationStub) Save(context.Context, *domaingeneration.AIExplanationGeneration, uint64) error {
	*s.sequence = append(*s.sequence, "generation.save")
	return nil
}

type runStub struct {
	sequence         *[]string
	authorizationRun *domainrun.AIExplanationRun
}

func (s *runStub) Create(context.Context, *domainrun.AIExplanationRun) error {
	*s.sequence = append(*s.sequence, "run.create")
	return nil
}
func (*runStub) FindByID(context.Context, meta.ID) (*domainrun.AIExplanationRun, error) {
	return nil, domainrun.ErrNotFound
}
func (*runStub) FindLatestByGenerationID(context.Context, meta.ID) (*domainrun.AIExplanationRun, error) {
	return nil, domainrun.ErrNotFound
}
func (s *runStub) Save(context.Context, *domainrun.AIExplanationRun) error {
	*s.sequence = append(*s.sequence, "run.save")
	return nil
}
func (s *runStub) AuthorizeRetry(_ context.Context, _ meta.ID, authorization domainrun.RetryAuthorization) (*domainrun.AIExplanationRun, bool, error) {
	*s.sequence = append(*s.sequence, "run.authorize")
	if s.authorizationRun == nil {
		return nil, false, domainrun.ErrNotFound
	}
	if existing := s.authorizationRun.RetryAuthorization(); existing != nil {
		if existing.SameAction(authorization) {
			return s.authorizationRun, false, nil
		}
		return nil, false, domainrun.ErrConflict
	}
	if err := s.authorizationRun.AuthorizeManualRetry(authorization); err != nil {
		return nil, false, err
	}
	return s.authorizationRun, true, nil
}
func (s *runStub) ScheduleRecoveryWakeup(_ context.Context, _ meta.ID, wakeup domainrun.RecoveryWakeup) (*domainrun.AIExplanationRun, bool, error) {
	*s.sequence = append(*s.sequence, "run.schedule_recovery")
	if s.authorizationRun == nil {
		return nil, false, domainrun.ErrNotFound
	}
	created, err := s.authorizationRun.ScheduleRecoveryWakeup(wakeup)
	return s.authorizationRun, created, err
}

type artifactStub struct{}

func (*artifactStub) Insert(context.Context, *domainartifact.AIExplanationArtifact) error { return nil }
func (*artifactStub) FindByID(context.Context, meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}
func (*artifactStub) FindByGenerationID(context.Context, meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}
func (*artifactStub) FindBySourceReportAndAudience(context.Context, meta.ID, policy.Audience) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}

type participantCapacityStub struct {
	sequence        *[]string
	reserveErr      error
	lastReservation domaingeneration.ParticipantDailyCapacityReservation
}

func (s *participantCapacityStub) EnsureParticipantDailyBucket(context.Context, int64, time.Time, time.Time) error {
	*s.sequence = append(*s.sequence, "capacity.ensure")
	return nil
}

func (s *participantCapacityStub) ReserveParticipantDailyProviderInvocations(_ context.Context, reservation domaingeneration.ParticipantDailyCapacityReservation) error {
	*s.sequence = append(*s.sequence, "capacity.reserve")
	s.lastReservation = reservation
	if err := reservation.Validate(); err != nil {
		return err
	}
	return s.reserveErr
}

type participantActiveCapacityStub struct {
	sequence   *[]string
	acquireErr error
	releaseErr error
}

func (s *participantActiveCapacityStub) EnsureParticipantActiveBucket(context.Context, int64, time.Time) error {
	*s.sequence = append(*s.sequence, "active.ensure")
	return nil
}

func (s *participantActiveCapacityStub) AcquireParticipantActiveSlot(_ context.Context, slot domaingeneration.ParticipantActiveSlot) error {
	*s.sequence = append(*s.sequence, "active.acquire")
	if err := slot.Validate(); err != nil {
		return err
	}
	return s.acquireErr
}

func (s *participantActiveCapacityStub) ReleaseParticipantActiveSlot(_ context.Context, release domaingeneration.ParticipantActiveSlotRelease) error {
	*s.sequence = append(*s.sequence, "active.release")
	if err := release.Validate(); err != nil {
		return err
	}
	return s.releaseErr
}

type stagerStub struct{ sequence *[]string }

func (s *stagerStub) Stage(_ context.Context, events ...event.DomainEvent) error {
	for _, item := range events {
		*s.sequence = append(*s.sequence, "event."+item.EventType())
	}
	return nil
}

type postCommitStub struct{ sequence *[]string }

func (s *postCommitStub) AfterCommit(_ context.Context, events []event.DomainEvent, _ time.Time) {
	for _, item := range events {
		*s.sequence = append(*s.sequence, "postcommit."+item.EventType())
	}
}

type eventFactoryStub struct{}

func (eventFactoryStub) Requested(*domaingeneration.AIExplanationGeneration) (event.DomainEvent, error) {
	return event.New("ai.requested", "AIExplanationGeneration", "700", struct{}{}), nil
}
func (eventFactoryStub) RetryRequested(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, domainrun.RetryAuthorization) (event.DomainEvent, error) {
	return event.New("ai.retry_requested", "AIExplanationGeneration", "700", struct{}{}), nil
}
func (eventFactoryStub) LeaseRecoveryRequested(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, domainrun.RecoveryWakeup) (event.DomainEvent, error) {
	return event.New("ai.lease_recovery_requested", "AIExplanationGeneration", "700", struct{}{}), nil
}
func (eventFactoryStub) Generated(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, *domainartifact.AIExplanationArtifact) (event.DomainEvent, error) {
	return event.New("ai.generated", "AIExplanationGeneration", "700", struct{}{}), nil
}
func (eventFactoryStub) Failed(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun) (event.DomainEvent, error) {
	return event.New("ai.failed", "AIExplanationGeneration", "700", struct{}{}), nil
}

func assertSequence(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("sequence = %v, want %v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("sequence = %v, want %v", actual, expected)
		}
	}
}
