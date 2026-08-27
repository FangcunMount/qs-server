package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestAuthorizePersistsOneIdempotentManualRetry(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	generationRecord, latest := failedRecoveryFixture(t, now)
	committer := &retryCommitterStub{}
	service, err := NewService(
		&recoveryGenerationRepositoryStub{value: generationRecord},
		&recoveryRunRepositoryStub{value: latest}, committer, func() time.Time { return now.Add(time.Minute) },
	)
	if err != nil {
		t.Fatal(err)
	}
	command := Command{OrgID: 7, GenerationID: generationRecord.ID(), ExpectedAttempt: 1, RequestID: "retry-request-1", Actor: "user:42", Reason: "provider timeout recovery"}
	result, err := service.Authorize(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Authorization.NextAttempt != 2 || result.Authorization.Origin != retrygovernance.AttemptOriginManual || committer.calls != 1 {
		t.Fatalf("first authorization = %#v, calls = %d", result, committer.calls)
	}
	if result.Authorization.EventID == "" || result.Authorization.RequestID != command.RequestID {
		t.Fatalf("authorization identity = %#v", result.Authorization)
	}

	replayed, err := service.Authorize(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Created || replayed.Authorization.EventID != result.Authorization.EventID || committer.calls != 1 {
		t.Fatalf("replayed authorization = %#v, calls = %d", replayed, committer.calls)
	}

	conflicting := command
	conflicting.Reason = "different authorization payload"
	if _, err := service.Authorize(context.Background(), conflicting); !errors.Is(err, domainrun.ErrConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestAuthorizeRejectsOrganizationAndStaleAttempt(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	generationRecord, latest := failedRecoveryFixture(t, now)
	service, err := NewService(
		&recoveryGenerationRepositoryStub{value: generationRecord},
		&recoveryRunRepositoryStub{value: latest}, &retryCommitterStub{}, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	base := Command{OrgID: 7, GenerationID: generationRecord.ID(), ExpectedAttempt: 1, RequestID: "retry-request-1", Actor: "user:42", Reason: "manual recovery"}
	wrongOrg := base
	wrongOrg.OrgID = 8
	if _, err := service.Authorize(context.Background(), wrongOrg); err == nil {
		t.Fatal("organization mismatch must fail closed")
	}
	stale := base
	stale.ExpectedAttempt = 2
	if _, err := service.Authorize(context.Background(), stale); !errors.Is(err, domainrun.ErrConflict) {
		t.Fatalf("stale attempt error = %v", err)
	}
}

func TestLeaseRecovererSchedulesOneDurableWakeupWithoutNewAttempt(t *testing.T) {
	startedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	generationRecord, runRecord := runningRecoveryFixture(t, startedAt)
	leaseExpiredAt := *runRecord.LeaseExpiresAt()
	runs := &recoveryRunRepositoryStub{value: runRecord, leases: []domainrun.ExpiredLease{{
		RunID: runRecord.ID(), GenerationID: generationRecord.ID(), LeaseExpiredAt: leaseExpiredAt,
		InvocationPhase: domainrun.InvocationPhasePrepared,
	}}}
	committer := &leaseRecoveryCommitterStub{}
	recoverer, err := NewLeaseRecoverer(runs, &recoveryGenerationRepositoryStub{value: generationRecord}, runs, committer)
	if err != nil {
		t.Fatal(err)
	}
	at := leaseExpiredAt.Add(time.Minute)
	scheduled, err := recoverer.RecoverExpiredLeases(context.Background(), at, 20)
	if err != nil || scheduled != 1 || committer.calls != 1 {
		t.Fatalf("first recovery = scheduled:%d calls:%d error:%v", scheduled, committer.calls, err)
	}
	wakeup := runRecord.RecoveryWakeup()
	if wakeup == nil || wakeup.ExpectedLeaseExpiresAt != leaseExpiredAt || wakeup.InvocationPhase != domainrun.InvocationPhasePrepared {
		t.Fatalf("stored wake-up = %#v", wakeup)
	}
	scheduled, err = recoverer.RecoverExpiredLeases(context.Background(), at, 20)
	if err != nil || scheduled != 0 || committer.calls != 2 || runRecord.Attempt() != 1 {
		t.Fatalf("replayed recovery = scheduled:%d calls:%d attempt:%d error:%v", scheduled, committer.calls, runRecord.Attempt(), err)
	}
}

func failedRecoveryFixture(t *testing.T, now time.Time) (*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun) {
	generationRecord, latest := runningRecoveryFixture(t, now)
	failedAt := now.Add(time.Second)
	if err := latest.Fail(failedAt, domainrun.Failure{Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Fail(latest.ID(), failedAt); err != nil {
		t.Fatal(err)
	}
	return generationRecord, latest
}

func runningRecoveryFixture(t *testing.T, now time.Time) (*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun) {
	t.Helper()
	snapshot, err := domaininput.NewSnapshot([]byte(`{"schema_version":"ai-explanation-input/v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	profile := aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("profile"))}
	execution := aiexplanation.ProviderExecutionSpec{Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))}
	generationRecord, err := domaingeneration.New(domaingeneration.NewInput{
		ID: meta.FromUint64(700), Key: domaingeneration.Key{SourceReportID: meta.FromUint64(101), Audience: policy.AudienceParticipant, Profile: profile, InputFingerprint: snapshot.Fingerprint(), ExecutionSpecFingerprint: execution.Fingerprint},
		Association: aiexplanation.Association{OrgID: 7, AssessmentID: meta.FromUint64(501), TesteeID: 9}, RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: "user-42"}, Input: snapshot,
		Prompt: aiexplanation.PromptRef{TemplateID: "cross-dimension-participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123"}, ExecutionSpec: execution, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
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
	return generationRecord, latest
}

type recoveryGenerationRepositoryStub struct {
	value *domaingeneration.AIExplanationGeneration
}

func (*recoveryGenerationRepositoryStub) Create(context.Context, *domaingeneration.AIExplanationGeneration) error {
	return nil
}
func (s *recoveryGenerationRepositoryStub) FindByID(context.Context, meta.ID) (*domaingeneration.AIExplanationGeneration, error) {
	return s.value, nil
}
func (*recoveryGenerationRepositoryStub) FindByKey(context.Context, domaingeneration.Key) (*domaingeneration.AIExplanationGeneration, error) {
	return nil, domaingeneration.ErrNotFound
}
func (*recoveryGenerationRepositoryStub) Save(context.Context, *domaingeneration.AIExplanationGeneration, uint64) error {
	return nil
}

type recoveryRunRepositoryStub struct {
	value  *domainrun.AIExplanationRun
	leases []domainrun.ExpiredLease
}

func (*recoveryRunRepositoryStub) Create(context.Context, *domainrun.AIExplanationRun) error {
	return nil
}
func (s *recoveryRunRepositoryStub) FindByID(context.Context, meta.ID) (*domainrun.AIExplanationRun, error) {
	return s.value, nil
}
func (s *recoveryRunRepositoryStub) FindLatestByGenerationID(context.Context, meta.ID) (*domainrun.AIExplanationRun, error) {
	return s.value, nil
}
func (*recoveryRunRepositoryStub) Save(context.Context, *domainrun.AIExplanationRun) error {
	return nil
}
func (s *recoveryRunRepositoryStub) ListExpiredLeases(context.Context, time.Time, int) ([]domainrun.ExpiredLease, error) {
	return append([]domainrun.ExpiredLease(nil), s.leases...), nil
}

type retryCommitterStub struct{ calls int }

func (s *retryCommitterStub) CommitRetryAuthorization(_ context.Context, _ *domaingeneration.AIExplanationGeneration, latest *domainrun.AIExplanationRun, authorization domainrun.RetryAuthorization) (*domainrun.AIExplanationRun, bool, error) {
	s.calls++
	stored := latest.RetryAuthorization()
	if stored == nil || stored.RequestID != authorization.RequestID {
		return nil, false, errors.New("authorization was not attached before commit")
	}
	return latest, true, nil
}

type leaseRecoveryCommitterStub struct{ calls int }

func (s *leaseRecoveryCommitterStub) CommitLeaseRecoveryWakeup(_ context.Context, _ *domaingeneration.AIExplanationGeneration, runRecord *domainrun.AIExplanationRun, wakeup domainrun.RecoveryWakeup) (bool, error) {
	s.calls++
	return runRecord.ScheduleRecoveryWakeup(wakeup)
}
