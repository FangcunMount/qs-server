package automation

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	interpretationexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type outcomeRepoStub struct {
	record *evaluationfact.Record
	err    error
	reads  int
}

func (s *outcomeRepoStub) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	s.reads++
	if s.err != nil {
		return nil, s.err
	}
	return s.record, nil
}
func (s *outcomeRepoStub) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	panic("automation must not resolve Outcome by Assessment")
}

type executorStub struct {
	calls   int
	input   interpinput.InterpretationInput
	traceID string
}

func (s *executorStub) Execute(_ context.Context, input interpinput.InterpretationInput, traceID string) (*interpretationexecution.ExecuteResult, error) {
	s.calls++
	s.input, s.traceID = input, traceID
	return &interpretationexecution.ExecuteResult{Status: interpretationexecution.ExecuteStatusProcessing}, nil
}

type admissionRepoStub struct {
	items   map[string]*admission.Failure
	creates int
}

func (s *admissionRepoStub) UpsertByFingerprint(_ context.Context, failure *admission.Failure) (bool, error) {
	if s.items == nil {
		s.items = map[string]*admission.Failure{}
	}
	if _, ok := s.items[failure.Fingerprint()]; ok {
		return false, nil
	}
	s.creates++
	s.items[failure.Fingerprint()] = failure
	return true, nil
}
func (s *admissionRepoStub) FindByFingerprint(_ context.Context, fingerprint string) (*admission.Failure, error) {
	if item, ok := s.items[fingerprint]; ok {
		return item, nil
	}
	return nil, admission.ErrNotFound
}
func (s *admissionRepoStub) FindByOutcomeID(context.Context, meta.ID, int) ([]*admission.Failure, error) {
	return nil, nil
}

func TestGenerateRequiresTrustedActorBeforeReadingOutcome(t *testing.T) {
	repo := &outcomeRepoStub{}
	service, err := NewService(repo, &executorStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Generate(context.Background(), GenerateCommand{OutcomeID: meta.FromUint64(1)}); err == nil {
		t.Fatal("Generate error = nil, want actor error")
	}
	if repo.reads != 0 {
		t.Fatalf("outcome reads = %d, want 0", repo.reads)
	}
}

func TestGeneratePersistsAdmissionFailureWithoutStartingExecutor(t *testing.T) {
	repo := &outcomeRepoStub{err: evaluationfact.ErrNotFound}
	admissions := &admissionRepoStub{}
	executor := &executorStub{}
	svc, err := NewService(repo, executor, admissions)
	if err != nil {
		t.Fatal(err)
	}
	concrete := svc.(*service)
	concrete.now = func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) }
	concrete.newID = func() meta.ID { return meta.FromUint64(77) }

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "evt-admission-1"))
	_, err = svc.Generate(ctx, GenerateCommand{Actor: TrustedServiceActor("worker"), OutcomeID: meta.FromUint64(9), TraceID: "trace-1"})
	rejected, ok := admission.RejectedFrom(err)
	if !ok || rejected.Failure == nil || rejected.Failure.Kind() != admission.KindOutcomeNotFound {
		t.Fatalf("err = %v, want admission rejected", err)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
	if admissions.creates != 1 {
		t.Fatalf("admission creates = %d, want 1", admissions.creates)
	}

	_, err = svc.Generate(ctx, GenerateCommand{Actor: TrustedServiceActor("worker"), OutcomeID: meta.FromUint64(9), TraceID: "trace-1"})
	if _, ok := admission.RejectedFrom(err); !ok {
		t.Fatalf("replay err = %v", err)
	}
	if admissions.creates != 1 {
		t.Fatalf("replay must be idempotent, creates=%d", admissions.creates)
	}
}
