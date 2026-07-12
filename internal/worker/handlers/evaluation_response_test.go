package handlers

import (
	"testing"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
)

func TestHandleEvaluateAssessmentResponseNilResponse(t *testing.T) {
	if err := handleEvaluateAssessmentResponse(nil); err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestHandleEvaluateAssessmentResponseSuccess(t *testing.T) {
	if err := handleEvaluateAssessmentResponse(&evalpb.ExecuteEvaluationResponse{Status: "evaluated"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleEvaluateAssessmentResponseRejectsInterpretationStatus(t *testing.T) {
	for _, status := range []string{"interpreted", "already_interpreted", "generated"} {
		t.Run(status, func(t *testing.T) {
			if err := handleEvaluateAssessmentResponse(&evalpb.ExecuteEvaluationResponse{Status: status}); err == nil {
				t.Fatalf("status %q must not be acknowledged as Evaluation success", status)
			}
		})
	}
}

func TestHandleEvaluateAssessmentResponseTerminalStatusesAck(t *testing.T) {
	for _, status := range []string{"failed", "already_evaluated"} {
		t.Run(status, func(t *testing.T) {
			err := handleEvaluateAssessmentResponse(&evalpb.ExecuteEvaluationResponse{
				Status:         status,
				FailureMessage: "terminal",
			})
			if err != nil {
				t.Fatalf("status %q: unexpected error: %v", status, err)
			}
		})
	}
}

func TestHandleEvaluateAssessmentResponseRetryableFailedNacks(t *testing.T) {
	err := handleEvaluateAssessmentResponse(&evalpb.ExecuteEvaluationResponse{
		Status:         "failed",
		FailureMessage: "calculation failed",
		Retryable:      true,
		RunId:          "42:1",
		FailureKind:    "calculation",
	})
	if err == nil {
		t.Fatal("expected retryable failed status to nack")
	}
}

func TestHandleEvaluateAssessmentResponseRetryableFailure(t *testing.T) {
	for _, status := range []string{"", "skipped", "processing"} {
		t.Run(status, func(t *testing.T) {
			err := handleEvaluateAssessmentResponse(&evalpb.ExecuteEvaluationResponse{
				Status:         status,
				FailureMessage: "temporary",
			})
			if err == nil {
				t.Fatalf("status %q: expected retryable error", status)
			}
		})
	}
}

func TestHandleGenerateReportResponseNilResponse(t *testing.T) {
	if err := handleGenerateReportResponse(nil); err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestHandleGenerateReportResponseTerminalStatusesAck(t *testing.T) {
	for _, status := range []string{"failed", "already_generated"} {
		t.Run(status, func(t *testing.T) {
			err := handleGenerateReportResponse(&interpretationpb.GenerateReportFromAssessmentResponse{
				Success: false,
				Status:  status,
				Message: "terminal",
			})
			if err != nil {
				t.Fatalf("status %q: unexpected error: %v", status, err)
			}
		})
	}
}

func TestHandleGenerateReportResponseAcksNonRetryableGenerationFailure(t *testing.T) {
	err := handleGenerateReportResponse(&interpretationpb.GenerateReportFromAssessmentResponse{
		Success:      false,
		Status:       "failed",
		Retryable:    false,
		GenerationId: "generation-9",
		RunId:        "run-2",
		FailureKind:  "template",
		FailureCode:  "builder_not_found",
		Message:      "报告生成器未配置",
	})
	if err != nil {
		t.Fatalf("non-retryable generation failure must ACK: %v", err)
	}
}

func TestHandleGenerateReportResponseRetryableFailedNacks(t *testing.T) {
	err := handleGenerateReportResponse(&interpretationpb.GenerateReportFromAssessmentResponse{
		Success:   false,
		Status:    "failed",
		Message:   "temporary",
		Retryable: true,
	})
	if err == nil {
		t.Fatal("expected retryable failed report generation to nack")
	}
}

func TestHandleGenerateReportResponseRetryableFailure(t *testing.T) {
	err := handleGenerateReportResponse(&interpretationpb.GenerateReportFromAssessmentResponse{
		Success: false,
		Status:  "skipped",
		Message: "temporary unavailable",
	})
	if err == nil {
		t.Fatal("expected retryable error")
	}
}
