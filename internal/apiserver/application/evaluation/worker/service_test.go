package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type engineStub struct{ err error }

func (s engineStub) Evaluate(context.Context, uint64) error { return s.err }

type assessmentRepoStub struct {
	domainassessment.Repository
	value *domainassessment.Assessment
}

func (s assessmentRepoStub) FindByID(context.Context, domainassessment.ID) (*domainassessment.Assessment, error) {
	return s.value, nil
}

type runRepoStub struct {
	evaluationrun.Repository
	value *evalrun.EvaluationRun
}

func (s runRepoStub) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return s.value, nil
}

func TestExecuteReturnsRetryableFailureReceiptWithoutTransportRequery(t *testing.T) {
	a, err := domainassessment.NewAssessment(9, domaintestee.NewID(7), domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(3)), domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkAsFailed("calculation failed"); err != nil {
		t.Fatal(err)
	}
	run := evalrun.NewEvaluationRun(1)
	if err := run.Start(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if err := run.Fail(time.Unix(11, 0), evalrun.Failure{Kind: evalrun.FailureKindCalculation, Message: "calculation failed", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	run.TraceID = "trace-1"
	run.InputSnapshotRef = "model:S@1"
	svc := NewService(engineStub{err: errors.New("boom")}, assessmentRepoStub{value: a}, nil, runRepoStub{value: &run})
	result, err := svc.Execute(context.Background(), Command{AssessmentID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || !result.Retryable || result.RunID == "" || result.FailureKind != "calculation" || result.TraceID != "trace-1" || result.InputSnapshotRef != "model:S@1" {
		t.Fatalf("result = %#v", result)
	}
}
