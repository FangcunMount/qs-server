//go:build refactor_target

package interpretation

import (
	"context"
	"errors"
	"testing"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestTargetReportFailureDoesNotModifyPersistedEvaluationOutcome(t *testing.T) {
	record := reportOutcomeRecord(t)
	payloadBefore := append([]byte(nil), record.Payload()...)
	outcomes := &outcomeRepoForReport{record: record}
	states := &reportStateStoreStub{}
	svc := NewOutcomeReportService(outcomes, states, &failThenGenerate{}, &durableReportSaverStub{})

	failed, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if !errors.Is(err, errReportBuild) {
		t.Fatalf("generation error = %v, want report failure", err)
	}
	if failed.Status() != domainreport.ReportStatusFailed {
		t.Fatalf("report status = %s, want failed", failed.Status())
	}
	if string(record.Payload()) != string(payloadBefore) {
		t.Fatal("report failure modified persisted EvaluationOutcome")
	}
}

func TestTargetReportRetryReadsEvaluationOutcomeWithoutEvaluationService(t *testing.T) {
	record := reportOutcomeRecord(t)
	outcomes := &outcomeRepoForReport{record: record}
	generator := &failThenGenerate{}
	svc := NewOutcomeReportService(outcomes, &reportStateStoreStub{}, generator, &durableReportSaverStub{})

	_, _ = svc.GenerateByOutcomeID(context.Background(), record.ID())
	generated, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if err != nil {
		t.Fatalf("retry generation: %v", err)
	}
	if generated.Status() != domainreport.ReportStatusGenerated || outcomes.reads != 2 || generator.calls != 2 {
		t.Fatalf("status=%s outcome_reads=%d generator_calls=%d", generated.Status(), outcomes.reads, generator.calls)
	}
}
