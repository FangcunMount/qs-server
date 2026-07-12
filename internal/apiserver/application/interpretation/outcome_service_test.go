package interpretation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/generation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type outcomeRepoForReport struct {
	record *evaluationfact.Record
	reads  int
}

func (r *outcomeRepoForReport) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	r.reads++
	return r.record, nil
}
func (r *outcomeRepoForReport) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	r.reads++
	return r.record, nil
}

func reportOutcomeRecord(t *testing.T) *evaluationfact.Record {
	t.Helper()
	execution := domainoutcome.NewExecution(domainoutcome.ModelRef{ModelKind: modelcatalog.KindScale, ModelAlgorithm: modelcatalog.AlgorithmScaleDefault, ModelCode: "S-1", ModelVersion: "1.0.0", ModelTitle: "Scale"}, domainoutcome.Summary{PrimaryLabel: "low"}, domainoutcome.Detail{Kind: modelcatalog.KindScale})
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{ID: meta.FromUint64(9), OrgID: 11, AssessmentID: meta.FromUint64(7), TesteeID: 8, RunID: "7:1", Model: domainoutcome.ModelIdentity{Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault, Code: "S-1", Version: "1.0.0", Title: "Scale"}, Runtime: domainoutcome.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange}, Payload: payload, EvaluatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return evaluationfact.WrapRecord(record)
}

type executorStub struct {
	input  interpinput.InterpretationInput
	result *interpretationgeneration.ExecuteResult
	err    error
}

func (s *executorStub) Execute(_ context.Context, input interpinput.InterpretationInput, _ string) (*interpretationgeneration.ExecuteResult, error) {
	s.input = input
	return s.result, s.err
}

func TestOutcomeReportServiceBuildsInterpretationInputDirectlyFromRecord(t *testing.T) {
	record := reportOutcomeRecord(t)
	outcomes := &outcomeRepoForReport{record: record}
	executor := &executorStub{result: &interpretationgeneration.ExecuteResult{Status: interpretationgeneration.ExecuteStatusGenerated}}
	service := NewOutcomeReportService(outcomes, executor)

	if _, err := service.GenerateByOutcomeID(context.Background(), record.ID()); err != nil {
		t.Fatal(err)
	}
	if outcomes.reads != 1 || executor.input.OutcomeID != record.ID() || executor.input.Association.AssessmentID != record.AssessmentID() {
		t.Fatalf("outcome reads=%d input=%#v", outcomes.reads, executor.input)
	}
}

func TestOutcomeReportServiceLooksUpAssessmentOnlyToReadOutcome(t *testing.T) {
	record := reportOutcomeRecord(t)
	outcomes := &outcomeRepoForReport{record: record}
	service := NewOutcomeReportService(outcomes, &executorStub{result: &interpretationgeneration.ExecuteResult{Status: interpretationgeneration.ExecuteStatusProcessing}})
	if _, err := service.GenerateByAssessmentID(context.Background(), meta.FromUint64(record.AssessmentID().Uint64())); err != nil {
		t.Fatal(err)
	}
	if outcomes.reads != 1 {
		t.Fatalf("outcome reads=%d", outcomes.reads)
	}
}
