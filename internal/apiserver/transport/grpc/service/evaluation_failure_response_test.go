package service

import (
	"context"
	"testing"

	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/generation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

func TestGenerateReportFailureResponseUsesPersistedInterpretationFailure(t *testing.T) {
	err := &interpretationgeneration.FailedError{
		GenerationID: domaingeneration.ID(meta.FromUint64(99)),
		RunID:        interpretationrun.ID(meta.FromUint64(100)),
		Failure:      interpretationrun.Failure{Kind: interpretationrun.FailureKindTemplate, Code: "builder_not_found", SafeMessage: "报告生成器未配置", Retryable: false},
	}
	resp := generateReportFailureResponse(err)

	if resp.GetRetryable() {
		t.Fatal("expected non-retryable persisted report failure")
	}
	if resp.GetGenerationId() != "99" || resp.GetRunId() != "100" || resp.GetFailureKind() != "template" || resp.GetFailureCode() != "builder_not_found" || resp.GetMessage() != "报告生成器未配置" {
		t.Fatalf("report retry metadata = %#v", resp)
	}
}

func TestGenerateReportFailureResponseRetriesUncommittedInfrastructureError(t *testing.T) {
	resp := generateReportFailureResponse(context.DeadlineExceeded)
	if !resp.GetRetryable() || resp.GetFailureKind() != "internal" || resp.GetGenerationId() != "" {
		t.Fatalf("infrastructure response = %#v", resp)
	}
}
