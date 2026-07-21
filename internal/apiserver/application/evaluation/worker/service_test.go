package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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

type outcomeRepoStub struct {
	domainoutcome.Repository
	value *domainoutcome.Record
}

func (s outcomeRepoStub) FindByAssessmentID(context.Context, meta.ID) (*domainoutcome.Record, error) {
	return s.value, nil
}

func (s runRepoStub) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	return s.value, nil
}

func TestExecuteReturnsRetryableFailureReceiptWithoutTransportRequery(t *testing.T) {
	a, err := domainassessment.NewAssessment(
		9,
		domaintestee.NewID(7),
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"),
		domainassessment.NewAnswerSheetRef(meta.FromUint64(3)),
		domainassessment.NewAdhocOrigin(),
		domainassessment.WithID(meta.FromUint64(1)),
		domainassessment.WithEvaluationModel(domainassessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S"), "1", "scale")),
	)
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
	run = evalrun.Reconstruct(evalrun.ReconstructInput{RunID: run.ID(), AssessmentID: run.AssessmentID(), Attempt: run.Attempt(), Failure: run.Failure(), TraceID: "trace-1", InputSnapshotRef: "model:S@1", StartedAt: run.StartedAt(), FinishedAt: run.FinishedAt()})
	svc := NewService(engineStub{err: errors.New("boom")}, assessmentRepoStub{value: a}, outcomeRepoStub{}, runRepoStub{value: &run})
	result, err := svc.Execute(context.Background(), Command{AssessmentID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || !result.Retryable || result.RunID == "" || result.FailureKind != "calculation" || result.TraceID != "trace-1" || result.InputSnapshotRef != "model:S@1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteBuildsEvaluatedReceiptFromCanonicalOutcome(t *testing.T) {
	now := time.Unix(20, 0)
	total, projectionTotal := 999.0, 42.0
	projectionRisk := domainassessment.RiskLevelLow
	a := domainassessment.Reconstruct(
		meta.FromUint64(1), 9, domaintestee.NewID(7),
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(3)),
		domainassessment.NewAdhocOrigin(), domainassessment.StatusEvaluated, &total, &projectionRisk, &now, &now, nil, nil,
	)
	execution := &domainoutcome.Execution{
		ModelRef: domainoutcome.ModelRef{ModelKind: modelcatalog.KindScale, ModelCode: "S", ModelVersion: "1", ModelTitle: "Canonical"},
		Primary:  &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: projectionTotal},
		Level:    &domainoutcome.ResultLevel{Code: "high"},
	}
	payload, err := evaloutcome.MarshalRecordV2(execution)
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID: meta.FromUint64(10), OrgID: 9, AssessmentID: meta.FromUint64(1), TesteeID: 7, RunID: "1:1",
		Model:   domainoutcome.ModelIdentity{Kind: modelcatalog.KindScale, Code: "S", Version: "1", Title: "Canonical"},
		Payload: payload, SchemaVersion: 2, EvaluatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := evalrun.NewEvaluationRun(1)
	if err := run.Start(now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := run.Succeed(now); err != nil {
		t.Fatal(err)
	}
	svc := NewService(engineStub{}, assessmentRepoStub{value: a}, outcomeRepoStub{value: record}, runRepoStub{value: &run})
	result, err := svc.Execute(context.Background(), Command{AssessmentID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome == nil || result.Outcome.ID != "10" || result.Outcome.ModelCode != "S" || result.Outcome.Title != "Canonical" {
		t.Fatalf("outcome = %#v", result.Outcome)
	}
	if result.Outcome.TotalScore == nil || *result.Outcome.TotalScore != 42 || result.Outcome.RiskLevel != "high" {
		t.Fatalf("receipt did not use canonical Outcome: %#v", result.Outcome)
	}
}

func TestExecuteRejectsEvaluatedAssessmentWithoutCanonicalOutcome(t *testing.T) {
	now := time.Unix(20, 0)
	a := domainassessment.Reconstruct(
		meta.FromUint64(1), 9, domaintestee.NewID(7),
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), domainassessment.NewAnswerSheetRef(meta.FromUint64(3)),
		domainassessment.NewAdhocOrigin(), domainassessment.StatusEvaluated, nil, nil, &now, &now, nil, nil,
	)
	svc := NewService(engineStub{}, assessmentRepoStub{value: a}, outcomeRepoStub{}, runRepoStub{})
	if _, err := svc.Execute(context.Background(), Command{AssessmentID: 1}); err == nil {
		t.Fatal("expected missing canonical Outcome error")
	}
}
