package handlers

import (
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

func TestHandleEvaluateAssessmentResponseNilResponse(t *testing.T) {
	if err := handleEvaluateAssessmentResponse(nil); err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestHandleEvaluateAssessmentResponseSuccess(t *testing.T) {
	if err := handleEvaluateAssessmentResponse(&pb.EvaluateAssessmentResponse{Success: true, Status: "interpreted"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleEvaluateAssessmentResponseTerminalStatusesAck(t *testing.T) {
	for _, status := range []string{"failed", "already_interpreted", "already_evaluated"} {
		t.Run(status, func(t *testing.T) {
			err := handleEvaluateAssessmentResponse(&pb.EvaluateAssessmentResponse{
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

func TestHandleEvaluateAssessmentResponseRetryableFailedNacks(t *testing.T) {
	err := handleEvaluateAssessmentResponse(&pb.EvaluateAssessmentResponse{
		Success:     false,
		Status:      "failed",
		Message:     "calculation failed",
		Retryable:   true,
		RunId:       "42:1",
		FailureKind: "calculation",
	})
	if err == nil {
		t.Fatal("expected retryable failed status to nack")
	}
}

func TestHandleEvaluateAssessmentResponseRetryableFailure(t *testing.T) {
	for _, status := range []string{"", "skipped", "processing"} {
		t.Run(status, func(t *testing.T) {
			err := handleEvaluateAssessmentResponse(&pb.EvaluateAssessmentResponse{
				Success: false,
				Status:  status,
				Message: "temporary",
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
	for _, status := range []string{"failed", "already_interpreted"} {
		t.Run(status, func(t *testing.T) {
			err := handleGenerateReportResponse(&pb.GenerateReportFromAssessmentResponse{
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

func TestHandleGenerateReportResponseRetryableFailedNacks(t *testing.T) {
	err := handleGenerateReportResponse(&pb.GenerateReportFromAssessmentResponse{
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
	err := handleGenerateReportResponse(&pb.GenerateReportFromAssessmentResponse{
		Success: false,
		Status:  "skipped",
		Message: "temporary unavailable",
	})
	if err == nil {
		t.Fatal("expected retryable error")
	}
}
