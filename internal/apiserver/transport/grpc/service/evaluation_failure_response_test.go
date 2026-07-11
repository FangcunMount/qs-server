package service

import (
	"context"
	"testing"

	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
)

type stubRunQueryService struct {
	run *runqueryApp.RunResult
	err error
}

func (s stubRunQueryService) ListByAssessmentID(context.Context, uint64, int) (*runqueryApp.RunListResult, error) {
	return nil, nil
}

func (s stubRunQueryService) FindLatestByAssessmentID(context.Context, uint64) (*runqueryApp.RunResult, error) {
	return s.run, s.err
}

func (s stubRunQueryService) ListRetryableFailed(context.Context, int64, int, uint64) (*runqueryApp.RetryableFailedListResult, error) {
	return nil, nil
}

func TestEvaluateFailureResponsePopulatesRetryableFromLatestRun(t *testing.T) {
	resp := evaluateFailureResponse(context.Background(), stubRunQueryService{
		run: &runqueryApp.RunResult{
			RunID:     "42:1",
			Retryable: true,
			ErrorCode: "calculation",
		},
	}, 42, "calculation failed")

	if resp.GetSuccess() {
		t.Fatal("expected failure response")
	}
	if resp.GetStatus() != "failed" {
		t.Fatalf("status = %q, want failed", resp.GetStatus())
	}
	if !resp.GetRetryable() {
		t.Fatal("expected retryable=true")
	}
	if resp.GetRunId() != "42:1" {
		t.Fatalf("run_id = %q, want 42:1", resp.GetRunId())
	}
	if resp.GetFailureKind() != "calculation" {
		t.Fatalf("failure_kind = %q, want calculation", resp.GetFailureKind())
	}
}

func TestEvaluateFailureResponseWithoutRunDefaultsNonRetryable(t *testing.T) {
	resp := evaluateFailureResponse(context.Background(), stubRunQueryService{}, 42, "validation failed")
	if resp.GetRetryable() {
		t.Fatal("expected retryable=false when no run metadata")
	}
}

func TestGenerateReportFailureResponseUsesIndependentReportRetry(t *testing.T) {
	resp := generateReportFailureResponse(context.Background(), stubRunQueryService{
		run: &runqueryApp.RunResult{
			RunID:     "99:2",
			Retryable: false,
			ErrorCode: "validation",
		},
	}, 99, "report failed")

	if !resp.GetRetryable() {
		t.Fatal("expected report failure to remain retryable")
	}
	if resp.GetRunId() != "" || resp.GetFailureKind() != "report_generation" {
		t.Fatalf("report retry metadata = run:%q kind:%q", resp.GetRunId(), resp.GetFailureKind())
	}
}
