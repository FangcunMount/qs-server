package testee

import (
	"context"
	"errors"
	"testing"

	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type latestRunStub struct {
	run   *evalrun.EvaluationRun
	err   error
	calls int
}

func (s *latestRunStub) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	s.calls++
	return s.run, s.err
}

func runtimeStatusAssessment(t *testing.T) *domainassessment.Assessment {
	t.Helper()
	a, err := domainassessment.NewAssessment(9, domaintestee.NewID(7),
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"),
		domainassessment.NewAnswerSheetRef(meta.FromUint64(2)),
		domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(42)))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRuntimeStatusReaderAuthorizesBeforeReadingFreshRun(t *testing.T) {
	run := evalrun.NewEvaluationRunWithAttempt(42, 2)
	runs := &latestRunStub{run: &run}
	access := NewService(assessmentRepoStub{value: runtimeStatusAssessment(t)}, nil, nil)
	reader := NewRuntimeStatusReader(access, runs)

	if _, err := reader.Get(context.Background(), Actor{TesteeID: 8}, 42); err == nil {
		t.Fatal("foreign testee read a run")
	}
	if runs.calls != 0 {
		t.Fatalf("run reads after denied access = %d, want 0", runs.calls)
	}
	got, err := reader.Get(context.Background(), Actor{TesteeID: 7}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Attempt != 2 || got.Status != evalrun.StatusPending || runs.calls != 1 {
		t.Fatalf("latest status = %+v, run reads = %d", got, runs.calls)
	}
}

func TestRuntimeStatusReaderDoesNotTreatReadFailureAsMissingRun(t *testing.T) {
	want := errors.New("database unavailable")
	runs := &latestRunStub{err: want}
	access := NewService(assessmentRepoStub{value: runtimeStatusAssessment(t)}, nil, nil)
	got, err := NewRuntimeStatusReader(access, runs).Get(context.Background(), Actor{TesteeID: 7}, 42)
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("status = %+v, error = %v; want database error", got, err)
	}
}
