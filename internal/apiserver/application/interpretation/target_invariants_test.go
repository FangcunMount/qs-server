package interpretation

import (
	"context"
	"errors"
	"testing"

	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/generation"
)

func TestReportGenerationFailureDoesNotModifyEvaluationOutcome(t *testing.T) {
	record := reportOutcomeRecord(t)
	payloadBefore := append([]byte(nil), record.Payload()...)
	service := NewOutcomeReportService(&outcomeRepoForReport{record: record}, &executorStub{err: errors.New("build failed")})
	if _, err := service.GenerateByOutcomeID(context.Background(), record.ID()); err == nil {
		t.Fatal("generation error = nil")
	}
	if string(record.Payload()) != string(payloadBefore) {
		t.Fatal("report generation modified persisted EvaluationOutcome")
	}
}

func TestReportRetryUsesOutcomeInputWithoutEvaluator(t *testing.T) {
	record := reportOutcomeRecord(t)
	executor := &executorStub{result: &interpretationgeneration.ExecuteResult{Status: interpretationgeneration.ExecuteStatusGenerated}}
	service := NewOutcomeReportService(&outcomeRepoForReport{record: record}, executor)
	if _, err := service.GenerateByOutcomeID(context.Background(), record.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateByOutcomeID(context.Background(), record.ID()); err != nil {
		t.Fatal(err)
	}
	if executor.input.OutcomeID != record.ID() {
		t.Fatalf("input outcome id=%s", executor.input.OutcomeID)
	}
}
